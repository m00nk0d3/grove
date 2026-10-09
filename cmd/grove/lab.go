package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/sandcastle"
	"github.com/m00nk0d3/grove/internal/tui/modal"
)

// labView is the Lab tab's state: every entry of the repository and what the
// list shows of them. The cursor indexes visible(), so the entry drawn as
// selected is the entry acted on.
type labView struct {
	entries []domain.LabEntry
	// archived shows the archive instead of the active groups.
	archived bool
	// doneExpanded shows every published entry, not only the latest.
	doneExpanded bool
	filter       domain.LabFilter
	cursor       int
	// runs holds the Sandcastle runs of the repository by run ID, refreshed
	// with mission control state. An entry's session state is read from its
	// runs here, never stored on the entry.
	runs map[string]labRun
	// locks are the entries' held session locks, by entry ID.
	locks map[string]data.LabLockOwner
	// talks holds what each entry's session has asked the user, by entry ID.
	talks map[string]labTalk
	// page is the open entry page, or nil while the list shows.
	page *labPage
}

// labRun is a Sandcastle run as the Lab shows it.
type labRun struct {
	workflow domain.WorkflowRunRef
	paneID   string
}

// waiting reports whether the run's agent is waiting for the user.
func (r labRun) waiting() bool {
	return strings.EqualFold(r.workflow.Status, domain.WorkflowBlocked)
}

// live reports whether the run has not finished.
func (r labRun) live() bool {
	switch strings.ToLower(r.workflow.Status) {
	case domain.WorkflowQueued:
		return true
	case domain.WorkflowRunning, domain.WorkflowBlocked:
		return r.paneID != ""
	}
	return false
}

func newLabView() labView {
	return labView{filter: domain.LabFilterAll}
}

// setMission records the runs and panes mission control reports.
func (v *labView) setMission(state *domain.MissionControlState) {
	v.runs = make(map[string]labRun)
	if state == nil {
		return
	}
	livePanes := make(map[string]struct{}, len(state.Panes))
	for _, pane := range state.Panes {
		livePanes[pane.PaneID] = struct{}{}
	}
	for _, workflow := range state.WorkflowRuns {
		id := firstNonEmptyString(workflow.RunID, workflow.WorkflowID)
		if id == "" {
			continue
		}
		run := labRun{workflow: workflow}
		for _, agent := range agentsForWorkflow(state, id) {
			if _, ok := livePanes[agent.PaneID]; ok {
				run.paneID = agent.PaneID
				break
			}
		}
		v.runs[id] = run
	}
}

// latestRun returns the entry's most recent run that mission control knows.
func (v labView) latestRun(e domain.LabEntry) (labRun, bool) {
	for i := len(e.Runs) - 1; i >= 0; i-- {
		if run, ok := v.runs[e.Runs[i]]; ok {
			return run, true
		}
	}
	return labRun{}, false
}

// entryRuns returns the entry's runs that mission control knows, oldest first.
func (v labView) entryRuns(e domain.LabEntry) []labRun {
	var out []labRun
	for _, id := range e.Runs {
		if run, ok := v.runs[id]; ok {
			out = append(out, run)
		}
	}
	return out
}

// waiting reports whether the entry's agent is waiting for the user.
func (v labView) waiting(e domain.LabEntry) bool {
	run, ok := v.latestRun(e)
	return ok && run.waiting()
}

// labGroup is a section of the Lab list. Entries are grouped by what they
// need from the user, so the list reads as a to-do list. See
// docs/LAB_DESIGN.md, "List".
type labGroup int

const (
	labGroupNeedsYou labGroup = iota
	labGroupWorking
	labGroupNotStarted
	labGroupDone
	labGroupArchived
	labGroupCount
)

var labGroupLabels = [labGroupCount]string{"NEEDS YOU", "WORKING", "NOT STARTED", "DONE", "ARCHIVED"}

// labDoneShown is how many published entries the list shows before the row
// that reveals the rest.
const labDoneShown = 5

// groupOf returns the group that lists e.
func (v labView) groupOf(e domain.LabEntry) labGroup {
	switch {
	case e.Archived:
		return labGroupArchived
	case e.Status == domain.LabStatusPublished:
		return labGroupDone
	}
	run, hasRun := v.latestRun(e)
	if ask, _ := v.ask(e); ask != labAskNone {
		return labGroupNeedsYou
	}
	if hasRun && run.live() {
		if run.waiting() {
			return labGroupNeedsYou
		}
		return labGroupWorking
	}
	if hasRun && strings.EqualFold(run.workflow.Status, domain.WorkflowFailed) {
		return labGroupNeedsYou
	}
	if e.Status == domain.LabStatusDraft {
		return labGroupNotStarted
	}
	// A session that stopped before publishing: ready to publish, to resume,
	// or to retry.
	return labGroupNeedsYou
}

// waitingSince is when e started waiting on the user, for putting the
// longest wait first.
func (v labView) waitingSince(e domain.LabEntry) time.Time {
	if run, ok := v.latestRun(e); ok && !run.workflow.UpdatedAt.IsZero() {
		return run.workflow.UpdatedAt
	}
	return e.Updated
}

// labItem is one selectable row of the list: an entry, or, when more is
// positive, the row that shows the published entries beyond the first few.
type labItem struct {
	entry domain.LabEntry
	group labGroup
	more  int
}

// items returns the list's rows in order: the active groups, or, while the
// archive is shown, the archived entries. Within NEEDS YOU the longest
// waiting comes first; elsewhere the newest.
func (v labView) items() []labItem {
	groups := make([][]domain.LabEntry, labGroupCount)
	for _, e := range v.entries {
		if e.Archived != v.archived || !v.filter.Matches(e) {
			continue
		}
		g := v.groupOf(e)
		groups[g] = append(groups[g], e)
	}
	var out []labItem
	for g := labGroup(0); g < labGroupCount; g++ {
		entries := groups[g]
		sort.SliceStable(entries, func(i, j int) bool {
			if g == labGroupNeedsYou {
				return v.waitingSince(entries[i]).Before(v.waitingSince(entries[j]))
			}
			return entries[i].Created.After(entries[j].Created)
		})
		hidden := 0
		if g == labGroupDone && !v.doneExpanded && len(entries) > labDoneShown {
			hidden = len(entries) - labDoneShown
			entries = entries[:labDoneShown]
		}
		for _, e := range entries {
			out = append(out, labItem{entry: e, group: g})
		}
		if hidden > 0 {
			out = append(out, labItem{group: g, more: hidden})
		}
	}
	return out
}

// groupCount is how many entries a group holds, hidden ones included.
func (v labView) groupCount(g labGroup) int {
	n := 0
	for _, e := range v.entries {
		if e.Archived == v.archived && v.filter.Matches(e) && v.groupOf(e) == g {
			n++
		}
	}
	return n
}

// archivedCount is how many entries the archive holds.
func (v labView) archivedCount() int {
	n := 0
	for _, e := range v.entries {
		if e.Archived {
			n++
		}
	}
	return n
}

// visible returns the entries the list shows, in order.
func (v labView) visible() []domain.LabEntry {
	var out []domain.LabEntry
	for _, it := range v.items() {
		if it.more == 0 {
			out = append(out, it.entry)
		}
	}
	return out
}

// selectedItem returns the row under the cursor.
func (v labView) selectedItem() (labItem, bool) {
	items := v.items()
	if v.cursor < 0 || v.cursor >= len(items) {
		return labItem{}, false
	}
	return items[v.cursor], true
}

// selected returns the entry under the cursor.
func (v labView) selected() (domain.LabEntry, bool) {
	it, ok := v.selectedItem()
	if !ok || it.more > 0 {
		return domain.LabEntry{}, false
	}
	return it.entry, true
}

// entry returns the stored entry with the given ID.
func (v labView) entry(id string) (domain.LabEntry, bool) {
	for _, e := range v.entries {
		if e.ID == id {
			return e, true
		}
	}
	return domain.LabEntry{}, false
}

func (v *labView) clamp() {
	n := len(v.items())
	if v.cursor >= n {
		v.cursor = n - 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
}

func (v *labView) move(delta int) {
	v.cursor += delta
	v.clamp()
}

// setArchived shows the archive, or the active entries.
func (v *labView) setArchived(archived bool) {
	v.archived = archived
	v.cursor = 0
}

func (v *labView) setFilter(f domain.LabFilter) {
	v.filter = f
	v.cursor = 0
}

// selectID puts the cursor on the entry with the given ID, showing the
// archive or the rest of DONE when that is where it is. The kind filter is
// cleared when it would hide the entry.
func (v *labView) selectID(id string) {
	e, ok := v.entry(id)
	if !ok {
		v.clamp()
		return
	}
	if !v.filter.Matches(e) {
		v.filter = domain.LabFilterAll
	}
	v.archived = e.Archived
	for pass := 0; pass < 2; pass++ {
		for i, it := range v.items() {
			if it.more == 0 && it.entry.ID == id {
				v.cursor = i
				return
			}
		}
		v.doneExpanded = true
	}
	v.clamp()
}

// labsLoadedMsg carries the repository's entries after a load or a change.
// selectID, when set, is the entry the list should move to; status is the
// message to show.
type labsLoadedMsg struct {
	// locks, when locksLoaded, are the entries' held session locks.
	locks       map[string]data.LabLockOwner
	locksLoaded bool
	// talks, when locksLoaded, are the entries' protocol files.
	talks    map[string]labTalk
	entries  []domain.LabEntry
	selectID string
	status   string
	err      error
}

// labStoreFor returns the Lab store of the repository at repoPath.
func labStoreFor(repoPath string) (*data.LabStore, error) {
	commonDir, err := gitCommonDir(repoPath)
	if err != nil {
		return nil, fmt.Errorf("locate the Lab: %w", err)
	}
	return data.NewLabStore(commonDir), nil
}

// loadLabs reads the repository's entries.
func loadLabs(repoPath string) ([]domain.LabEntry, error) {
	store, err := labStoreFor(repoPath)
	if err != nil {
		return nil, err
	}
	return store.Load()
}

// labChangeCmd applies change to the repository's Lab and reloads it.
func labChangeCmd(repoPath, selectID, status string, change func(*data.LabStore) error) tea.Cmd {
	return func() tea.Msg {
		store, err := labStoreFor(repoPath)
		if err == nil {
			err = change(store)
		}
		if err != nil {
			return labsLoadedMsg{err: err}
		}
		entries, err := store.Load()
		locks, lockErr := store.Locks()
		talks := loadLabTalks(store, entries)
		return labsLoadedMsg{entries: entries, locks: locks, locksLoaded: lockErr == nil, talks: talks, selectID: selectID, status: status, err: err}
	}
}

// handleLabsLoaded applies a load or change result.
func (m *Model) handleLabsLoaded(msg labsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.statusErr = msg.err.Error()
		return m, clearErrorCmd()
	}
	m.lab.entries = msg.entries
	if msg.locksLoaded {
		m.lab.locks = msg.locks
		m.lab.talks = msg.talks
	}
	defer m.syncLabPage()
	if msg.selectID != "" {
		m.lab.selectID(msg.selectID)
	} else {
		m.lab.clamp()
	}
	m.refreshLabInspector()
	if msg.status == "" {
		return m, nil
	}
	m.statusMsg = msg.status
	return m, clearMsgCmd()
}

// handleLabCaptureSubmitted stores a captured or edited entry.
func (m *Model) handleLabCaptureSubmitted(msg modal.LabCaptureSubmittedMsg) (tea.Model, tea.Cmd) {
	m.activeModal = nil
	now := time.Now()
	var entry domain.LabEntry
	status := "Captured"
	if msg.ID == "" {
		entry = domain.NewLabEntry(msg.Kind, msg.Text, now)
	} else {
		existing, ok := m.lab.entry(msg.ID)
		if !ok {
			m.statusErr = "That Lab entry no longer exists"
			return m, clearErrorCmd()
		}
		if !existing.Editable() {
			m.statusErr = "Only drafts can be edited"
			return m, clearErrorCmd()
		}
		entry = existing
		entry.Kind = msg.Kind
		entry.Text = domain.NormalizeLabText(msg.Text)
		entry.Updated = now.UTC()
		status = "Saved"
	}
	status = fmt.Sprintf("%s %q", status, entry.Title())
	return m, labChangeCmd(m.RepoPath, entry.ID, status, func(s *data.LabStore) error { return s.Put(entry) })
}

// handleLabDeleteConfirmed removes an entry.
func (m *Model) handleLabDeleteConfirmed(msg modal.LabDeleteConfirmedMsg) (tea.Model, tea.Cmd) {
	m.activeModal = nil
	e, ok := m.lab.entry(msg.ID)
	if !ok {
		return m, nil
	}
	status := fmt.Sprintf("Deleted %q", e.Title())
	return m, labChangeCmd(m.RepoPath, "", status, func(s *data.LabStore) error { return s.Remove(msg.ID) })
}

// openLabCapture opens the editor for a new entry. The kind follows the
// active filter, so capturing while viewing bugs captures a bug.
func (m *Model) openLabCapture() {
	kind := domain.LabKindIdea
	if m.lab.filter == domain.LabFilterBug {
		kind = domain.LabKindBug
	}
	m.activeModal = modal.NewLabCaptureModal(kind)
}

// labNextStep performs the selected entry's next step: Enter in the Lab.
func (m *Model) labNextStep() (tea.Model, tea.Cmd) {
	if it, ok := m.lab.selectedItem(); ok && it.more > 0 {
		m.lab.doneExpanded = true
		return m, nil
	}
	e, ok := m.lab.selected()
	if !ok {
		m.openLabCapture()
		return m, nil
	}
	m.openLabPage(e)
	return m, nil
}

// labPrimaryAction performs what an entry needs next: Enter on its page, for
// everything but the cards the page answers itself.
func (m *Model) labPrimaryAction(e domain.LabEntry) (tea.Model, tea.Cmd) {
	if e.Archived {
		m.statusMsg = "Archived — restore it from the Actions panel to continue"
		return m, clearMsgCmd()
	}
	if e.Status == domain.LabStatusPublished {
		return m.openLabIssue(e)
	}
	// A draft waiting for review is reviewed in Grove, not in the pane.
	if labPublishRequires(e) != nil && m.lab.draftReady(e) {
		if e.Mode == domain.LabModeGrill && e.Status == domain.LabStatusTicketed {
			return m.prepareLabPublish(e)
		}
		return m.openLabInspectorOnArtifacts()
	}
	// What the session asks is answered in Grove; the pane is only watched.
	if m.openLabAsk(e) {
		return m, nil
	}
	if run, ok := m.lab.latestRun(e); ok && run.live() && run.paneID != "" {
		return m.openLabRunPane(firstNonEmptyString(run.workflow.RunID, run.workflow.WorkflowID))
	}
	if m.lab.canShape(e) {
		return m.startLabSession(e, domain.LabModeShape)
	}
	if m.lab.canGrill(e) {
		return m.startLabSession(e, domain.LabModeGrill)
	}
	if e.Editable() {
		m.activeModal = modal.NewLabEditModal(e)
		return m, nil
	}
	return m.openLabInspector()
}

// labContextActions lists what the Actions panel offers for the selected
// entry, which depends on its state and its session.
func labContextActions(v labView) []contextActionOption {
	capture := contextActionOption{icon: "+", label: "Capture new entry", action: modal.ContextActionLabCapture}
	e, ok := v.selected()
	if !ok {
		return []contextActionOption{capture}
	}
	var actions []contextActionOption
	if labPublishRequires(e) != nil && v.draftReady(e) {
		actions = append(actions, contextActionOption{icon: "↑", label: "Publish issue", action: modal.ContextActionLabPublish})
	}
	if v.canShape(e) {
		label := "Shape into an issue"
		if e.Status == domain.LabStatusShaping {
			label = "Resume shaping"
		}
		actions = append(actions, contextActionOption{icon: "◇", label: label, action: modal.ContextActionLabShape})
	}
	if v.canEscalate(e) {
		actions = append(actions, contextActionOption{icon: "⇪", label: "Escalate to grill", action: modal.ContextActionLabEscalate})
	}
	if v.canGrill(e) {
		label := "Grill"
		if e.Status != domain.LabStatusDraft {
			label = "Resume grilling"
		}
		actions = append(actions, contextActionOption{icon: "✦", label: label, action: modal.ContextActionLabGrill})
	}
	if labPrimaryIssue(e) != nil {
		actions = append(actions,
			contextActionOption{icon: "↵", label: "Open in Issues", action: modal.ContextActionLabOpenIssue},
			contextActionOption{icon: "◉", label: "Open on GitHub", action: modal.ContextActionOpenGitHub},
		)
	}
	actions = append(actions, contextActionOption{icon: "◎", label: "Inspect", action: modal.ContextActionLabInspect})
	if v.hasLiveSession(e) {
		actions = append(actions, contextActionOption{icon: "■", label: "End session", action: modal.ContextActionLabEnd})
	}
	if t := v.talks[e.ID]; v.hasLiveSession(e) && t.hasSession && t.session.Stage == "interview" {
		actions = append(actions, contextActionOption{icon: "»", label: "Write the spec now", action: modal.ContextActionLabFinishInterview})
	}
	if run, ok := v.latestRun(e); ok && run.live() && run.paneID != "" {
		actions = append(actions, contextActionOption{icon: "◫", label: "View agent", action: modal.ContextActionLabViewAgent})
	}
	if owner, ok := v.foreignLock(e); ok {
		actions = append(actions, contextActionOption{icon: "⊘", label: "Clear lock from " + owner.Host, action: modal.ContextActionLabClearLock})
	}
	switch {
	case e.Archived:
		actions = append(actions,
			contextActionOption{icon: "↺", label: "Restore", action: modal.ContextActionLabRestore},
			contextActionOption{icon: "!", label: "Delete", action: modal.ContextActionLabDelete},
		)
	case e.Editable():
		actions = append(actions,
			contextActionOption{icon: "✎", label: "Edit", action: modal.ContextActionLabEdit},
			contextActionOption{icon: "▣", label: "Archive", action: modal.ContextActionLabArchive},
			contextActionOption{icon: "!", label: "Delete", action: modal.ContextActionLabDelete},
		)
	default:
		actions = append(actions,
			contextActionOption{icon: "▣", label: "Archive", action: modal.ContextActionLabArchive},
		)
	}
	return append(actions, capture)
}

// handleLabAction runs a Lab action from the Actions panel.
func (m *Model) handleLabAction(action string) (tea.Model, tea.Cmd) {
	if action == modal.ContextActionLabCapture {
		m.openLabCapture()
		return m, nil
	}
	e, ok := m.lab.selected()
	if !ok {
		m.statusErr = "No Lab entry selected"
		return m, clearErrorCmd()
	}
	if m.lab.page != nil {
		// An action chosen from the entry page returns the keyboard to it.
		m.focused = panelList
	}
	switch action {
	case modal.ContextActionLabFinishInterview:
		return m.handleLabRequestSubmitted(modal.LabRequestSubmittedMsg{EntryID: e.ID, Kind: domain.LabRequestFinishInterview})
	case modal.ContextActionLabViewAgent:
		if run, ok := m.lab.latestRun(e); ok && run.live() {
			return m.openLabRunPane(firstNonEmptyString(run.workflow.RunID, run.workflow.WorkflowID))
		}
		m.statusErr = "The session is not running"
		return m, clearErrorCmd()
	case modal.ContextActionLabInspect:
		return m.openLabInspector()
	case modal.ContextActionLabOpenIssue:
		return m.openLabIssue(e)
	case modal.ContextActionLabClearLock:
		return m.clearLabLock(e)
	case modal.ContextActionLabShape:
		return m.startLabSession(e, domain.LabModeShape)
	case modal.ContextActionLabGrill, modal.ContextActionLabEscalate:
		return m.startLabSession(e, domain.LabModeGrill)
	case modal.ContextActionLabPublish:
		return m.prepareLabPublish(e)
	case modal.ContextActionLabEnd:
		return m.endLabSession(e)
	case modal.ContextActionLabEdit:
		if !e.Editable() {
			m.statusErr = "Only drafts can be edited"
			return m, clearErrorCmd()
		}
		m.activeModal = modal.NewLabEditModal(e)
		return m, nil
	case modal.ContextActionLabArchive, modal.ContextActionLabRestore:
		archive := action == modal.ContextActionLabArchive
		if archive && m.lab.hasLiveSession(e) {
			m.activeModal = modal.NewLabArchiveModal(e)
			return m, nil
		}
		e.Archived = archive
		e.Updated = time.Now().UTC()
		verb := "Restored"
		if archive {
			verb = "Archived"
		}
		status := fmt.Sprintf("%s %q", verb, e.Title())
		return m, labChangeCmd(m.RepoPath, e.ID, status, func(s *data.LabStore) error { return s.Put(e) })
	case modal.ContextActionLabDelete:
		if m.lab.hasLiveSession(e) {
			m.statusErr = "End the entry's session before deleting it"
			return m, clearErrorCmd()
		}
		if !e.Deletable() {
			m.statusErr = "Only drafts and archived entries can be deleted — archive it first"
			return m, clearErrorCmd()
		}
		m.activeModal = modal.NewLabDeleteModal(e)
		return m, nil
	}
	m.statusErr = fmt.Sprintf("Unknown action: %s", action)
	return m, clearErrorCmd()
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// labInspectorState builds what the inspector shows for e.
func (m *Model) labInspectorState(e domain.LabEntry) modal.LabInspectorState {
	state := labStateOf(m.lab, e)
	var runs []modal.LabInspectorRun
	for _, run := range m.lab.entryRuns(e) {
		runs = append(runs, modal.LabInspectorRun{Workflow: run.workflow, PaneID: run.paneID})
	}
	return modal.LabInspectorState{
		Entry:           e,
		Badge:           state.badge,
		Attention:       state.tone == labToneAttention,
		StageLabel:      labStageLabel(m.lab, e),
		PublishRequires: labPublishRequires(e),
		Runs:            runs,
	}
}

// openLabInspector opens the inspector on the selected entry.
func (m *Model) openLabInspector() (tea.Model, tea.Cmd) {
	e, ok := m.lab.selected()
	if !ok {
		m.statusErr = "No Lab entry selected"
		return m, clearErrorCmd()
	}
	inspector := modal.NewLabInspectorModal(m.labInspectorState(e))
	m.activeModal = inspector
	return m, inspector.Init()
}

// refreshLabInspector updates an open inspector after its entry or runs
// changed. An inspector whose entry was deleted is closed.
func (m *Model) refreshLabInspector() {
	inspector, ok := m.activeModal.(*modal.LabInspectorModal)
	if !ok {
		return
	}
	e, ok := m.lab.entry(inspector.EntryID())
	if !ok {
		m.activeModal = nil
		return
	}
	inspector.SetState(m.labInspectorState(e))
}

// loadLabArtifactsCmd reads an entry's drafted artifacts.
func loadLabArtifactsCmd(repoPath, id string) tea.Cmd {
	return func() tea.Msg {
		store, err := labStoreFor(repoPath)
		if err != nil {
			return modal.LabArtifactsLoadedMsg{EntryID: id, Err: err}
		}
		artifacts, err := store.Artifacts(id)
		return modal.LabArtifactsLoadedMsg{EntryID: id, Artifacts: artifacts, Err: err}
	}
}

// openLabRunPane focuses the Herdr pane of a run's agent.
func (m *Model) openLabRunPane(runID string) (tea.Model, tea.Cmd) {
	run, ok := m.lab.runs[runID]
	if !ok || run.paneID == "" {
		m.statusErr = "The session's pane is no longer available"
		return m, clearErrorCmd()
	}
	return m, m.focusPaneCmd(run.paneID, firstNonEmptyString(run.workflow.Title, "Lab session"))
}

// labSessionStartedMsg reports a shape or grill session start.
type labSessionStartedMsg struct {
	workflow domain.WorkflowRunRef
	loaded   labsLoadedMsg
}

// labProcessAlive reports whether a process on this host is running, for
// taking over a Lab entry lock its owner left behind. Tests replace it.
var labProcessAlive = pidAlive

// canShape reports whether a shape session can start for e: a bug that is a
// draft, or one being shaped whose session has ended.
func (v labView) canShape(e domain.LabEntry) bool {
	if e.Archived || e.Kind != domain.LabKindBug || e.Mode == domain.LabModeGrill {
		return false
	}
	switch e.Status {
	case domain.LabStatusDraft:
		return true
	case domain.LabStatusShaping:
		return !v.hasLiveSession(e)
	}
	return false
}

// canGrill reports whether a grill session can start for e: an idea that is a
// draft, or one being grilled whose session has ended.
func (v labView) canGrill(e domain.LabEntry) bool {
	if e.Archived {
		return false
	}
	if e.Kind != domain.LabKindIdea && e.Mode != domain.LabModeGrill {
		return false
	}
	switch e.Status {
	case domain.LabStatusDraft:
		return true
	case domain.LabStatusGrilling, domain.LabStatusSpecced, domain.LabStatusTicketed:
		return !v.hasLiveSession(e)
	}
	return false
}

// labSessionVerbs names a session mode in messages.
var labSessionVerbs = map[domain.LabMode]string{
	domain.LabModeShape: "shaping",
	domain.LabModeGrill: "grilling",
}

// startLabSession starts a shape or grill session for e. The entry's lock is
// held while the run starts and is recorded, so a second Grove cannot start
// another.
func (m *Model) startLabSession(e domain.LabEntry, mode domain.LabMode) (tea.Model, tea.Cmd) {
	verb := labSessionVerbs[mode]
	escalating := mode == domain.LabModeGrill && m.lab.canEscalate(e)
	allowed := (mode == domain.LabModeShape && m.lab.canShape(e)) || (mode == domain.LabModeGrill && (m.lab.canGrill(e) || escalating))
	if !allowed {
		m.statusErr = fmt.Sprintf("A %s session cannot start for this entry now", verb)
		return m, clearErrorCmd()
	}
	if !m.Config.Sandcastle.Enabled {
		m.statusErr = "Sandcastle is disabled; enable it in settings to start Lab sessions"
		return m, clearErrorCmd()
	}
	starter := m.workflowStarter
	repoPath := m.RepoPath
	agent := m.Config.Sandcastle.Agent()
	mapTokens := m.Config.Lab.RepoMapTokens
	m.statusMsg = fmt.Sprintf("Starting a %s session for %q…", verb, e.Title())
	return m, func() tea.Msg {
		fail := func(err error) tea.Msg { return labSessionStartedMsg{loaded: labsLoadedMsg{err: err}} }
		if starter == nil {
			return fail(fmt.Errorf("the Sandcastle runtime is unavailable"))
		}
		store, err := labStoreFor(repoPath)
		if err != nil {
			return fail(err)
		}
		release, err := store.LockEntry(e.ID, labProcessAlive)
		if err != nil {
			return fail(err)
		}
		defer release()

		// Every stage of the session reads the repository map for the
		// checkout's commit. Without one the agent explores unaided, so a
		// failure to build it does not stop the session.
		if err := ensureLabRepoMap(store, repoPath, mapTokens); err != nil {
			slog.Warn("lab: could not build the repository map", "err", err)
		}

		// Escalating hands the entry from its shape session to a grill. The
		// shape run is told to close first; its close file is its own, so the
		// grill that starts next cannot undo it.
		if escalating && len(e.Runs) > 0 {
			if err := store.CloseSession(e.ID, e.Runs[len(e.Runs)-1]); err != nil {
				return fail(err)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		workflow, err := starter.StartWorkflow(ctx, sandcastle.StartWorkflowRequest{
			Kind:      string(mode),
			RepoPath:  repoPath,
			EntryID:   e.ID,
			AgentKind: agent,
			Source:    "grove",
		})
		if err != nil {
			return fail(fmt.Errorf("start the %s session: %w", verb, err))
		}
		e.Mode = mode
		switch {
		case mode == domain.LabModeShape:
			e.Status = domain.LabStatusShaping
		case e.Status == domain.LabStatusDraft || e.Status == domain.LabStatusShaping || e.Status == domain.LabStatusPublished:
			e.Status = domain.LabStatusGrilling
		}
		e.Runs = append(e.Runs, firstNonEmptyString(workflow.RunID, workflow.WorkflowID))
		e.Updated = time.Now().UTC()
		if err := store.Put(e); err != nil {
			return fail(fmt.Errorf("record the %s session: %w", verb, err))
		}
		entries, err := store.Load()
		return labSessionStartedMsg{
			workflow: workflow,
			loaded: labsLoadedMsg{
				entries:  entries,
				selectID: e.ID,
				status:   fmt.Sprintf("%s %q — its questions appear here in the Lab", strings.ToUpper(verb[:1])+verb[1:], e.Title()),
				err:      err,
			},
		}
	}
}

// handleLabSessionStarted records a started session and shows it.
func (m *Model) handleLabSessionStarted(msg labSessionStartedMsg) (tea.Model, tea.Cmd) {
	if msg.loaded.err != nil {
		m.statusMsg = ""
		return m.handleLabsLoaded(msg.loaded)
	}
	if m.sandcastleSnapshot == nil {
		m.sandcastleSnapshot = &sandcastle.Snapshot{
			Integration: domain.ExternalIntegration{Name: "sandcastle", Mode: "connected", Available: true, Enabled: true},
		}
	}
	m.sandcastleSnapshot.Workflows = append(m.sandcastleSnapshot.Workflows, msg.workflow)
	_, cmd := m.handleLabsLoaded(msg.loaded)
	cmds := []tea.Cmd{cmd, m.rebuildMissionStateCmd()}
	if m.healthChecker != nil {
		cmds = append(cmds, sandcastleSnapshotCmd(m.healthChecker))
	}
	return m, tea.Batch(cmds...)
}

// endLabSession tells e's session to close its agent pane and finish.
func (m *Model) endLabSession(e domain.LabEntry) (tea.Model, tea.Cmd) {
	if !m.lab.hasLiveSession(e) {
		m.statusErr = "This entry has no live session"
		return m, clearErrorCmd()
	}
	status := fmt.Sprintf("Ending the session for %q", e.Title())
	run, _ := m.lab.latestRun(e)
	runID := firstNonEmptyString(run.workflow.RunID, run.workflow.WorkflowID)
	return m, labChangeCmd(m.RepoPath, e.ID, status, func(s *data.LabStore) error { return s.CloseSession(e.ID, runID) })
}

// draftReady reports whether e's session has finished its draft: its live run
// is at the Publish step, or its session has ended.
func (v labView) draftReady(e domain.LabEntry) bool {
	run, ok := v.latestRun(e)
	if !ok || !run.live() {
		return true
	}
	return strings.EqualFold(run.workflow.CurrentStep, "Publish")
}

// labNextAction describes the primary action Enter performs for an entry.
// Keeping this beside labNextStep makes the list and context panel explain the
// actual workflow instead of exposing lifecycle states the user must decode.
func labNextAction(v labView, e domain.LabEntry) string {
	switch {
	case e.Archived:
		return "Restore from Actions to continue"
	case e.Status == domain.LabStatusPublished:
		if issue := labPrimaryIssue(e); issue != nil {
			return fmt.Sprintf("Open issue #%d", *issue)
		}
		return "Open published issues"
	case labPublishRequires(e) != nil && v.draftReady(e):
		if e.Mode == domain.LabModeGrill && e.Status == domain.LabStatusTicketed {
			return "Review and publish the epic"
		}
		return "Review the drafted bug report"
	}
	if action := labAskAction(v, e); action != "" {
		return action
	}
	if run, ok := v.latestRun(e); ok && run.live() && run.paneID != "" {
		if run.waiting() {
			// Waiting on something Grove has no card for.
			return "Answer the agent in its pane"
		}
		return "Watch the agent"
	}
	switch {
	case v.canShape(e):
		if e.Status == domain.LabStatusDraft {
			return "Shape this bug into a report"
		}
		return "Resume shaping the bug report"
	case v.canGrill(e):
		if e.Status == domain.LabStatusDraft {
			return "Grill this idea"
		}
		return "Resume grilling this idea"
	case e.Editable():
		return "Edit this capture"
	default:
		return "Inspect progress"
	}
}

// openLabInspectorOnArtifacts opens the inspector on the selected entry's
// Artifacts tab, for review.
func (m *Model) openLabInspectorOnArtifacts() (tea.Model, tea.Cmd) {
	updated, cmd := m.openLabInspector()
	if inspector, ok := m.activeModal.(*modal.LabInspectorModal); ok {
		inspector.ShowArtifacts()
	}
	return updated, cmd
}

// hasLiveSession reports whether e has a session that has not finished.
func (v labView) hasLiveSession(e domain.LabEntry) bool {
	run, ok := v.latestRun(e)
	return ok && run.live()
}

// grillStepStatus is the entry status a grill session has reached at each of
// its run's steps.
var grillStepStatus = map[string]domain.LabStatus{
	"Scout":     domain.LabStatusGrilling,
	"Interview": domain.LabStatusGrilling,
	"Spec":      domain.LabStatusSpecced,
	"Tickets":   domain.LabStatusTicketed,
	"Publish":   domain.LabStatusTicketed,
}

var labStatusRank = map[domain.LabStatus]int{
	domain.LabStatusDraft:     0,
	domain.LabStatusGrilling:  1,
	domain.LabStatusSpecced:   2,
	domain.LabStatusTicketed:  3,
	domain.LabStatusPublished: 4,
}

// statusAdvances returns the grilled entries whose run has reached a later
// stage than the entry records, with their status moved forward. A status
// never moves back, so a resumed session that revisits an earlier step keeps
// what was reached.
func (v labView) statusAdvances() []domain.LabEntry {
	var out []domain.LabEntry
	for _, e := range v.entries {
		if e.Mode != domain.LabModeGrill || e.Archived || e.Status == domain.LabStatusPublished {
			continue
		}
		run, ok := v.latestRun(e)
		if !ok {
			continue
		}
		next, ok := grillStepStatus[run.workflow.CurrentStep]
		if !ok || labStatusRank[next] <= labStatusRank[e.Status] {
			continue
		}
		e.Status = next
		e.Updated = time.Now().UTC()
		out = append(out, e)
	}
	return out
}

// syncLabStatusesCmd records the stages grill sessions have reached.
func (m *Model) syncLabStatusesCmd() tea.Cmd {
	advances := m.lab.statusAdvances()
	if len(advances) == 0 {
		return nil
	}
	return labChangeCmd(m.RepoPath, "", "", func(s *data.LabStore) error {
		for _, e := range advances {
			if err := s.Put(e); err != nil {
				return err
			}
		}
		return nil
	})
}

// canEscalate reports whether e is a bug that turned out bigger than one
// issue and can be taken into a full grill: one being shaped, or one already
// published, whose issue then becomes a sub-issue of the epic.
func (v labView) canEscalate(e domain.LabEntry) bool {
	if e.Archived || e.Kind != domain.LabKindBug || e.Mode != domain.LabModeShape {
		return false
	}
	return e.Status == domain.LabStatusShaping || e.Status == domain.LabStatusPublished
}

// handleLabArchiveConfirmed ends a live session and archives its entry.
func (m *Model) handleLabArchiveConfirmed(msg modal.LabArchiveConfirmedMsg) (tea.Model, tea.Cmd) {
	m.activeModal = nil
	e, ok := m.lab.entry(msg.ID)
	if !ok {
		return m, nil
	}
	var runID string
	if run, live := m.lab.latestRun(e); live && run.live() {
		runID = firstNonEmptyString(run.workflow.RunID, run.workflow.WorkflowID)
	}
	e.Archived = true
	e.Updated = time.Now().UTC()
	status := fmt.Sprintf("Ended the session and archived %q", e.Title())
	return m, labChangeCmd(m.RepoPath, e.ID, status, func(s *data.LabStore) error {
		if runID != "" {
			if err := s.CloseSession(e.ID, runID); err != nil {
				return err
			}
		}
		return s.Put(e)
	})
}

// labPrimaryIssue returns the issue a published entry is known by: its epic,
// or the single issue of a shaped bug.
func labPrimaryIssue(e domain.LabEntry) *int {
	if e.Issues.Epic != nil {
		return e.Issues.Epic
	}
	return e.Issues.Issue
}

// openLabIssue shows a published entry's epic or issue in the Issues tab,
// where an epic's sub-issues are listed beneath it.
func (m *Model) openLabIssue(e domain.LabEntry) (tea.Model, tea.Cmd) {
	num := labPrimaryIssue(e)
	if num == nil {
		m.statusErr = "This entry has no published issue"
		return m, clearErrorCmd()
	}
	for i, issue := range m.issues {
		if issue.Number == *num {
			m.view = viewIssues
			m.selectedIssueIdx = i
			m.ctxScrollOffset = 0
			m.contextActionIdx = 0
			return m, nil
		}
	}
	m.statusMsg = fmt.Sprintf("#%d is not among the synced issues yet — syncing GitHub", *num)
	return m, tea.Batch(clearMsgCmd(), m.syncGitHubCmd(true))
}

// labLocalHost is this machine's name, for telling a lock held here from one
// held by a Grove elsewhere. Tests replace it.
var labLocalHost = func() string {
	host, _ := os.Hostname()
	return host
}

// foreignLock returns e's session lock when a Grove on another host holds it.
// A lock left on this host is taken over automatically, so only another
// host's lock needs clearing by hand.
func (v labView) foreignLock(e domain.LabEntry) (data.LabLockOwner, bool) {
	owner, ok := v.locks[e.ID]
	if !ok || owner.Host == labLocalHost() {
		return data.LabLockOwner{}, false
	}
	return owner, true
}

// clearLabLock removes a lock another host's Grove left on e.
func (m *Model) clearLabLock(e domain.LabEntry) (tea.Model, tea.Cmd) {
	owner, ok := m.lab.foreignLock(e)
	if !ok {
		m.statusErr = "This entry has no lock from another machine"
		return m, clearErrorCmd()
	}
	status := fmt.Sprintf("Cleared the lock held by Grove on %s", owner.Host)
	return m, labChangeCmd(m.RepoPath, e.ID, status, func(s *data.LabStore) error { return s.ClearEntryLock(e.ID) })
}
