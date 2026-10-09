package data

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// DefaultRepoMapTokens is the repository map's budget when none is configured.
const DefaultRepoMapTokens = 8000

// Directories whose contents say nothing about the repository's design. The
// map shows each as one line with its file count.
var repoMapCollapsed = map[string]bool{
	"vendor": true, "node_modules": true, "dist": true, "build": true, "out": true,
	"target": true, "third_party": true, "bin": true, "obj": true, "__pycache__": true,
	"coverage": true, ".venv": true, "venv": true,
}

// repoMapFilesPerDir is the usual cap on the files listed in one directory;
// the layout lists more when the budget allows and fewer when it is tight.
const repoMapFilesPerDir = 12

// repoMapLanguages names languages by file extension.
var repoMapLanguages = map[string]string{
	".go": "Go", ".ts": "TypeScript", ".tsx": "TypeScript", ".js": "JavaScript", ".jsx": "JavaScript",
	".mjs": "JavaScript", ".py": "Python", ".rs": "Rust", ".java": "Java", ".kt": "Kotlin",
	".cs": "C#", ".rb": "Ruby", ".php": "PHP", ".swift": "Swift", ".c": "C", ".h": "C",
	".cpp": "C++", ".hpp": "C++", ".scala": "Scala", ".ex": "Elixir", ".exs": "Elixir",
	".sh": "Shell", ".ps1": "PowerShell", ".sql": "SQL", ".lua": "Lua", ".dart": "Dart",
	".vue": "Vue", ".svelte": "Svelte",
}

// repoMapTests recognises test files, by language.
var repoMapTests = []struct {
	label string
	match func(string) bool
}{
	{"Go tests beside the code (`*_test.go`)", func(p string) bool { return strings.HasSuffix(p, "_test.go") }},
	{"TypeScript and JavaScript tests (`*.test.*`, `*.spec.*`)", func(p string) bool {
		base := path.Base(p)
		return regexp.MustCompile(`\.(test|spec)\.[cm]?[jt]sx?$`).MatchString(base)
	}},
	{"Python tests (`test_*.py`, `*_test.py`)", func(p string) bool {
		base := path.Base(p)
		return strings.HasSuffix(base, ".py") && (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py"))
	}},
	{"Rust tests (`tests/*.rs`)", func(p string) bool { return strings.HasSuffix(p, ".rs") && strings.Contains("/"+p, "/tests/") }},
	{"C# tests (`*Tests.cs`)", func(p string) bool { return strings.HasSuffix(p, "Tests.cs") || strings.HasSuffix(p, "Test.cs") }},
}

var (
	repoMapADR        = regexp.MustCompile(`(^|/)(docs/)?(adr|adrs|decisions)/[^/]+\.md$|(^|/)docs/ADR-[^/]+\.md$`)
	repoMapMakeTarget = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9_.-]*):([^=]|$)`)
	repoMapGoDoc      = regexp.MustCompile(`^//\s*Package\s+\w+\s+(.*)$`)
)

// BuildRepoMap describes the repository at root, whose tracked files are files
// at commit, in Markdown held to about budget tokens. It needs no model: the
// layout comes from the file list, purposes from package documentation and
// READMEs, and conventions from file names and build files. The domain
// documents — every CONTEXT.md, and each decision record by title and
// decision — are kept whole where the budget allows, then the layout is
// trimmed from its deepest levels up until the map fits.
func BuildRepoMap(root string, files []string, commit string, budget int) string {
	if budget <= 0 {
		budget = DefaultRepoMapTokens
	}
	limit := budget * 4 // characters, at about four per token
	sort.Strings(files)

	var head strings.Builder
	short := commit
	if len(short) > 12 {
		short = short[:12]
	}
	fmt.Fprintf(&head, "# Repository map\n\nCommit %s · %d tracked files. Built by Grove from the tracked files; read the files themselves for detail.\n\n", short, len(files))
	head.WriteString(repoMapConventions(root, files))

	domain := repoMapDomain(root, files, limit*2/5)
	tree := newRepoTree(root, files)
	layoutLimit := limit - head.Len() - len(domain)
	layout := tree.render(layoutLimit)

	var b strings.Builder
	b.WriteString(head.String())
	b.WriteString(domain)
	b.WriteString(layout)
	return b.String()
}

// repoMapConventions lists the languages, the test layout, and the build and
// test commands.
func repoMapConventions(root string, files []string) string {
	counts := map[string]int{}
	for _, f := range files {
		if lang, ok := repoMapLanguages[strings.ToLower(path.Ext(f))]; ok && !repoMapIsCollapsed(f) {
			counts[lang]++
		}
	}
	var langs []string
	for lang := range counts {
		langs = append(langs, lang)
	}
	sort.Slice(langs, func(i, j int) bool {
		if counts[langs[i]] != counts[langs[j]] {
			return counts[langs[i]] > counts[langs[j]]
		}
		return langs[i] < langs[j]
	})
	if len(langs) > 6 {
		langs = langs[:6]
	}

	var b strings.Builder
	b.WriteString("## Conventions\n\n")
	if len(langs) > 0 {
		parts := make([]string, len(langs))
		for i, lang := range langs {
			parts[i] = fmt.Sprintf("%s (%s)", lang, plural(counts[lang], "file"))
		}
		fmt.Fprintf(&b, "- Languages: %s\n", strings.Join(parts, ", "))
	}
	for _, kind := range repoMapTests {
		n := 0
		for _, f := range files {
			if !repoMapIsCollapsed(f) && kind.match(f) {
				n++
			}
		}
		if n > 0 {
			fmt.Fprintf(&b, "- Tests: %s, %s\n", kind.label, plural(n, "file"))
		}
	}
	var commands []string
	for _, f := range files {
		if repoMapIsCollapsed(f) {
			continue
		}
		switch path.Base(f) {
		case "Makefile":
			if targets := makeTargets(filepath.Join(root, filepath.FromSlash(f))); len(targets) > 0 {
				commands = append(commands, fmt.Sprintf("`%s` targets: %s", f, strings.Join(targets, ", ")))
			}
		case "package.json":
			if scripts := packageScripts(filepath.Join(root, filepath.FromSlash(f))); len(scripts) > 0 {
				commands = append(commands, fmt.Sprintf("`%s` scripts: %s", f, strings.Join(scripts, ", ")))
			}
		}
	}
	if len(commands) > 6 {
		commands = commands[:6]
	}
	if len(commands) > 0 {
		b.WriteString("- Build and test commands:\n")
		for _, c := range commands {
			fmt.Fprintf(&b, "  - %s\n", c)
		}
	}
	b.WriteString("\n")
	return b.String()
}

// repoMapDomain renders the glossaries in full and the decision records by
// title and decision, within limit characters.
func repoMapDomain(root string, files []string, limit int) string {
	var contexts, adrs []string
	for _, f := range files {
		switch {
		case repoMapIsCollapsed(f):
		case path.Base(f) == "CONTEXT.md" || path.Base(f) == "CONTEXT-MAP.md":
			contexts = append(contexts, f)
		case repoMapADR.MatchString(f):
			adrs = append(adrs, f)
		}
	}
	if len(contexts) == 0 && len(adrs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Domain documents\n\n")
	for _, f := range contexts {
		body := strings.TrimSpace(readText(filepath.Join(root, filepath.FromSlash(f))))
		room := limit - b.Len() - len(adrs)*160
		if room < 200 {
			fmt.Fprintf(&b, "### %s\n\n(not shown: over the map's budget; read the file)\n\n", f)
			continue
		}
		if len(body) > room {
			body = body[:room] + "\n\n(truncated; read the file for the rest)"
		}
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", f, body)
	}
	if len(adrs) > 0 {
		b.WriteString("### Decision records\n\n")
		for _, f := range adrs {
			title, decision := adrSummary(filepath.Join(root, filepath.FromSlash(f)))
			line := fmt.Sprintf("- `%s` — %s", f, title)
			if decision != "" {
				line += ": " + decision
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func repoMapIsCollapsed(file string) bool {
	parts := strings.Split(file, "/")
	for _, p := range parts[:len(parts)-1] {
		if repoMapCollapsed[p] || (strings.HasPrefix(p, ".") && p != ".github" && p != ".grove") {
			return true
		}
	}
	return false
}

func readText(file string) string {
	raw, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

// firstSentence shortens a paragraph to its first sentence, at most max
// characters.
func firstSentence(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i+1]
	}
	if len(s) > max {
		s = strings.TrimSpace(s[:max-1]) + "…"
	}
	return s
}

// adrSummary returns a decision record's title and the first sentence of its
// decision, or of its first paragraph when it has no Decision section.
func adrSummary(file string) (title, decision string) {
	// The first paragraph of each section, by lower-cased heading, in order.
	type section struct{ heading, para string }
	var sections []section
	var para []string
	flush := func() {
		if len(sections) > 0 && sections[len(sections)-1].para == "" && len(para) > 0 {
			sections[len(sections)-1].para = strings.Join(para, " ")
		}
		para = nil
	}
	sections = append(sections, section{})
	for _, line := range strings.Split(readText(file), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "# ") && title == "":
			flush()
			title = strings.TrimPrefix(trimmed, "# ")
		case strings.HasPrefix(trimmed, "#"):
			flush()
			sections = append(sections, section{heading: strings.ToLower(strings.TrimLeft(trimmed, "# "))})
		case trimmed == "":
			flush()
		default:
			para = append(para, trimmed)
		}
	}
	flush()
	if title == "" {
		title = strings.TrimSuffix(path.Base(filepath.ToSlash(file)), ".md")
	}
	for _, s := range sections {
		if strings.Contains(s.heading, "decision") && s.para != "" {
			return title, firstSentence(s.para, 200)
		}
	}
	// Without a Decision section, the first paragraph that is not status or
	// date metadata.
	for _, s := range sections {
		if s.para != "" && s.heading != "status" && s.heading != "date" {
			return title, firstSentence(s.para, 200)
		}
	}
	return title, ""
}

func makeTargets(file string) []string {
	f, err := os.Open(file)
	if err != nil {
		return nil
	}
	defer f.Close()
	var targets []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() && len(targets) < 12 {
		if m := repoMapMakeTarget.FindStringSubmatch(scanner.Text()); m != nil && m[1] != ".PHONY" {
			targets = append(targets, m[1])
		}
	}
	return targets
}

func packageScripts(file string) []string {
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal([]byte(readText(file)), &pkg) != nil {
		return nil
	}
	var names []string
	for name := range pkg.Scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 12 {
		names = names[:12]
	}
	return names
}

// repoDir is one directory of the layout.
type repoDir struct {
	name    string
	path    string
	files   []string
	dirs    map[string]*repoDir
	total   int // files at or below this directory
	purpose string
}

type repoTree struct {
	root *repoDir
}

func newRepoTree(root string, files []string) repoTree {
	top := &repoDir{dirs: map[string]*repoDir{}}
	for _, f := range files {
		parts := strings.Split(f, "/")
		d := top
		d.total++
		for i, p := range parts[:len(parts)-1] {
			child, ok := d.dirs[p]
			if !ok {
				child = &repoDir{name: p, path: strings.Join(parts[:i+1], "/"), dirs: map[string]*repoDir{}}
				d.dirs[p] = child
			}
			d = child
			d.total++
		}
		d.files = append(d.files, parts[len(parts)-1])
	}
	var describe func(d *repoDir)
	describe = func(d *repoDir) {
		if d.path != "" && !repoMapCollapsed[d.name] {
			d.purpose = dirPurpose(root, d)
		}
		if repoMapCollapsed[d.name] {
			return
		}
		for _, c := range d.dirs {
			describe(c)
		}
	}
	describe(top)
	return repoTree{root: top}
}

// dirPurpose is one line on what a directory is for: its README's first
// sentence, its Go package documentation, or its package.json description.
func dirPurpose(root string, d *repoDir) string {
	dir := filepath.Join(root, filepath.FromSlash(d.path))
	for _, f := range d.files {
		if strings.EqualFold(f, "README.md") || strings.EqualFold(f, "README") {
			if s := readmeSentence(filepath.Join(dir, f)); s != "" {
				return s
			}
		}
	}
	for _, f := range d.files {
		if strings.HasSuffix(f, ".go") && !strings.HasSuffix(f, "_test.go") {
			if s := goPackageDoc(filepath.Join(dir, f)); s != "" {
				return s
			}
		}
	}
	for _, f := range d.files {
		if f == "package.json" {
			var pkg struct {
				Description string `json:"description"`
			}
			if json.Unmarshal([]byte(readText(filepath.Join(dir, f))), &pkg) == nil && pkg.Description != "" {
				return firstSentence(pkg.Description, 120)
			}
		}
	}
	return ""
}

func readmeSentence(file string) string {
	var para []string
	for _, line := range strings.Split(readText(file), "\n") {
		trimmed := strings.TrimSpace(line)
		skip := strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "[!") ||
			strings.HasPrefix(trimmed, "![") || strings.HasPrefix(trimmed, "<") || strings.HasPrefix(trimmed, "|")
		switch {
		case trimmed == "" || skip:
			if len(para) > 0 {
				return firstSentence(strings.Join(para, " "), 120)
			}
		default:
			para = append(para, trimmed)
		}
	}
	return firstSentence(strings.Join(para, " "), 120)
}

func goPackageDoc(file string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var doc []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(doc) == 0 {
			if m := repoMapGoDoc.FindStringSubmatch(line); m != nil {
				doc = append(doc, m[1])
			}
			if strings.HasPrefix(line, "package ") {
				return ""
			}
			continue
		}
		if !strings.HasPrefix(line, "//") {
			break
		}
		doc = append(doc, strings.TrimSpace(strings.TrimPrefix(line, "//")))
	}
	return firstSentence(strings.Join(doc, " "), 120)
}

// render draws the layout within limit characters. It tries every depth from
// the deepest down and keeps the deepest that fits; directories below the
// depth shown appear as one line with their file count.
func (t repoTree) render(limit int) string {
	maxDepth := 1
	var depth func(d *repoDir, n int)
	depth = func(d *repoDir, n int) {
		maxDepth = max(maxDepth, n)
		for _, c := range d.dirs {
			depth(c, n+1)
		}
	}
	depth(t.root, 0)
	var out string
	for level := maxDepth + 1; level >= 1; level-- {
		for _, cap := range []int{1 << 30, 24, repoMapFilesPerDir, 4} {
			out = t.renderDepth(level, cap)
			if len(out) <= limit {
				return out
			}
		}
	}
	return out
}

func (t repoTree) renderDepth(level, fileCap int) string {
	var b strings.Builder
	b.WriteString("## Layout\n\n```\n")
	var walk func(d *repoDir, indent string, n int)
	walk = func(d *repoDir, indent string, n int) {
		names := make([]string, 0, len(d.dirs))
		for name := range d.dirs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			c := d.dirs[name]
			line := fmt.Sprintf("%s%s/", indent, name)
			expand := n+1 < level && !repoMapCollapsed[name]
			if !expand {
				line += " (" + plural(c.total, "file") + ")"
			}
			if c.purpose != "" {
				line += " — " + c.purpose
			}
			b.WriteString(line + "\n")
			if expand {
				walk(c, indent+"  ", n+1)
			}
		}
		if n < level {
			files := d.files
			more := 0
			if len(files) > fileCap {
				more = len(files) - fileCap
				files = files[:fileCap]
			}
			for _, f := range files {
				b.WriteString(indent + f + "\n")
			}
			if more > 0 {
				fmt.Fprintf(&b, "%s… %d more files\n", indent, more)
			}
		}
	}
	walk(t.root, "", 0)
	b.WriteString("```\n")
	return b.String()
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// RepoMapPath is where the repository map for commit is kept.
func (s *LabStore) RepoMapPath(commit string) string {
	return filepath.Join(s.dir, "repo-map", commit+".md")
}

// EnsureRepoMap writes the repository map for commit unless one exists, and
// returns its path. The map depends only on the commit's tracked files, so
// it is built once per commit.
func (s *LabStore) EnsureRepoMap(checkout string, files []string, commit string, budget int) (string, error) {
	target := s.RepoMapPath(commit)
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("create the repository map directory: %w", err)
	}
	if err := writeFileAtomic(target, []byte(BuildRepoMap(checkout, files, commit, budget))); err != nil {
		return "", fmt.Errorf("write the repository map: %w", err)
	}
	return target, nil
}
