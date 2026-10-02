package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/tui/modal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsNewerRelease(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{name: "multi-digit minor", current: "1.9.3", latest: "v1.10.0", want: true},
		{name: "same version", current: "v1.9.3", latest: "1.9.3"},
		{name: "current is newer", current: "2.0.0", latest: "1.10.0"},
		{name: "release supersedes prerelease", current: "v2.0.0-rc.1", latest: "v2.0.0", want: true},
		{name: "development build", current: "dev", latest: "v2.0.0"},
		{name: "git describe build", current: "1.9.3-4-gabcdef", latest: "v2.0.0"},
		{name: "dirty build", current: "1.9.3-dirty", latest: "v2.0.0"},
		{name: "invalid latest", current: "1.9.3", latest: "next"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isNewerRelease(tt.current, tt.latest))
		})
	}
}

func TestInstallCommand(t *testing.T) {
	assert.Contains(t, installCommand("linux"), "install.sh")
	assert.Contains(t, installCommand("darwin"), "install.sh")
	assert.Contains(t, installCommand("windows"), "install.ps1")
	assert.Contains(t, installCommand("windows"), "irm ")
}

func TestFetchAndCompareVersionsWith(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.10.0"}`))
	}))
	defer server.Close()

	current, latest, command, err := fetchAndCompareVersionsWith(
		server.Client(),
		server.URL,
		"v1.9.3",
		"linux",
	)
	require.NoError(t, err)
	assert.Equal(t, "v1.9.3", current)
	assert.Equal(t, "v1.10.0", latest)
	assert.Contains(t, command, "install.sh")
}

func TestUpdateCheckAddsNonBlockingBadge(t *testing.T) {
	m := NewModel()

	updated, cmd := m.Update(updateCheckedMsg{
		current: "v1.9.3",
		latest:  "v1.10.0",
		command: installCommand("linux"),
	})
	require.Nil(t, cmd)
	require.Same(t, m, updated)
	assert.Nil(t, m.activeModal)
	assert.Contains(t, m.View(), "Update: v1.10.0 [u] details")

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	require.Nil(t, cmd)
	assert.IsType(t, &modal.UpdateNotificationModal{}, m.activeModal)
}
