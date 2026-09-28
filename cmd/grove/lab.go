package main

import (
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
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
}

func newLabView() labView {
	return labView{filter: domain.LabFilterAll}
}

// visibleIn returns the entries in tab that match the kind filter, newest
// first.
func (v labView) visibleIn(tab labTab) []domain.LabEntry {
	var out []domain.LabEntry
	for _, e := range v.entries {
		if labTabOf(e) == tab && v.filter.Matches(e) {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
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
	if e.Editable() {
		m.activeModal = modal.NewLabEditModal(e)
		return m, nil
	}
	return m, nil
}

// labContextActions lists what the Actions panel offers for the selected
// entry, which depends on its state.
func labContextActions(e domain.LabEntry, ok bool) []contextActionOption {
	capture := contextActionOption{icon: "+", label: "Capture new entry", action: modal.ContextActionLabCapture}
	if !ok {
		return []contextActionOption{capture}
	}
	var actions []contextActionOption
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
