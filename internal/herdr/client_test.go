package herdr

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubResponse is a fixed response returned for a given subcommand.
type stubResponse struct {
	stdout []byte
	stderr []byte
	err    error
}

// fakeRunner records every Run invocation and returns per-subcommand
// responses. It is the fake runner required by ADR-0001 so adapter tests never
// launch real Herdr or Sandcastle binaries.
type fakeRunner struct {
	responses map[string]stubResponse
	calls     [][]string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{responses: make(map[string]stubResponse)}
}

func (s *fakeRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	call := append([]string{}, args...)
	s.calls = append(s.calls, call)
	if len(call) == 0 {
		return nil, nil, fmt.Errorf("unexpected herdr command (no args)")
	}
	resp, ok := s.responses[call[0]]
	if !ok {
		return nil, nil, fmt.Errorf("unexpected herdr command: %s %v", name, call)
	}
	return resp.stdout, resp.stderr, resp.err
}

// lastArgs returns the most recent recorded call whose subcommand matches.
func (s *fakeRunner) lastArgs(sub string) []string {
	for i := len(s.calls) - 1; i >= 0; i-- {
		if len(s.calls[i]) > 0 && s.calls[i][0] == sub {
			return s.calls[i]
		}
	}
	return nil
}

// ran returns true if any recorded call's subcommand matches.
func (s *fakeRunner) ran(sub string) bool {
	for _, c := range s.calls {
		if len(c) > 0 && c[0] == sub {
			return true
		}
	}
	return false
}

var (
	workspaceFixture = `{"id":"cli:workspace:list","result":{"type":"workspace_list","workspaces":[
		{"workspace_id":"w6","label":"grove","number":1,"agent_status":"working","focused":true},
		{"workspace_id":"w7","label":"sandbox","number":2,"agent_status":"idle","focused":false}
	]}}`

	paneFixture = `{"id":"cli:pane:list","result":{"type":"pane_list","panes":[
		{"pane_id":"w6:p24","cwd":"/home/m00nk0d3/dev/grove","foreground_cwd":"/home/m00nk0d3/dev/grove",
		 "agent":"copilot","agent_status":"idle","terminal_title":"Change Sandcastle Workflow - GitHub Copilot",
		 "terminal_title_stripped":"Change Sandcastle Workflow - GitHub Copilot","workspace_id":"w6"},
		{"pane_id":"w6:p2T","cwd":"/home/m00nk0d3/dev/grove","foreground_cwd":"/home/m00nk0d3/dev/grove",
		 "agent_status":"unknown","terminal_title":"m00nk0d3@homelab:~/dev/grove"},
		{"pane_id":"w6:p2U","cwd":"","foreground_cwd":"/home/m00nk0d3/dev/bonsai","agent_status":"unknown",
		 "terminal_title":"bonsai"}
	]}}`

	agentFixture = `{"id":"cli:agent:list","result":{"type":"agent_list","agents":[
		{"agent":"copilot","agent_status":"idle","pane_id":"w6:p24","tab_id":"w6:t5",
		 "terminal_title":"Change Sandcastle Workflow - GitHub Copilot",
		 "terminal_title_stripped":"Change Sandcastle Workflow - GitHub Copilot"},
		{"agent":"pi","agent_status":"working","pane_id":"w6:p2Y","name":"af-go-lean-implementer",
		 "terminal_title":"pi - agent-phase","terminal_title_stripped":"pi - agent-phase"}
	]}}`

	worktreeFixture = `{"id":"cli:worktree:list","result":{"type":"worktree_list","source":{"repo_name":"grove"},"worktrees":[
		{"branch":"main","label":"grove","open_workspace_id":"w6","path":"/home/m00nk0d3/dev/grove"},
		{"branch":"agent/phase-5-parse-herdr-snapshot-lists","label":"grove",
		 "open_workspace_id":"w6","path":"/home/m00nk0d3/dev/grove/.sandcastle/worktrees/x"}
	]}}`
)

// testEnv sets the Herdr env vars for inside/outside-herdr scenarios. It clears
// every variable explicitly so inherited values (e.g. running inside Herdr) do
// not leak in.
func testEnv(t *testing.T, inside bool, workspaceID string) {
	if inside {
		t.Setenv("HERDR_ENV", "1")
	} else {
		t.Setenv("HERDR_ENV", "")
	}
	if workspaceID != "" {
		t.Setenv("HERDR_WORKSPACE_ID", workspaceID)
	} else {
		t.Setenv("HERDR_WORKSPACE_ID", "")
	}
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("HERDR_PANE_ID", "")
}

// connectedStub wires up a fully-populated connected-mode stub.
func connectedStub(r *fakeRunner) {
	r.responses["--version"] = stubResponse{stdout: []byte("herdr 0.9.1\n")}
	r.responses["workspace"] = stubResponse{stdout: []byte(workspaceFixture)}
	r.responses["pane"] = stubResponse{stdout: []byte(paneFixture)}
	r.responses["agent"] = stubResponse{stdout: []byte(agentFixture)}
	r.responses["worktree"] = stubResponse{stdout: []byte(worktreeFixture)}
}

// --- tests -------------------------------------------------------------------

func TestSnapshot_Success(t *testing.T) {
	r := newFakeRunner()
	connectedStub(r)
	testEnv(t, true, "")

	snap, err := NewClient(r).Snapshot()
	require.NoError(t, err)

	assert.Equal(t, "connected", snap.Integration.Mode)
	assert.True(t, snap.Integration.Available)
	assert.True(t, snap.Integration.Enabled)
	assert.Equal(t, "0.9.1", snap.Integration.Version)
	assert.Equal(t, "herdr", snap.Integration.Name)
	assert.NotZero(t, snap.Integration.LastSync)

	require.Len(t, snap.Workspaces, 2)
	assert.Equal(t, "grove", snap.Workspaces[0].Name)
	assert.Equal(t, "sandbox", snap.Workspaces[1].Name)
	assert.Equal(t, "", snap.Workspaces[0].Path)

	require.Len(t, snap.Panes, 3)
	assert.Equal(t, "w6:p24", snap.Panes[0].PaneID)
	assert.Equal(t, "copilot", snap.Panes[0].AgentID)
	assert.Equal(t, "/home/m00nk0d3/dev/grove", snap.Panes[0].CWD)
	assert.Equal(t, "Change Sandcastle Workflow - GitHub Copilot", snap.Panes[0].Command)
	assert.Equal(t, "idle", snap.Panes[0].Status)
	assert.Equal(t, "", snap.Panes[1].AgentID)
	assert.Equal(t, "unknown", snap.Panes[1].Status)
	// CWD falls back to foreground_cwd when cwd is empty.
	assert.Equal(t, "/home/m00nk0d3/dev/bonsai", snap.Panes[2].CWD)

	require.Len(t, snap.Agents, 2)
	assert.Equal(t, "w6:p24", snap.Agents[0].AgentID)
	assert.Equal(t, "Change Sandcastle Workflow - GitHub Copilot", snap.Agents[0].Name)
	assert.Equal(t, "idle", snap.Agents[0].Status)
	assert.Equal(t, "w6:p2Y", snap.Agents[1].AgentID)
	assert.Equal(t, "af-go-lean-implementer", snap.Agents[1].Name)
	assert.Equal(t, "working", snap.Agents[1].Status)

	require.Len(t, snap.Worktrees, 2)
	assert.Equal(t, "/home/m00nk0d3/dev/grove", snap.Worktrees[0].Path)
	assert.Equal(t, "main", snap.Worktrees[0].Branch)
	assert.Equal(t, "grove", snap.Worktrees[0].Label)
	assert.Equal(t, "w6", snap.Worktrees[0].OpenWorkspaceID)
	assert.Equal(t, "agent/phase-5-parse-herdr-snapshot-lists", snap.Worktrees[1].Branch)
}

func TestSnapshot_EmptyLists(t *testing.T) {
	r := newFakeRunner()
	r.responses["--version"] = stubResponse{stdout: []byte("herdr 0.9.1\n")}
	r.responses["workspace"] = stubResponse{stdout: []byte(`{"id":"cli:workspace:list","result":{"type":"workspace_list","workspaces":[]}}`)}
	r.responses["pane"] = stubResponse{stdout: []byte(`{"id":"cli:pane:list","result":{"type":"pane_list","panes":[]}}`)}
	r.responses["agent"] = stubResponse{stdout: []byte(`{"id":"cli:agent:list","result":{"type":"agent_list","agents":[]}}`)}
	r.responses["worktree"] = stubResponse{stdout: []byte(`{"id":"cli:worktree:list","result":{"type":"worktree_list","worktrees":[]}}`)}
	testEnv(t, true, "")

	snap, err := NewClient(r).Snapshot()
	require.NoError(t, err)
	assert.Equal(t, "connected", snap.Integration.Mode)
	assert.Len(t, snap.Workspaces, 0)
	assert.Len(t, snap.Panes, 0)
	assert.Len(t, snap.Agents, 0)
	assert.Len(t, snap.Worktrees, 0)
}

func TestSnapshot_CommandFailureAbortsAndPreservesStderr(t *testing.T) {
	r := newFakeRunner()
	connectedStub(r)
	r.responses["pane"] = stubResponse{stderr: []byte("herdr: boom\n"), err: fmt.Errorf("exit status 1")}
	testEnv(t, true, "")

	snap, err := NewClient(r).Snapshot()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "herdr pane list")
	assert.Contains(t, err.Error(), "exit status 1")
	assert.Contains(t, err.Error(), "herdr: boom")
	assert.False(t, r.ran("agent"))
	assert.False(t, r.ran("worktree"))
	assert.Equal(t, "connected", snap.Integration.Mode)
	assert.Contains(t, snap.Integration.Error, "exit status 1")
	// A failed fetch returns an integration-carrying snapshot with empty lists
	// (the first-failure abort discards partial data, per ADR).
	assert.Len(t, snap.Workspaces, 0)
	assert.Len(t, snap.Panes, 0)
	assert.Len(t, snap.Agents, 0)
	assert.Len(t, snap.Worktrees, 0)
}

func TestSnapshot_MalformedJSON(t *testing.T) {
	r := newFakeRunner()
	connectedStub(r)
	r.responses["workspace"] = stubResponse{stdout: []byte(`{not json}}`)}
	testEnv(t, true, "")

	snap, err := NewClient(r).Snapshot()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "herdr workspace list")
	// Confirm it is genuinely a JSON parse failure, not a command error.
	var syntaxErr *json.SyntaxError
	require.True(t, errors.As(err, &syntaxErr), "malformed JSON should surface a json parse error")
	assert.False(t, r.ran("pane"))
	assert.False(t, r.ran("agent"))
	assert.False(t, r.ran("worktree"))
	assert.Len(t, snap.Workspaces, 0)
}

func TestSnapshot_MalformedJSONWrongShape(t *testing.T) {
	r := newFakeRunner()
	connectedStub(r)
	r.responses["pane"] = stubResponse{stdout: []byte("[]")}
	testEnv(t, true, "")

	_, err := NewClient(r).Snapshot()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "herdr pane list")
}

func TestSnapshot_StandaloneNoCommands(t *testing.T) {
	r := newFakeRunner()
	testEnv(t, false, "")

	snap, err := NewClient(r).Snapshot()
	require.NoError(t, err)
	assert.Equal(t, "standalone", snap.Integration.Mode)
	assert.Len(t, snap.Workspaces, 0)
	assert.Len(t, snap.Panes, 0)
	assert.Len(t, snap.Agents, 0)
	assert.Len(t, snap.Worktrees, 0)
	// No command was executed in standalone mode.
	assert.Empty(t, r.calls)
}

func TestSnapshot_NilRunnerInsideHerdr(t *testing.T) {
	testEnv(t, true, "")

	// A nil runner inside Herdr yields an explicit degraded error, not a panic.
	snap, err := NewClient(nil).Snapshot()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no command runner available")
	assert.Equal(t, "degraded", snap.Integration.Mode)
}

func TestSnapshot_StandaloneNilRunnerNoPanic(t *testing.T) {
	testEnv(t, false, "")

	// Outside Herdr, a nil runner must yield an empty snapshot without panic.
	snap, err := NewClient(nil).Snapshot()
	require.NoError(t, err)
	assert.Equal(t, "standalone", snap.Integration.Mode)
	assert.Len(t, snap.Panes, 0)
}

func TestSnapshot_DegradedWhenVersionFails(t *testing.T) {
	r := newFakeRunner()
	r.responses["--version"] = stubResponse{stderr: []byte("command not found"), err: fmt.Errorf("exit status 1")}
	testEnv(t, true, "")

	snap, err := NewClient(r).Snapshot()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "detect herdr")
	assert.Contains(t, err.Error(), "command not found")
	assert.Equal(t, "degraded", snap.Integration.Mode)
	assert.False(t, r.ran("workspace"))
}

func TestSnapshot_WorkspaceFlagScopedToWorkspace(t *testing.T) {
	r := newFakeRunner()
	connectedStub(r)
	testEnv(t, true, "ws-abc")

	_, err := NewClient(r).Snapshot()
	require.NoError(t, err)

	// --workspace is passed only to pane and worktree list.
	assert.Equal(t, []string{"pane", "list", "--workspace", "ws-abc"}, r.lastArgs("pane"))
	assert.Equal(t, []string{"worktree", "list", "--workspace", "ws-abc"}, r.lastArgs("worktree"))
	// workspace and agent list take no --workspace.
	assert.Equal(t, []string{"workspace", "list"}, r.lastArgs("workspace"))
	assert.Equal(t, []string{"agent", "list"}, r.lastArgs("agent"))
}

func TestSnapshot_WorkspaceFlagAbsentWhenEnvUnset(t *testing.T) {
	r := newFakeRunner()
	connectedStub(r)
	testEnv(t, true, "")

	_, err := NewClient(r).Snapshot()
	require.NoError(t, err)

	assert.Equal(t, []string{"pane", "list"}, r.lastArgs("pane"))
	assert.Equal(t, []string{"worktree", "list"}, r.lastArgs("worktree"))
}

func TestSnapshot_VersionExtraction(t *testing.T) {
	r := newFakeRunner()
	connectedStub(r)
	r.responses["--version"] = stubResponse{stdout: []byte("herdr 1.0.0-rc.1\n")}
	testEnv(t, true, "")

	snap, err := NewClient(r).Snapshot()
	require.NoError(t, err)
	assert.Equal(t, "1.0.0-rc.1", snap.Integration.Version)
}

func TestSnapshot_AgentNameFallsBackToStrippedTitle(t *testing.T) {
	r := newFakeRunner()
	connectedStub(r)
	r.responses["agent"] = stubResponse{stdout: []byte(`{"id":"cli:agent:list","result":{"type":"agent_list","agents":[{"agent":"pi","agent_status":"working","pane_id":"w6:p9","name":"","terminal_title_stripped":"pi - phase-5"}]}}`)}
	testEnv(t, true, "")

	snap, err := NewClient(r).Snapshot()
	require.NoError(t, err)
	require.Len(t, snap.Agents, 1)
	assert.Equal(t, "pi - phase-5", snap.Agents[0].Name)
	assert.Equal(t, "w6:p9", snap.Agents[0].AgentID)
}
