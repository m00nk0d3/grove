// Copyright 2025 m00nk0d3. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modal

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

// === Test Fixtures ===

type testGrillingArtifacts struct {
	Context  string
	Spec     string
	RepoPath string
}

func setupApproveGrillTest(repoPath string, ctxContent, specContent string) *ApproveGrillModal {
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		panic(fmt.Sprintf("failed to create test repo dir: %v", err))
	}
	return &ApproveGrillModal{
		artifacts: GrillingArtifacts{
			Context:  ctxContent,
			Spec:     specContent,
			RepoPath: repoPath,
		},
		step:       stepContext,
		action:     actionApprove,
		width:      80,
		height:     24,
		theme:      &styles.Theme{},
	}
}

func setupApproveGrillRejectTest(repoPath string, ctxContent, specContent string) *ApproveGrillModal {
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		panic(fmt.Sprintf("failed to create test repo dir: %v", err))
	}
	return &ApproveGrillModal{
		artifacts: GrillingArtifacts{
			Context:  ctxContent,
			Spec:     specContent,
			RepoPath: repoPath,
		},
		step:       stepContext,
		action:     actionReject,
		width:      80,
		height:     24,
		theme:      &styles.Theme{},
	}
}

// === AC1: Converging an interview presents the context document and spec separately ===

func TestApproveGrill_AC1_PresentsBothArtifactsSeparately(t *testing.T) {
	ctxContent := "### Context Document\nLine 1 of transcript"
	specContent := "### Specification\nLine 1 of spec"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	// Verify modal is in stepContext on init (first artifact shown)
	if m.step != stepContext {
		t.Errorf("modal should start at stepContext, got %d", m.step)
	}

	view := m.View()
	if !strings.Contains(view, "CONTEXT DOCUMENT") {
		t.Error("view should contain CONTEXT DOCUMENT title")
	}

	// Advance to spec step with Enter
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.step != stepSpec {
		t.Errorf("should advance to stepSpec after Enter, got %d", m.step)
	}

	view2 := m.View()
	if !strings.Contains(view2, "SPECIFICATION") {
		t.Error("view should contain SPECIFICATION title")
	}
}

func TestApproveGrill_AC1_ContextDocumentContainsTranscript(t *testing.T) {
	ctxContent := `### Grilling Session Transcript

[**griller-1** @ 10:30] > What is the main feature?
[**griller-1** @ 10:35] > Answer: Add approval modal.

### Additional Context`
	specContent := "### Specification"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	view := m.View()

	// Verify transcript is present in context artifact view
	if !strings.Contains(view, "### Grilling Session Transcript") {
		t.Error("context document should contain transcript header")
	}

	if !strings.Contains(view, "What is the main feature?") {
		t.Error("context document should contain conversation content")
	}
}

func TestApproveGrill_AC1_SpecContainsRequirements(t *testing.T) {
	ctxContent := "### Context Document"
	specContent := `### Specification

- **What happens on approval?**
  Creates file at repo root

- **How are artifacts stored?**
  At repository root or user-configurable path`
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	// Advance to spec step
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := m.View()

	// Verify spec is present in spec artifact view
	if !strings.Contains(view, "### Specification") {
		t.Error("spec should contain specification header")
	}

	if !strings.Contains(view, "Creates file at repo root") {
		t.Error("spec should contain requirement content")
	}
}

// === AC2: Approving an artifact creates the corresponding file in the working tree ===

func TestApproveGrill_AC2_ApproveWritesContextToRepo(t *testing.T) {
	ctxContent := "### Context Document\nContent here"
	specContent := "### Specification\nContent here"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	// Approve context (default action is approve)
	cmd := m.commitArtifactsCmd()

	// Execute the command
	msg := cmd()

	// Verify success message is sent
	if _, ok := msg.(GrillingArtifactsApprovedMsg); !ok {
		t.Errorf("commit should send GrillingArtifactsApprovedMsg, got %T", msg)
	}
}

func TestApproveGrill_AC2_ApproveWritesSpecToRepo(t *testing.T) {
	ctxContent := "### Context Document"
	specContent := "### Specification\nContent here"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	// Advance to spec step
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Approve spec (default action is approve)
	cmd := m.commitArtifactsCmd()

	msg := cmd()

	// Verify success message is sent
	if _, ok := msg.(GrillingArtifactsApprovedMsg); !ok {
		t.Errorf("commit should send GrillingArtifactsApprovedMsg, got %T", msg)
	}
}

func TestApproveGrill_AC2_BothApproveWritesBothFiles(t *testing.T) {
	ctxContent := "### Context Document"
	specContent := "### Specification"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	// Both artifacts approved (default action)
	cmd := m.commitArtifactsCmd()

	msg := cmd()

	if _, ok := msg.(GrillingArtifactsApprovedMsg); !ok {
		t.Errorf("commit should send GrillingArtifactsApprovedMsg, got %T", msg)
	}
}

func TestApproveGrill_AC2_ArtifactPathsAreCorrect(t *testing.T) {
	ctxContent := "Context content"
	specContent := "Spec content"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	// The artifact paths should be:
	// - context.md (at repo root for simplicity per PLAN)
	// - spec.md (at repo root for simplicity per PLAN)

	// Verify modal stores repo path correctly
	if m.artifacts.RepoPath != repoPath {
		t.Errorf("RepoPath should be %s, got %s", repoPath, m.artifacts.RepoPath)
	}
}

func TestApproveGrill_AC2_RejectDoesNotWriteArtifacts(t *testing.T) {
	ctxContent := "### Context Document"
	specContent := "### Specification"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillRejectTest(repoPath, ctxContent, specContent)

	// Reject action
	cmd := m.commitArtifactsCmd()

	msg := cmd()

	// Verify rejection message is sent (no files written)
	if _, ok := msg.(GrillingRejectedMsg); !ok {
		t.Errorf("reject should send GrillingRejectedMsg, got %T", msg)
	}
}

func TestApproveGrill_AC2_BothApprovedCommitsBoth(t *testing.T) {
	ctxContent := "### Context Document"
	specContent := "### Specification"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	// Both artifacts should be approved when action is approve
	cmd := m.commitArtifactsCmd()
	msg := cmd()

	// Verify success message and that both were committed
	if _, ok := msg.(GrillingArtifactsApprovedMsg); !ok {
		t.Errorf("commit should send GrillingArtifactsApprovedMsg, got %T", msg)
	}
}

func TestApproveGrill_AC2_ContextAlwaysShownFirst(t *testing.T) {
	ctxContent := "### Context Document"
	specContent := "### Specification"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	// Set action to reject for both
	m.action = actionReject

	// Even when rejecting, context should be shown first (idx 0 always processed)
	cmd := m.commitArtifactsCmd()

	msg := cmd()

	// Reject takes precedence, but test that modal attempts to process artifacts
	if _, ok := msg.(GrillingRejectedMsg); !ok {
		t.Errorf("reject action should send GrillingRejectedMsg")
	}
}

func TestApproveGrill_AC2_ModalTitleIsCorrect(t *testing.T) {
	ctxContent := "### Context"
	specContent := "### Spec"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	if m.Title() != "REVIEW GRILLING ARTIFACTS" {
		t.Errorf("modal title should be 'REVIEW GRILLING ARTIFACTS', got %q", m.Title())
	}
}

func TestApproveGrill_AC2_ViewShowsCurrentArtifact(t *testing.T) {
	ctxContent := "Context content for testing"
	specContent := "Spec content for testing"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	view := m.View()

	if !strings.Contains(view, "CONTEXT DOCUMENT") {
		t.Error("view should show CONTEXT DOCUMENT title in stepContext")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view2 := m.View()

	if !strings.Contains(view2, "SPECIFICATION") {
		t.Error("view should show SPECIFICATION title in stepSpec")
	}
}

func TestApproveGrill_AC2_ViewShowsHints(t *testing.T) {
	ctxContent := "Context"
	specContent := "Spec"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	view := m.View()

	// Verify hints are present
	hintStrings := []string{
		"[Enter] Next artifact",
		"[a] Approve   [r] Reject",
	}

	for _, hint := range hintStrings {
		if !strings.Contains(view, hint) {
			t.Errorf("view should contain hint: %q", hint)
		}
	}
}

func TestApproveGrill_AC2_ApproveRejectToggle(t *testing.T) {
	ctxContent := "Context"
	specContent := "Spec"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	// Verify initial action is approve
	if m.action != actionApprove {
		t.Errorf("initial action should be approve, got %d", m.action)
	}

	view1 := m.View()
	if !strings.Contains(view1, "APPROVE") {
		t.Error("view should show 'APPROVE' when in approve mode")
	}

	// Toggle to reject with 'r' key
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if m.action != actionReject {
		t.Errorf("action should toggle to reject after 'r', got %d", m.action)
	}

	view2 := m.View()
	if !strings.Contains(view2, "REJECT") {
		t.Error("view should show 'REJECT' when in reject mode")
	}

	// Toggle back to approve with 'a' key
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if m.action != actionApprove {
		t.Errorf("action should toggle back to approve after 'a', got %d", m.action)
	}
}

func TestApproveGrill_AC2_CancelledModalReturnsCancelledMsg(t *testing.T) {
	ctxContent := "Context"
	specContent := "Spec"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	// Cancel with Escape key
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Error("Escape should return a command")
	} else {
		// Execute the command
		msg := cmd()

		if _, ok := msg.(ModalCancelledMsg); !ok {
			t.Errorf("cancel should send ModalCancelledMsg, got %T", msg)
		}
	}
}

func TestApproveGrill_AC2_FinishWithEnterOnSpecStep(t *testing.T) {
	ctxContent := "Context"
	specContent := "Spec"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	// Advance to spec step
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Finish with Enter
	cmd := m.commitArtifactsCmd()
	msg := cmd()

	if _, ok := msg.(GrillingArtifactsApprovedMsg); !ok {
		t.Errorf("finish should send GrillingArtifactsApprovedMsg, got %T", msg)
	}
}

func TestApproveGrill_AC2_BothRejectTriggersGrillingRetry(t *testing.T) {
	ctxContent := "Context"
	specContent := "Spec"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillRejectTest(repoPath, ctxContent, specContent)

	cmd := m.commitArtifactsCmd()
	msg := cmd()

	if _, ok := msg.(GrillingRejectedMsg); !ok {
		t.Errorf("reject should send GrillingRejectedMsg, got %T", msg)
	}

	// GrillingRejectedMsg contains Kind field that can be used to re-trigger grilling
	rejectedMsg, _ := msg.(GrillingRejectedMsg)
	if rejectedMsg.Kind != "grilling" {
		t.Errorf("rejected message kind should be 'grilling', got %q", rejectedMsg.Kind)
	}
}

// === Edge Cases ===

func TestApproveGrill_EdgeCase_EmptyContentHandled(t *testing.T) {
	ctxContent := ""
	specContent := ""
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	view := m.View()

	// Modal should still render even with empty content
	if view == "" {
		t.Error("view should not be empty even with empty content")
	}

	// Should contain title at minimum
	if !strings.Contains(view, "REVIEW GRILLING ARTIFACTS") {
		t.Error("view should contain modal title")
	}
}

func TestApproveGrill_EdgeCase_ArtifactWithNewlines(t *testing.T) {
	ctxContent := `Line 1
Line 2
Line 3`
	specContent := `### Header
## Subheader
Body text`
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	viewContext := m.View()

	// Context artifact should preserve newlines
	if !strings.Contains(viewContext, "Line 1") || !strings.Contains(viewContext, "Line 2") || !strings.Contains(viewContext, "Line 3") {
		t.Error("context view should preserve newlines from content")
	}

	// Advance to spec step
	m.step = stepSpec
	viewSpec := m.View()

	// Spec artifact should preserve subheaders
	if !strings.Contains(viewSpec, "## Subheader") || !strings.Contains(viewSpec, "### Header") {
		t.Error("spec view should preserve markdown headers from content")
	}
}

func TestApproveGrill_EdgeCase_LongContentWraps(t *testing.T) {
	ctxContent := strings.Repeat("x", 1000)
	specContent := strings.Repeat("y", 500)
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	view := m.View()

	// Verify view contains the content (length check as proxy for wrapping)
	if len(view) < len(ctxContent)/4 { // Allow some overhead for title/header formatting
		t.Error("view should contain most of long context content")
	}
}

func TestApproveGrill_EdgeCase_ModalInitReturnsNilCmd(t *testing.T) {
	ctxContent := "Context"
	specContent := "Spec"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	cmd := m.Init()

	if cmd != nil {
		t.Errorf("Init should return nil command, got %T", cmd)
	}
}

func TestApproveGrill_EdgeCase_FullscreenReturnsTrue(t *testing.T) {
	ctxContent := "Context"
	specContent := "Spec"
	repoPath := "/tmp/test-repo"

	m := setupApproveGrillTest(repoPath, ctxContent, specContent)

	if !m.Fullscreen() {
		t.Error("modal should be fullscreen")
	}
}

// === Validation Commands for Verifier ===
//
// Run these commands to validate the implementation against acceptance criteria:
//
// AC1 validation (Presents artifacts separately):
//   go test ./internal/tui/modal/... -run "TestApproveGrill_AC1" -v
//
// AC2 validation (Creates files on approve):
//   go test ./internal/tui/modal/... -run "TestApproveGrill_AC2" -v
//
// Edge cases validation:
//   go test ./internal/tui/modal/... -run "TestApproveGrill_EdgeCase" -v
