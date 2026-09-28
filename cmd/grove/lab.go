package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/sandcastle"
	"github.com/m00nk0d3/grove/internal/tui/modal"
)

// labTab is one of the Lab list's lifecycle tabs.
type labTab int

const (
	labTabActive labTab = iota
	labTabDrafts
	labTabPublished
	labTabArchived
	labTabCount
)

var labTabLabels = [labTabCount]string{"ACTIVE / ATTENTION", "DRAFTS", "PUBLISHED", "ARCHIVED"}

// labTabOf returns the tab that lists e.
func labTabOf(e domain.LabEntry) labTab {
	switch {
	case e.Archived:
		return labTabArchived
	case e.Status == domain.LabStatusDraft:
		return labTabDrafts
	case e.Status == domain.LabStatusPublished:
		return labTabPublished
	default:
		return labTabActive
	}
}

// labView is the Lab tab's state: every entry of the repository and what the
// list shows of them. The cursor indexes visible(), so the entry drawn as
// selected is the entry acted on.
type labView struct {
	entries []domain.LabEntry
	tab     labTab
	filter  domain.LabFilter
	cursor  int
	// runs holds the Sandcastle runs of the repository by run ID, refreshed
	// with mission control state. An entry's session state is read from its
	// runs here, never stored on the entry.
	runs map[string]labRun
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
	case domain.WorkflowQueued, domain.WorkflowRunning, domain.WorkflowBlocked:
		return true
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
	for _, workflow := range state.WorkflowRuns {
		id := firstNonEmptyString(workflow.RunID, workflow.WorkflowID)
		if id == "" {
			continue
		}
		run := labRun{workflow: workflow}
		for _, agent := range agentsForWorkflow(state, id) {
			if agent.PaneID != "" {
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

// visibleIn returns the entries in tab that match the kind filter: entries
// waiting for the user first, then newest first.
func (v labView) visibleIn(tab labTab) []domain.LabEntry {
	var out []domain.LabEntry
	for _, e := range v.entries {
		if labTabOf(e) == tab && v.filter.Matches(e) {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if wi, wj := v.waiting(out[i]), v.waiting(out[j]); wi != wj {
			return wi
		}
		return out[i].Created.After(out[j].Created)
	})
	return out
}

// visible returns the entries the list shows.
func (v labView) visible() []domain.LabEntry { return v.visibleIn(v.tab) }

// selected returns the entry under the cursor.
func (v labView) selected() (domain.LabEntry, bool) {
	visible := v.visible()
	if v.cursor < 0 || v.cursor >= len(visible) {
		return domain.LabEntry{}, false
	}
	return visible[v.cursor], true
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
	n := len(v.visible())
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

func (v *labView) setTab(tab labTab) {
	v.tab = (tab%labTabCount + labTabCount) % labTabCount
	v.cursor = 0
}

func (v *labView) setFilter(f domain.LabFilter) {
	v.filter = f
	v.cursor = 0
}

// selectID moves to the tab listing the entry with the given ID and puts the
// cursor on it. The kind filter is cleared when it would hide the entry.
func (v *labView) selectID(id string) {
	e, ok := v.entry(id)
	if !ok {
		v.clamp()
		return
	}
	if !v.filter.Matches(e) {
		v.filter = domain.LabFilterAll
	}
	v.tab = labTabOf(e)
	for i, visible := range v.visible() {
		if visible.ID == id {
			v.cursor = i
			return
		}
	}
}

// labsLoadedMsg carries the repository's entries after a load or a change.
// selectID, when set, is the entry the list should move to; status is the
// message to show.
type labsLoadedMsg struct {
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
		return labsLoadedMsg{entries: entries, selectID: selectID, status: status, err: err}
	}
}

// handleLabsLoaded applies a load or change result.
func (m *Model) handleLabsLoaded(msg labsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.statusErr = msg.err.Error()
		return m, clearErrorCmd()
	}
	m.lab.entries = msg.entries
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
	e, ok := m.lab.selected()
	if !ok {
		m.openLabCapture()
		return m, nil
	}
	if e.Archived {
		m.statusMsg = "Archived — restore it from the Actions panel to continue"
		return m, clearMsgCmd()
	}
	// A draft waiting for review is reviewed in Grove, not in the pane.
	if labPublishRequires(e) != nil && m.lab.draftReady(e) {
		return m.openLabInspectorOnArtifacts()
	}
	if run, ok := m.lab.latestRun(e); ok && run.live() && run.paneID != "" {
		return m.openLabRunPane(firstNonEmptyString(run.workflow.RunID, run.workflow.WorkflowID))
	}
	if m.lab.canShape(e) {
		return m.startLabShape(e)
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
	actions = append(actions, contextActionOption{icon: "◎", label: "Inspect", action: modal.ContextActionLabInspect})
	if v.hasLiveSession(e) {
		actions = append(actions, contextActionOption{icon: "■", label: "End session", action: modal.ContextActionLabEnd})
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
	switch action {
	case modal.ContextActionLabInspect:
		return m.openLabInspector()
	case modal.ContextActionLabShape:
		return m.startLabShape(e)
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
		e.Archived = archive
		e.Updated = time.Now().UTC()
		verb := "Restored"
		if archive {
			verb = "Archived"
		}
		status := fmt.Sprintf("%s %q", verb, e.Title())
		return m, labChangeCmd(m.RepoPath, e.ID, status, func(s *data.LabStore) error { return s.Put(e) })
	case modal.ContextActionLabDelete:
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
	if e.Archived || e.Kind != domain.LabKindBug {
		return false
	}
	if e.Status == domain.LabStatusDraft {
		return true
	}
	if e.Status != domain.LabStatusShaping {
		return false
	}
	run, ok := v.latestRun(e)
	return !ok || !run.live()
}

// hasLiveSession reports whether e has a session that has not finished.
func (v labView) hasLiveSession(e domain.LabEntry) bool {
	run, ok := v.latestRun(e)
	return ok && run.live()
}

// startLabShape starts a shape session for e. The entry's lock is held while
// the run starts and is recorded, so a second Grove cannot start another.
func (m *Model) startLabShape(e domain.LabEntry) (tea.Model, tea.Cmd) {
	if !m.lab.canShape(e) {
		m.statusErr = "Only a bug draft, or a bug whose shaping session has ended, can be shaped"
		return m, clearErrorCmd()
	}
	if !m.Config.Sandcastle.Enabled {
		m.statusErr = "Sandcastle is disabled; enable it in settings to shape entries"
		return m, clearErrorCmd()
	}
	starter := m.workflowStarter
	repoPath := m.RepoPath
	agent := m.Config.Sandcastle.DefaultAgent
	m.statusMsg = fmt.Sprintf("Starting a shaping session for %q…", e.Title())
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

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		workflow, err := starter.StartWorkflow(ctx, sandcastle.StartWorkflowRequest{
			Kind:      string(domain.LabModeShape),
			RepoPath:  repoPath,
			EntryID:   e.ID,
			AgentKind: agent,
			Source:    "grove",
		})
		if err != nil {
			return fail(fmt.Errorf("start the shaping session: %w", err))
		}
		e.Mode = domain.LabModeShape
		e.Status = domain.LabStatusShaping
		e.Runs = append(e.Runs, firstNonEmptyString(workflow.RunID, workflow.WorkflowID))
		e.Updated = time.Now().UTC()
		if err := store.Put(e); err != nil {
			return fail(fmt.Errorf("record the shaping session: %w", err))
		}
		entries, err := store.Load()
		return labSessionStartedMsg{
			workflow: workflow,
			loaded: labsLoadedMsg{
				entries:  entries,
				selectID: e.ID,
				status:   fmt.Sprintf("Shaping %q — answer the agent in its Herdr pane", e.Title()),
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
	return m, labChangeCmd(m.RepoPath, e.ID, status, func(s *data.LabStore) error { return s.CloseSession(e.ID) })
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

// openLabInspectorOnArtifacts opens the inspector on the selected entry's
// Artifacts tab, for review.
func (m *Model) openLabInspectorOnArtifacts() (tea.Model, tea.Cmd) {
	updated, cmd := m.openLabInspector()
	if inspector, ok := m.activeModal.(*modal.LabInspectorModal); ok {
		inspector.ShowArtifacts()
	}
	return updated, cmd
}
