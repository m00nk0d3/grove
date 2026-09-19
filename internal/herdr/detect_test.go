package herdr

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRunner struct {
	stdout []byte
	stderr []byte
	err    error
}

func (s stubRunner) Run(_ string, _ ...string) ([]byte, []byte, error) {
	return s.stdout, s.stderr, s.err
}

func unsetEnv(keys ...string) {
	for _, k := range keys {
		os.Unsetenv(k)
	}
}

func TestDetect_StandaloneWhenHERDR_ENVUnset(t *testing.T) {
	unsetEnv("HERDR_ENV", "HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID")

	d := Detect(stubRunner{})

	assert.Equal(t, ModeStandalone, d.Mode)
	assert.Empty(t, d.WorkspaceID)
	assert.Empty(t, d.TabID)
	assert.Empty(t, d.PaneID)
	assert.NoError(t, d.Err)
}

func TestDetect_ConnectedWhenHERDR_ENVSetAndBinarySucceeds(t *testing.T) {
	t.Setenv("HERDR_ENV", "1")
	unsetEnv("HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID")

	d := Detect(stubRunner{stdout: []byte("herdr 0.1.0")})

	assert.Equal(t, ModeConnected, d.Mode)
	assert.NoError(t, d.Err)
}

func TestDetect_DegradedWhenHERDR_ENVSetAndBinaryFails(t *testing.T) {
	t.Setenv("HERDR_ENV", "1")
	unsetEnv("HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID")

	stderr := []byte("command not found")
	err := fmt.Errorf("exit status 1")
	d := Detect(stubRunner{stderr: stderr, err: err})

	assert.Equal(t, ModeDegraded, d.Mode)
	require.Error(t, d.Err)
	assert.Contains(t, d.Err.Error(), "exit status 1")
	assert.Contains(t, d.Err.Error(), "command not found")
}

func TestDetect_PopulatesWorkspaceTabPaneIDs(t *testing.T) {
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_WORKSPACE_ID", "ws-abc")
	t.Setenv("HERDR_TAB_ID", "tab-42")
	t.Setenv("HERDR_PANE_ID", "pane-7")

	d := Detect(stubRunner{stdout: []byte("herdr 0.1.0")})

	assert.Equal(t, ModeConnected, d.Mode)
	assert.Equal(t, "ws-abc", d.WorkspaceID)
	assert.Equal(t, "tab-42", d.TabID)
	assert.Equal(t, "pane-7", d.PaneID)
}

func TestDetect_NilRunnerReturnsDegraded(t *testing.T) {
	t.Setenv("HERDR_ENV", "1")
	unsetEnv("HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID")

	d := Detect(nil)

	assert.Equal(t, ModeDegraded, d.Mode)
	require.Error(t, d.Err)
	assert.Contains(t, d.Err.Error(), "no command runner available")
}

func TestDetect_StandaloneIgnoresEnvIDs(t *testing.T) {
	unsetEnv("HERDR_ENV")
	t.Setenv("HERDR_WORKSPACE_ID", "ws-abc")
	t.Setenv("HERDR_TAB_ID", "tab-42")
	t.Setenv("HERDR_PANE_ID", "pane-7")

	d := Detect(stubRunner{})

	assert.Equal(t, ModeStandalone, d.Mode)
	assert.Empty(t, d.WorkspaceID)
	assert.Empty(t, d.TabID)
	assert.Empty(t, d.PaneID)
}
