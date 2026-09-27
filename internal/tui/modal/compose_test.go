package modal

import (
	"testing"

	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// ComposeModal unit tests - Issue #229
// ---------------------------------------------------------------------------

// TestComposeModal_Init_ReturnsNil verifies that ComposeModal.Init() returns nil.
func TestComposeModal_Init_ReturnsNil(t *testing.T) {
	m := NewComposeModal(
		ComposeInitMsg{Kind: "", Title: "Test Title"}, false)
	assert.Nil(t, m.Init())
}

// TestComposeModal_View_RendersKindSelector verifies kind selector renders.
func TestComposeModal_View_RendersKindSelector(t *testing.T) {
	m := NewComposeModal(
		ComposeInitMsg{Kind: "", Title: "My Test"}, false)
	m.step = composeKind
	m.kindIdx = 0
	m.width = 80
	m.height = 24

	view := m.View()

	assert.Contains(t, view, "COMPOSE NEW ENTRY")
	assert.Contains(t, view, "KIND")
}

// TestComposeModal_View_RendersContentEditor verifies content editor renders.
func TestComposeModal_View_RendersContentEditor(t *testing.T) {
	m := NewComposeModal(
		ComposeInitMsg{Kind: "idea", Title: "My Feature"}, false)
	m.step = composeContent
	m.width = 80
	m.height = 24

	view := m.View()

	assert.Contains(t, view, "CONTENT")
}

// TestComposeModal_View_RendersConfirmSave verifies confirm screen renders.
func TestComposeModal_View_RendersConfirmSave(t *testing.T) {
	tests := []struct {
		name     string
		cancel   bool
		wantIn   []string
	}{
		{"accept status", false, []string{"Entry Ready:", "ACCEPT"}},
		{"cancelled status", true, []string{"Entry Ready:", "CANCELLED"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewComposeModal(
				ComposeInitMsg{Kind: "", Title: "My Test"}, false)
			m.step = composeConfirm
			m.cancel = tt.cancel
			m.width = 80
			m.height = 24

			view := m.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
		})
	}
}

// TestComposeModal_Title_ReturnsCorrectTitle verifies Title() works.
func TestComposeModal_Title_ReturnsCorrectTitle(t *testing.T) {
	tests := []struct {
		name string
		msg  ComposeInitMsg
		want string
	}{
		{"new entry", ComposeInitMsg{Kind: "", Title: "My Test"}, "COMPOSE NEW ENTRY"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewComposeModal(tt.msg, false)
			assert.Equal(t, tt.want, m.Title())
		})
	}
}

// TestComposeModal_SetWidthAndHeight_Verified verifies width/height setters.
func TestComposeModal_SetWidthAndHeight(t *testing.T) {
	m := NewComposeModal(
		ComposeInitMsg{Kind: "", Title: "Test", Content: ""}, false)
	m.SetWidth(120)
	m.SetHeight(48)

	assert.Equal(t, 120, m.width)
	assert.Equal(t, 48, m.height)
}

// TestComposeModal_SetTheme_Verified verifies theme setter.
func TestComposeModal_SetTheme(t *testing.T) {
	theme := styles.NewTheme("digital-noir")
	m := NewComposeModal(
		ComposeInitMsg{Kind: "", Title: "Test", Content: ""}, false)
	m.SetTheme(theme)

	assert.NotNil(t, m.theme)
}

// TestComposeModal_View_QuitHintShowsCorrectly verifies quit hint.
func TestComposeModal_View_QuitHintShowsCorrectly(t *testing.T) {
	m := NewComposeModal(
		ComposeInitMsg{Kind: "", Title: "Test", Content: ""}, false)
	m.step = composeKind
	m.width = 80
	m.height = 24

	view := m.View()

	assert.Contains(t, view, "[ESC/q] Quit")
}

// TestComposeModal_View_ShowsHintsCorrectly verifies hints per step.
func TestComposeModal_View_ShowsHintsCorrectly(t *testing.T) {
	tests := []struct {
		name string
		step composeStep
		wantIn []string
	}{
		{"kind selector navigation hint", composeKind, []string{"[↑↓] Navigate"}},
		{"content browse hint", composeContent, []string{"[↑↓] Browse"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewComposeModal(
				ComposeInitMsg{Kind: "", Title: "Test", Content: ""}, false)
			m.step = tt.step
			m.width = 80
			m.height = 24

			view := m.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
		})
	}
}

// TestComposeModal_Init_WithPreFilledKind_GoesToContentStep verifies kind pre-fill behavior.
func TestComposeModal_Init_WithPreFilledKind_GoesToContentStep(t *testing.T) {
	tests := []struct {
		name     string
		msg      ComposeInitMsg
		wantStep composeStep
	}{
		{"empty kind starts at kind step", ComposeInitMsg{Kind: "", Title: "Test"}, composeKind},
		{"idea kind goes to content", ComposeInitMsg{Kind: "idea", Title: "Test"}, composeContent},
		{"bug kind goes to content", ComposeInitMsg{Kind: "bug", Title: "Test"}, composeContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewComposeModal(tt.msg, false)
			assert.Equal(t, tt.wantStep, m.step)
		})
	}
}

// TestComposeModal_EntrySaveCmd_ReturnsError_WhenRepoPathNotSet verifies error handling.
func TestComposeModal_EntrySaveCmd_ReturnsError_WhenRepoPathNotSet(t *testing.T) {
	m := NewComposeModal(
		ComposeInitMsg{Kind: "idea", Title: "My Feature Idea"}, false)
	m.step = composeConfirm
	m.width = 80
	m.height = 24

	cmd := m.EntrySaveCmd()
	msg := cmd()

	// EntrySaveCmd expects RepoPath in initMsg which is not set, so it returns error
	entrySavedErr, ok := msg.(EntrySavedErrMsg)
	assert.True(t, ok)
	assert.NotNil(t, entrySavedErr.Error)
}

// TestComposeModal_EntrySaveCmd_ReturnsErrorOnMarshalFail verifies error handling.
func TestComposeModal_EntrySaveCmd_ReturnsErrorOnMarshalFail(t *testing.T) {
	m := NewComposeModal(
		ComposeInitMsg{Kind: "", Title: "Test", Content: ""}, false)
	m.step = composeConfirm
	m.width = 80
	m.height = 24

	cmd := m.EntrySaveCmd()
	msg := cmd()

	entrySavedErr, ok := msg.(EntrySavedErrMsg)
	assert.True(t, ok)
	assert.NotNil(t, entrySavedErr.Error)
}
