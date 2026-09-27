// Copyright 2025 m00nk0d3. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modal

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

// GrillingArtifactsRequestedMsg is dispatched when grilling workflow completes
// and artifacts are ready for review.

const (
	artifactContext = "context.md"
	artifactSpec    = "spec.md"
)

type approveGrillStep int

const (
	stepContext approveGrillStep = iota
	stepSpec
)

type GrillingArtifactAction int

const (
	actionApprove GrillingArtifactAction = iota
	actionReject
)

// NewApproveGrillModal creates a new approval modal for reviewing grilling artifacts.
func NewApproveGrillModal(artifacts GrillingArtifacts) *ApproveGrillModal {
	return &ApproveGrillModal{
		artifacts:  artifacts,
		step:       stepContext,
		action:     actionApprove,
		width:      0,
		height:     0,
		theme:      nil,
		contextIdx: 0,
	}
}

// ApproveGrillModal handles the multi-step approval flow for grilling artifacts.
type ApproveGrillModal struct {
	artifacts   GrillingArtifacts
	step        approveGrillStep
	action      GrillingArtifactAction
	width       int
	height      int
	theme       *styles.Theme
	contextIdx  int
}

// Init satisfies tea.Model.
func (m *ApproveGrillModal) Init() tea.Cmd {
	return nil
}

// Title returns the modal title for themed rendering.
func (m *ApproveGrillModal) Title() string {
	return "REVIEW GRILLING ARTIFACTS"
}

// Fullscreen returns true as this modal should cover the terminal.
func (m *ApproveGrillModal) Fullscreen() bool {
	return true
}

// View renders the approval modal content showing current artifact.
func (m *ApproveGrillModal) View() string {
	m.setDimensions()

	var content string
	switch m.step {
	case stepContext:
		content = m.formatArtifact("CONTEXT DOCUMENT", m.artifacts.Context, false)
	case stepSpec:
		content = m.formatArtifact("SPECIFICATION", m.artifacts.Spec, true)
	default:
		content = "Loading…"
	}

	action := "APPROVE"
	if m.action == actionReject {
		action = "REJECT"
	}

	var hints []string
	if m.step == stepContext {
		hints = []string{
			"[Enter] Next artifact",
			"[a] Approve   [r] Reject",
		}
	} else {
		hints = []string{
			"[Enter] Finish",
			"[a] Both approve   [r] Both reject",
		}
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Padding(0, 1).Render(m.Title()))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", m.width))
	b.WriteString("\n\n")
	b.WriteString(content)
	b.WriteString("\n\n")
	b.WriteString(action + " this artifact")

	for _, hint := range hints {
		b.WriteString("\n" + hint)
	}

	b.WriteString("\n\n[ESC/q] Cancel")

	return b.String()
}

// setDimensions ensures the modal has valid dimensions.
func (m *ApproveGrillModal) setDimensions() {
	if m.width <= 0 {
		m.width = 80
	}
	if m.height <= 0 {
		m.height = 24
	}
}

// formatArtifact formats the artifact content for display.
func (m *ApproveGrillModal) formatArtifact(title string, content string, isSpec bool) string {
	headerStyle := lipgloss.NewStyle().Padding(2, 1).Bold(true)
	if isSpec {
		headerStyle = headerStyle.Foreground(lipgloss.Color("#ff0"))
	} else {
		headerStyle = headerStyle.Foreground(lipgloss.Color("#0f0"))
	}

	header := headerStyle.Render(title + ":")

	return header + "\n\n" + content
}

// Update handles input for the approval modal.
func (m *ApproveGrillModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		return m, func() tea.Msg { return ModalCancelledMsg{} }

	case tea.KeyEnter:
		if m.step == stepContext {
			m.contextIdx++
			m.action = actionApprove
			m.step = stepSpec
		} else {
			return m, m.commitArtifactsCmd()
		}

	case tea.KeyTab:
		if m.step == stepContext {
			m.contextIdx++
			m.action = actionApprove
			m.step = stepSpec
		} else {
			return m, m.commitArtifactsCmd()
		}

	case tea.KeyCtrlN:
		return m, func() tea.Msg { return ModalCancelledMsg{} }

	case tea.KeyRunes:
		switch key.String() {
		case "a":
			if m.action == actionReject {
				m.action = actionApprove
			} else {
				m.action = actionApprove
			}
		case "r":
			if m.action == actionApprove {
				m.action = actionReject
			}
		}
	}

	return m, nil
}

// commitArtifactsCmd commits approved artifacts or triggers retry on rejection.
func (m *ApproveGrillModal) commitArtifactsCmd() tea.Cmd {
	return func() tea.Msg {
		repoPath := m.artifacts.RepoPath

		if m.action == actionApprove {
			err := writeArtifact(repoPath, artifactContext, m.artifacts.Context)
			if err != nil {
				return GrillingArtifactsCommitErr{Error: err}
			}

			err = writeArtifact(repoPath, artifactSpec, m.artifacts.Spec)
			if err != nil {
				return GrillingArtifactsCommitErr{Error: err}
			}

			return GrillingArtifactsApprovedMsg{}
		}

		return GrillingRejectedMsg{Kind: "grilling"}
	}
}

// writeArtifact writes the artifact to disk at repo root.
func writeArtifact(repoPath string, name string, content string) error {
	path := filepath.Join(repoPath, name+".md")
	return os.WriteFile(path, []byte(content), 0o644)
}
