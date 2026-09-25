// Package markdown renders the GitHub-flavoured Markdown that workflows write
// as reports into styled terminal text. It covers what those reports use:
// headings, paragraphs, nested and task lists, block quotes, fenced code,
// rules, and tables, with bold, italic, code, and link spans inside them.
// Every rendered line fits within the requested width.
package markdown

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

// minWidth is the narrowest width Render lays text out for.
const minWidth = 12

// palette holds the styles Render draws with, derived from a theme.
type palette struct {
	text, muted, accent, code lipgloss.Style
}

func newPalette(t styles.Theme) palette {
	c := func(hex string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)) }
	return palette{
		text:   c(t.Fg()),
		muted:  c(t.Muted()),
		accent: c(t.Accent()),
		code:   c(t.Warning()),
	}
}

var (
	headingRe = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	ruleRe    = regexp.MustCompile(`^\s{0,3}([-*_])(\s*[-*_]){2,}\s*$`)
	listRe    = regexp.MustCompile(`^(\s*)([-*+]|\d{1,9}[.)])\s+(.*)$`)
	fenceRe   = regexp.MustCompile("^\\s{0,3}(```+|~~~+)\\s*([^`\\s]*)")
	tableSep  = regexp.MustCompile(`^\s*:?-{1,}:?\s*$`)
)

// Render lays out src at the given width in the theme's colours.
func Render(src string, width int, theme styles.Theme) string {
	if width < minWidth {
		width = minWidth
	}
	r := renderer{p: newPalette(theme), width: width}
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	return strings.Join(r.blocks(lines), "\n")
}

type renderer struct {
	p     palette
	width int
}

// blocks renders a sequence of source lines, separating blocks with one
// blank line.
func (r renderer) blocks(lines []string) []string {
	var out []string
	emit := func(block []string) {
		if len(block) == 0 {
			return
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, block...)
	}

	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			i++
		case strings.HasPrefix(trimmed, "<!--"):
			for i < len(lines) && !strings.Contains(lines[i], "-->") {
				i++
			}
			i++
		case fenceRe.MatchString(line):
			m := fenceRe.FindStringSubmatch(line)
			fence := m[1]
			var code []string
			i++
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), fence[:3]) {
				code = append(code, lines[i])
				i++
			}
			i++ // closing fence
			emit(r.codeBlock(code, m[2]))
		case headingRe.MatchString(trimmed):
			m := headingRe.FindStringSubmatch(trimmed)
			emit(r.heading(len(m[1]), m[2]))
			i++
		case ruleRe.MatchString(line):
			emit([]string{r.p.muted.Render(strings.Repeat("─", r.width))})
			i++
		case strings.HasPrefix(trimmed, ">"):
			var quote []string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), ">") {
				q := strings.TrimPrefix(strings.TrimSpace(lines[i]), ">")
				quote = append(quote, strings.TrimPrefix(q, " "))
				i++
			}
			emit(r.quote(quote))
		case strings.HasPrefix(trimmed, "|"):
			var rows []string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
				rows = append(rows, strings.TrimSpace(lines[i]))
				i++
			}
			emit(r.table(rows))
		case listRe.MatchString(line):
			var items []string
			for i < len(lines) {
				l := lines[i]
				t := strings.TrimSpace(l)
				if t == "" {
					// A blank line ends the list unless another item follows.
					if i+1 < len(lines) && listRe.MatchString(lines[i+1]) {
						i++
						continue
					}
					break
				}
				if listRe.MatchString(l) {
					items = append(items, l)
				} else if len(items) > 0 && !startsBlock(l) {
					items[len(items)-1] += " " + t
				} else {
					break
				}
				i++
			}
			emit(r.list(items))
		default:
			var para []string
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" && !startsBlock(lines[i]) {
				para = append(para, strings.TrimSpace(lines[i]))
				i++
			}
			emit(r.wrap(strings.Join(para, " "), r.width, r.p.text))
		}
	}
	return out
}

// startsBlock reports whether line opens a block other than a paragraph, which
// ends the paragraph or list item before it.
func startsBlock(line string) bool {
	t := strings.TrimSpace(line)
	return headingRe.MatchString(t) || fenceRe.MatchString(line) || ruleRe.MatchString(line) ||
		strings.HasPrefix(t, ">") || strings.HasPrefix(t, "|") || listRe.MatchString(line)
}

func (r renderer) heading(level int, text string) []string {
	switch level {
	case 1:
		return append(r.wrap(text, r.width, r.p.accent.Bold(true)), r.p.accent.Render(strings.Repeat("━", r.width)))
	case 2:
		return append(r.wrap(text, r.width, r.p.accent.Bold(true)), r.p.muted.Render(strings.Repeat("─", r.width)))
	case 3:
		return r.wrap(text, r.width, r.p.text.Bold(true))
	default:
		return r.wrap(text, r.width, r.p.muted.Bold(true))
	}
}

func (r renderer) codeBlock(code []string, lang string) []string {
	gutter := r.p.muted.Render("│ ")
	var out []string
	if lang != "" {
		out = append(out, r.p.muted.Render("╭ "+ansi.Truncate(lang, r.width-2, "…")))
	}
	for _, line := range code {
		line = strings.ReplaceAll(line, "\t", "    ")
		out = append(out, gutter+r.p.text.Render(ansi.Truncate(line, r.width-2, "…")))
	}
	if len(out) == 0 {
		out = append(out, gutter)
	}
	return out
}

func (r renderer) quote(lines []string) []string {
	gutter := r.p.accent.Render("▌ ")
	inner := renderer{p: r.p, width: max(r.width-2, minWidth-2)}
	var out []string
	for _, line := range inner.blocks(lines) {
		out = append(out, gutter+line)
	}
	return out
}

// list renders list items, nesting them by their indentation.
func (r renderer) list(items []string) []string {
	var out []string
	for _, item := range items {
		m := listRe.FindStringSubmatch(item)
		indent := strings.ReplaceAll(m[1], "\t", "    ")
		level := min(len(indent)/2, 4)
		marker := m[2]
		text := m[3]
		switch {
		case strings.HasPrefix(text, "[ ] "):
			marker, text = "○", text[4:]
		case strings.HasPrefix(text, "[x] "), strings.HasPrefix(text, "[X] "):
			marker, text = "✓", text[4:]
		case marker == "-" || marker == "*" || marker == "+":
			marker = []string{"•", "◦", "▪", "▫", "·"}[level]
		}
		prefix := strings.Repeat("  ", level) + marker + " "
		pw := lipgloss.Width(prefix)
		if pw > r.width-minWidth/2 {
			prefix, pw = marker+" ", lipgloss.Width(marker)+1
		}
		styled := r.p.accent.Render(strings.TrimSuffix(prefix, " ")) + " "
		for i, line := range r.wrap(text, r.width-pw, r.p.text) {
			if i == 0 {
				out = append(out, styled+line)
			} else {
				out = append(out, strings.Repeat(" ", pw)+line)
			}
		}
	}
	return out
}

// table renders a pipe table, wrapping cells so the table fits the width. A
// table with too many columns to be legible at this width is listed as one
// record per row instead.
func (r renderer) table(rows []string) []string {
	var grid [][]string
	header := false
	for i, row := range rows {
		cells := splitCells(row)
		if i == 1 && isSeparatorRow(cells) {
			header = true
			continue
		}
		grid = append(grid, cells)
	}
	cols := 0
	for _, row := range grid {
		cols = max(cols, len(row))
	}
	if cols == 0 {
		return nil
	}
	for i := range grid {
		for len(grid[i]) < cols {
			grid[i] = append(grid[i], "")
		}
	}

	available := r.width - 3*(cols-1)
	if available < cols*6 {
		return r.tableRecords(grid, header)
	}

	natural := make([]int, cols)
	for _, row := range grid {
		for c, cell := range row {
			natural[c] = max(natural[c], plainWidth(cell), 1)
		}
	}
	widths := fitColumns(natural, available)

	sep := r.p.muted.Render(" │ ")
	var out []string
	for ri, row := range grid {
		style := r.p.text
		if header && ri == 0 {
			style = r.p.accent.Bold(true)
		}
		wrapped := make([][]string, cols)
		height := 1
		for c, cell := range row {
			wrapped[c] = r.wrap(cell, widths[c], style)
			height = max(height, len(wrapped[c]))
		}
		for l := 0; l < height; l++ {
			parts := make([]string, cols)
			for c := range row {
				cell := ""
				if l < len(wrapped[c]) {
					cell = wrapped[c][l]
				}
				parts[c] = cell + strings.Repeat(" ", max(widths[c]-lipgloss.Width(cell), 0))
			}
			out = append(out, strings.TrimRight(strings.Join(parts, sep), " "))
		}
		if header && ri == 0 {
			rules := make([]string, cols)
			for c := range rules {
				rules[c] = strings.Repeat("─", widths[c])
			}
			out = append(out, r.p.muted.Render(strings.Join(rules, "─┼─")))
		}
	}
	return out
}

func (r renderer) tableRecords(grid [][]string, header bool) []string {
	var names []string
	body := grid
	if header {
		names, body = grid[0], grid[1:]
	}
	var out []string
	for i, row := range body {
		if i > 0 {
			out = append(out, r.p.muted.Render(strings.Repeat("┄", min(r.width, 24))))
		}
		for c, cell := range row {
			label := ""
			if c < len(names) && strings.TrimSpace(names[c]) != "" {
				label = "**" + strings.TrimSpace(names[c]) + ":** "
			}
			out = append(out, r.wrap(label+cell, r.width, r.p.text)...)
		}
	}
	return out
}

// fitColumns shares available columns among table columns. A column that
// fits in an even share keeps its natural width, and what it leaves over goes
// to the wider ones.
func fitColumns(natural []int, available int) []int {
	widths := make([]int, len(natural))
	total := 0
	for _, w := range natural {
		total += w
	}
	if total <= available {
		copy(widths, natural)
		return widths
	}
	remaining := available
	open := make([]bool, len(natural))
	for i := range open {
		open[i] = true
	}
	for {
		count := 0
		for _, o := range open {
			if o {
				count++
			}
		}
		if count == 0 {
			return widths
		}
		share := remaining / count
		settled := false
		for i, o := range open {
			if o && natural[i] <= share {
				widths[i] = natural[i]
				remaining -= natural[i]
				open[i] = false
				settled = true
			}
		}
		if !settled {
			extra := remaining - share*count
			for i, o := range open {
				if o {
					widths[i] = share
					if extra > 0 {
						widths[i]++
						extra--
					}
				}
			}
			return widths
		}
	}
}

// splitCells splits a table row on the pipes that separate cells, leaving
// escaped pipes and pipes inside code spans in the cell.
func splitCells(row string) []string {
	row = strings.TrimSpace(row)
	row = strings.TrimPrefix(row, "|")
	if strings.HasSuffix(row, "|") && !strings.HasSuffix(row, `\|`) {
		row = row[:len(row)-1]
	}
	var cells []string
	var cell strings.Builder
	inCode := false
	for i := 0; i < len(row); i++ {
		ch := row[i]
		switch {
		case ch == '\\' && i+1 < len(row) && row[i+1] == '|':
			cell.WriteByte('|')
			i++
		case ch == '`':
			inCode = !inCode
			cell.WriteByte(ch)
		case ch == '|' && !inCode:
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
		default:
			cell.WriteByte(ch)
		}
	}
	return append(cells, strings.TrimSpace(cell.String()))
}

func isSeparatorRow(cells []string) bool {
	for _, c := range cells {
		if !tableSep.MatchString(c) {
			return false
		}
	}
	return len(cells) > 0
}

// ── Inline spans ─────────────────────────────────────────────────────────────

type spanKind uint8

const (
	spanBold spanKind = 1 << iota
	spanItalic
	spanCode
	spanLink
)

type span struct {
	text string
	kind spanKind
}

// parseInline splits text into runs of plain, bold, italic, code, and link
// text. Link targets are dropped; the link text is kept.
func parseInline(text string) []span {
	var spans []span
	var cur strings.Builder
	kind := spanKind(0)
	flush := func() {
		if cur.Len() > 0 {
			spans = append(spans, span{text: cur.String(), kind: kind})
			cur.Reset()
		}
	}
	for i := 0; i < len(text); i++ {
		ch := text[i]
		switch {
		case ch == '\\' && i+1 < len(text) && strings.ContainsRune("\\`*_[]()#+-.!|<>", rune(text[i+1])):
			cur.WriteByte(text[i+1])
			i++
		case ch == '`':
			ticks := 1
			for i+ticks < len(text) && text[i+ticks] == '`' {
				ticks++
			}
			marker := strings.Repeat("`", ticks)
			end := strings.Index(text[i+ticks:], marker)
			if end < 0 {
				cur.WriteString(marker)
				i += ticks - 1
				continue
			}
			flush()
			code := strings.TrimSpace(text[i+ticks : i+ticks+end])
			spans = append(spans, span{text: code, kind: kind | spanCode})
			i += ticks + end + ticks - 1
		case (ch == '*' || ch == '_') && i+1 < len(text) && text[i+1] == ch:
			flush()
			kind ^= spanBold
			i++
		case ch == '*' && italicDelimiter(text, i, kind&spanItalic != 0):
			flush()
			kind ^= spanItalic
		case ch == '[':
			close := strings.Index(text[i:], "](")
			if close < 0 {
				cur.WriteByte(ch)
				continue
			}
			end := strings.IndexByte(text[i+close:], ')')
			if end < 0 {
				cur.WriteByte(ch)
				continue
			}
			flush()
			for _, s := range parseInline(text[i+1 : i+close]) {
				spans = append(spans, span{text: s.text, kind: s.kind | kind | spanLink})
			}
			i += close + end
		default:
			cur.WriteByte(ch)
		}
	}
	flush()
	return spans
}

// italicDelimiter reports whether the '*' at i opens or closes emphasis, as
// opposed to being a literal asterisk such as a multiplication sign.
func italicDelimiter(text string, i int, open bool) bool {
	if open {
		return i > 0 && text[i-1] != ' '
	}
	return i+1 < len(text) && text[i+1] != ' ' && strings.Contains(text[i+1:], "*")
}

func (r renderer) styleFor(base lipgloss.Style, kind spanKind) lipgloss.Style {
	st := base
	if kind&spanCode != 0 {
		st = r.p.code
	}
	if kind&spanLink != 0 {
		st = r.p.accent.Underline(true)
	}
	if kind&spanBold != 0 {
		st = st.Bold(true)
	}
	if kind&spanItalic != 0 {
		st = st.Italic(true)
	}
	return st
}

// plainWidth is the display width of text once its inline markup is removed.
func plainWidth(text string) int {
	w := 0
	for _, s := range parseInline(text) {
		w += lipgloss.Width(s.text)
	}
	return w
}

// piece is a run of one word drawn in one style.
type piece struct {
	text  string
	style lipgloss.Style
}

// wrap renders inline markup in text and breaks it into lines of at most width
// columns at spaces, splitting a word only when it is wider than a line.
func (r renderer) wrap(text string, width int, base lipgloss.Style) []string {
	width = max(width, 1)
	var words [][]piece
	var word []piece
	endWord := func() {
		if len(word) > 0 {
			words = append(words, word)
			word = nil
		}
	}
	for _, s := range parseInline(text) {
		st := r.styleFor(base, s.kind)
		for j, part := range strings.Split(s.text, " ") {
			if j > 0 {
				endWord()
			}
			if part != "" {
				word = append(word, piece{text: part, style: st})
			}
		}
	}
	endWord()

	var lines []string
	var line strings.Builder
	lineWidth := 0
	// endLine closes the line being built. splitToFit accounts for the width of
	// the line it continues on, so endLine leaves lineWidth to its caller.
	endLine := func() {
		lines = append(lines, line.String())
		line.Reset()
	}
	for _, w := range words {
		ww := 0
		for _, p := range w {
			ww += lipgloss.Width(p.text)
		}
		if lineWidth > 0 && lineWidth+1+ww > width {
			endLine()
			lineWidth = 0
		}
		if lineWidth > 0 {
			line.WriteString(" ")
			lineWidth++
		}
		for _, p := range w {
			for _, chunk := range splitToFit(p.text, width, &lineWidth) {
				if chunk.newLine {
					endLine()
				}
				line.WriteString(p.style.Render(chunk.text))
			}
		}
	}
	if lineWidth > 0 || len(lines) == 0 {
		lines = append(lines, line.String())
	}
	return lines
}

type chunk struct {
	text    string
	newLine bool // the chunk starts a new line
}

// splitToFit breaks text into chunks that fit the rest of the current line and
// then whole lines, updating lineWidth as it goes.
func splitToFit(text string, width int, lineWidth *int) []chunk {
	var chunks []chunk
	var cur strings.Builder
	startNew := false
	for _, r := range text {
		rw := lipgloss.Width(string(r))
		if *lineWidth+rw > width && *lineWidth > 0 {
			if cur.Len() > 0 {
				chunks = append(chunks, chunk{text: cur.String(), newLine: startNew})
				cur.Reset()
			}
			startNew = true
			*lineWidth = 0
		}
		cur.WriteRune(r)
		*lineWidth += rw
	}
	if cur.Len() > 0 {
		chunks = append(chunks, chunk{text: cur.String(), newLine: startNew})
	}
	return chunks
}
