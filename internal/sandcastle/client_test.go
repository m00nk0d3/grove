package sandcastle

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRunner struct {
	responses map[string]fakeResponse
	calls     [][]string
}

type fakeResponse struct {
	stdout []byte
	stderr []byte
	err    error
}

func (f *fakeRunner) Run(_ context.Context, _ string, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, args)
	key := args[0]
	resp, ok := f.responses[key]
	if !ok {
		return nil, nil, fmt.Errorf("unexpected command: %v", args)
	}
	return resp.stdout, resp.stderr, resp.err
}

func (f *fakeRunner) lastCall() []string {
	if len(f.calls) == 0 {
		return nil
	}
	return f.calls[len(f.calls)-1]
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{responses: make(map[string]fakeResponse)}
}

func fakeLookPath(name string) (string, error) {
	return name, nil
}

func newTestClient(runner *fakeRunner) Client {
	return NewClient(ClientConfig{
		Binary:       "sandcastle",
		DefaultAgent: "opencode",
		Timeout:      5 * time.Second,
		LookPath:     fakeLookPath,
	}, runner)
}

func intPtr(n int) *int { return &n }

func indexAfter(slice []string, target string) int {
	for i, s := range slice {
		if s == target {
			return i
		}
	}
	return -1
}

const statusPayload = `{
	"version": "0.4.0",
	"updated_at": "2026-09-17T21:19:00Z",
	"active_workflows": 1,
	"workflows": [
		{
			"id": "run_123",
			"title": "Implement issue #42",
			"status": "running",
			"repo": "/home/user/dev/project",
			"worktree_path": "/home/user/dev/project-worktrees/issue-42",
			"branch": "issue-42",
			"default_agent": "pi",
			"current_step": "Editing files",
			"progress": {"completed": 3, "total": 7, "percent": 42},
			"github": {"issue": 42, "pull_request": null},
			"agents": [
				{
					"id": "agent_pi_1",
					"kind": "pi",
					"name": "pi-main",
					"status": "working",
					"summary": "Refactoring renderer state model",
					"pane_id": "w1:p3"
				}
			],
			"steps": [
				{"id": "step_1", "title": "Inspect repo", "status": "succeeded"},
				{"id": "step_2", "title": "Implement mission-control model", "status": "running"}
			],
			"started_at": "2026-09-17T21:00:00Z",
			"updated_at": "2026-09-17T21:19:00Z"
		}
	]
}`

func TestSnapshot_SuccessfulStatus(t *testing.T) {
	runner := newFakeRunner()
	runner.responses["status"] = fakeResponse{stdout: []byte(statusPayload)}

	client := newTestClient(runner)
	snap, err := client.Snapshot(context.Background(), "/repo")
	require.NoError(t, err)

	assert.Equal(t, "0.4.0", snap.Integration.Version)
	assert.Equal(t, "connected", snap.Integration.Mode)
	assert.True(t, snap.Integration.Available)
	require.Len(t, snap.Workflows, 1)

	wf := snap.Workflows[0]
	assert.Equal(t, "run_123", wf.WorkflowID)
	assert.Equal(t, "run_123", wf.RunID)
	assert.Equal(t, "running", wf.Status)
	assert.Equal(t, "issue-42", wf.Branch)
	assert.Equal(t, "/home/user/dev/project-worktrees/issue-42", wf.WorktreePath)
	assert.Equal(t, "Implement issue #42", wf.Title)
	assert.Equal(t, "pi", wf.DefaultAgent)
	assert.Equal(t, "Editing files", wf.CurrentStep)
	assert.Equal(t, 42, wf.Progress.Percent)
	assert.Equal(t, 3, wf.Progress.Completed)
	require.Len(t, wf.Steps, 2)
	assert.Equal(t, "Inspect repo", wf.Steps[0].Title)
	assert.Equal(t, "succeeded", wf.Steps[0].Status)
	assert.False(t, wf.StartedAt.IsZero())
	assert.False(t, wf.UpdatedAt.IsZero())
	require.NotNil(t, wf.IssueNumber)
	assert.Equal(t, 42, *wf.IssueNumber)
	assert.Nil(t, wf.PRNumber)

	require.Len(t, snap.Agents, 1)
	assert.Equal(t, "agent_pi_1", snap.Agents[0].AgentID)
	assert.Equal(t, "pi-main", snap.Agents[0].Name)
	assert.Equal(t, "working", snap.Agents[0].Status)
	assert.Equal(t, "run_123", snap.Agents[0].WorkflowRunID)
	assert.Equal(t, "pi", snap.Agents[0].Kind)
	assert.Equal(t, "Refactoring renderer state model", snap.Agents[0].Summary)
	assert.Equal(t, "w1:p3", snap.Agents[0].PaneID)

	assert.Equal(t, []string{"status"}, runner.lastCall())
}

func TestNormalizeWorkflow_WithIssueAndPR(t *testing.T) {
	raw := workflowRunRaw{
		ID:           "run_456",
		Kind:         "review",
		Title:        "Fix bug",
		Status:       "succeeded",
		Repo:         "/repo",
		WorktreePath: "/worktrees/fix",
		Branch:       "fix-bug",
		DefaultAgent: "pi",
		Github:       githubRaw{Issue: intPtr(99), PullRequest: intPtr(101)},
		Agents: []agentRaw{
			{ID: "a1", Kind: "pi", Name: "pi-bot", Status: "done", Summary: "completed"},
		},
	}

	wf, agents := normalizeWorkflow(raw)

	assert.Equal(t, "run_456", wf.WorkflowID)
	assert.Equal(t, "review", wf.Kind)
	assert.Equal(t, "succeeded", wf.Status)
	assert.Equal(t, "fix-bug", wf.Branch)
	require.NotNil(t, wf.IssueNumber)
	assert.Equal(t, 99, *wf.IssueNumber)
	require.NotNil(t, wf.PRNumber)
	assert.Equal(t, 101, *wf.PRNumber)

	require.Len(t, agents, 1)
	assert.Equal(t, "a1", agents[0].AgentID)
	assert.Equal(t, "done", agents[0].Status)
}

func TestStartWorkflow_Success(t *testing.T) {
	runner := newFakeRunner()
	runner.responses["workflow"] = fakeResponse{
		stdout: []byte(`{"workflow": {"id": "run_789", "status": "queued", "default_agent": "pi", "worktree_path": "/path/to/worktree"}}`),
	}

	client := newTestClient(runner)
	wf, err := client.StartWorkflow(context.Background(), StartWorkflowRequest{
		RepoPath:     "/repo",
		WorktreePath: "/path/to/worktree",
		Branch:       "feat-x",
	})
	require.NoError(t, err)

	assert.Equal(t, "run_789", wf.WorkflowID)
	assert.Equal(t, "queued", wf.Status)
	assert.Equal(t, "/path/to/worktree", wf.WorktreePath)

	call := runner.lastCall()
	assert.Contains(t, call, "workflow")
	assert.Contains(t, call, "start")
	assert.Contains(t, call, "--kind")
	assert.Contains(t, call, "imp")
	assert.Contains(t, call, "--agent")
	assert.Contains(t, call, "opencode")
	assert.Contains(t, call, "--source")
	assert.Contains(t, call, "grove")
}

func TestStartWorkflow_DefaultAgentAndSource(t *testing.T) {
	runner := newFakeRunner()
	runner.responses["workflow"] = fakeResponse{
		stdout: []byte(`{"workflow": {"id": "run_1", "status": "queued"}}`),
	}

	client := newTestClient(runner)
	_, err := client.StartWorkflow(context.Background(), StartWorkflowRequest{
		RepoPath:     "/repo",
		WorktreePath: "/wt",
	})
	require.NoError(t, err)

	call := runner.lastCall()
	agentIdx := indexAfter(call, "--agent")
	require.GreaterOrEqual(t, agentIdx, 0)
	assert.Equal(t, "opencode", call[agentIdx+1])

	sourceIdx := indexAfter(call, "--source")
	require.GreaterOrEqual(t, sourceIdx, 0)
	assert.Equal(t, "grove", call[sourceIdx+1])
}

func TestStartWorkflow_CustomAgentAndSource(t *testing.T) {
	runner := newFakeRunner()
	runner.responses["workflow"] = fakeResponse{
		stdout: []byte(`{"workflow": {"id": "run_2", "status": "queued"}}`),
	}

	client := newTestClient(runner)
	_, err := client.StartWorkflow(context.Background(), StartWorkflowRequest{
		RepoPath:     "/repo",
		WorktreePath: "/wt",
		AgentKind:    "claude",
		Source:       "manual",
	})
	require.NoError(t, err)

	call := runner.lastCall()
	agentIdx := indexAfter(call, "--agent")
	require.GreaterOrEqual(t, agentIdx, 0)
	assert.Equal(t, "claude", call[agentIdx+1])

	sourceIdx := indexAfter(call, "--source")
	require.GreaterOrEqual(t, sourceIdx, 0)
	assert.Equal(t, "manual", call[sourceIdx+1])
}

func TestStartWorkflow_IncludesGitHubTargets(t *testing.T) {
	runner := newFakeRunner()
	runner.responses["workflow"] = fakeResponse{
		stdout: []byte(`{"workflow": {"id": "run_3", "status": "queued"}}`),
	}
	issueNumber := 42
	prNumber := 84

	client := newTestClient(runner)
	_, err := client.StartWorkflow(context.Background(), StartWorkflowRequest{
		RepoPath:    "/repo",
		IssueNumber: &issueNumber,
		PRNumber:    &prNumber,
	})
	require.NoError(t, err)

	call := runner.lastCall()
	assert.Contains(t, call, "--issue")
	assert.Contains(t, call, "42")
	assert.Contains(t, call, "--pr")
	assert.Contains(t, call, "84")
}

func TestStartWorkflow_IncludesRequestedKind(t *testing.T) {
	runner := newFakeRunner()
	runner.responses["workflow"] = fakeResponse{
		stdout: []byte(`{"workflow": {"id": "run_4", "status": "queued"}}`),
	}

	prNumber := 84

	client := newTestClient(runner)
	_, err := client.StartWorkflow(context.Background(), StartWorkflowRequest{
		Kind:      "review",
		RepoPath:  "/repo",
		PRNumber:  &prNumber,
		AgentKind: "opencode",
	})
	require.NoError(t, err)

	call := runner.lastCall()
	kindIdx := indexAfter(call, "--kind")
	require.GreaterOrEqual(t, kindIdx, 0)
	assert.Equal(t, "review", call[kindIdx+1])
}

func TestRemoveWorkflow_RequiresRunID(t *testing.T) {
	client := newTestClient(newFakeRunner())

	err := client.RemoveWorkflow(context.Background(), "/repo", "", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "run ID is required")
}

func TestRemoveWorkflow_SendsStopOnlyWhenConfirmed(t *testing.T) {
	tests := []struct {
		name string
		stop bool
	}{
		{name: "remove history", stop: false},
		{name: "stop active workflow", stop: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.responses["workflow"] = fakeResponse{stdout: []byte(`{"removed":"run_42"}`)}
			client := newTestClient(runner)

			err := client.RemoveWorkflow(context.Background(), "/repo", "run_42", tt.stop)

			require.NoError(t, err)
			call := runner.lastCall()
			assert.Equal(t, []string{"workflow", "remove", "run_42", "--json", "--repo", "/repo"}, call[:6])
			assert.Equal(t, tt.stop, containsString(call, "--stop"))
		})
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestAvailable_MissingBinary(t *testing.T) {
	runner := newFakeRunner()
	client := NewClient(ClientConfig{
		Binary:  "nonexistent-binary-xyz",
		Timeout: 5 * time.Second,
	}, runner)

	info := client.Available(context.Background())
	assert.False(t, info.Available)
	assert.Contains(t, info.Error, "not found")
}

func TestSnapshot_MissingBinary(t *testing.T) {
	runner := newFakeRunner()
	client := NewClient(ClientConfig{
		Binary:  "nonexistent-binary-xyz",
		Timeout: 5 * time.Second,
	}, runner)

	snap, err := client.Snapshot(context.Background(), "/repo")
	require.NoError(t, err)
	assert.False(t, snap.Integration.Available)
	assert.Equal(t, "missing", snap.Integration.Mode)
	assert.Empty(t, snap.Workflows)
}

func TestStartWorkflow_MissingBinary(t *testing.T) {
	runner := newFakeRunner()
	client := NewClient(ClientConfig{
		Binary:  "nonexistent-binary-xyz",
		Timeout: 5 * time.Second,
	}, runner)

	_, err := client.StartWorkflow(context.Background(), StartWorkflowRequest{
		RepoPath:     "/repo",
		WorktreePath: "/wt",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
}

func TestSnapshot_MalformedJSON(t *testing.T) {
	runner := newFakeRunner()
	runner.responses["status"] = fakeResponse{stdout: []byte("not json")}

	client := newTestClient(runner)
	snap, err := client.Snapshot(context.Background(), "/repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse sandcastle status")
	assert.True(t, snap.Integration.Available)
	assert.Equal(t, "degraded", snap.Integration.Mode)
	assert.Contains(t, snap.Integration.Error, "malformed")
}

func TestStartWorkflow_MalformedJSON(t *testing.T) {
	runner := newFakeRunner()
	runner.responses["workflow"] = fakeResponse{stdout: []byte("{bad")}

	client := newTestClient(runner)
	_, err := client.StartWorkflow(context.Background(), StartWorkflowRequest{
		RepoPath:     "/repo",
		WorktreePath: "/wt",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse start response")
}

func TestSnapshot_CommandFailure(t *testing.T) {
	runner := newFakeRunner()
	runner.responses["status"] = fakeResponse{
		stderr: []byte("connection refused"),
		err:    &exec.ExitError{},
	}

	client := newTestClient(runner)
	snap, err := client.Snapshot(context.Background(), "/repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandcastle status")
	assert.Equal(t, "degraded", snap.Integration.Mode)
	assert.Contains(t, snap.Integration.Error, "connection refused")
}

func TestStartWorkflow_CommandFailure(t *testing.T) {
	runner := newFakeRunner()
	runner.responses["workflow"] = fakeResponse{
		stderr: []byte("permission denied"),
		err:    &exec.ExitError{},
	}

	client := newTestClient(runner)
	_, err := client.StartWorkflow(context.Background(), StartWorkflowRequest{
		RepoPath:     "/repo",
		WorktreePath: "/wt",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start workflow")
}

func TestSnapshot_EmptyWorkflows(t *testing.T) {
	runner := newFakeRunner()
	runner.responses["status"] = fakeResponse{
		stdout: []byte(`{"version": "0.4.0", "updated_at": "2026-09-17T21:19:00Z", "active_workflows": 0, "workflows": []}`),
	}

	client := newTestClient(runner)
	snap, err := client.Snapshot(context.Background(), "/repo")
	require.NoError(t, err)
	assert.True(t, snap.Integration.Available)
	assert.Empty(t, snap.Workflows)
	assert.Empty(t, snap.Agents)
}

func TestSnapshot_EmptyAgentsAndSteps(t *testing.T) {
	runner := newFakeRunner()
	runner.responses["status"] = fakeResponse{
		stdout: []byte(`{
			"version": "0.4.0",
			"updated_at": "2026-09-17T21:19:00Z",
			"active_workflows": 1,
			"workflows": [
				{
					"id": "run_1",
					"title": "Test",
					"status": "running",
					"repo": "/repo",
					"worktree_path": "/wt",
					"branch": "main",
					"progress": {"completed": 0, "total": 0, "percent": 0},
					"github": {"issue": null, "pull_request": null},
					"agents": [],
					"steps": [],
					"started_at": "2026-09-17T21:00:00Z",
					"updated_at": "2026-09-17T21:19:00Z"
				}
			]
		}`),
	}

	client := newTestClient(runner)
	snap, err := client.Snapshot(context.Background(), "/repo")
	require.NoError(t, err)
	require.Len(t, snap.Workflows, 1)
	assert.Empty(t, snap.Agents)
}

func TestNormalizeWorkflow_FallbackAgent(t *testing.T) {
	raw := workflowRunRaw{
		ID:           "run_x",
		Status:       "queued",
		WorktreePath: "/wt",
		Branch:       "main",
	}

	wf, _ := normalizeWorkflow(raw)
	assert.Equal(t, "run_x", wf.WorkflowID)
	assert.Equal(t, "queued", wf.Status)
}

func TestNewClient_DefaultsToTheGroveSandcastleCommand(t *testing.T) {
	var looked string
	client := NewClient(ClientConfig{LookPath: func(name string) (string, error) {
		looked = name
		return name, nil
	}}, newFakeRunner())

	info := client.Available(context.Background())

	assert.True(t, info.Available)
	assert.Equal(t, "grove-sandcastle", looked, "the installers provide grove-sandcastle, not sandcastle")
}

func TestNormalizeWorkflowStatus_KnownValues(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"queued", "queued"},
		{"running", "running"},
		{"blocked", "blocked"},
		{"failed", "failed"},
		{"succeeded", "succeeded"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeWorkflowStatus(tt.input))
		})
	}
}

func TestNormalizeWorkflowStatus_UnknownPassthrough(t *testing.T) {
	assert.Equal(t, "custom-status", normalizeWorkflowStatus("custom-status"))
}

func TestNormalizeWorkflowStatus_EmptyString(t *testing.T) {
	assert.Equal(t, "", normalizeWorkflowStatus(""))
}

func TestNormalizeAgentStatus_KnownValues(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"working", "working"},
		{"idle", "idle"},
		{"blocked", "blocked"},
		{"failed", "failed"},
		{"done", "done"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeAgentStatus(tt.input))
		})
	}
}

func TestNormalizeAgentStatus_UnknownPassthrough(t *testing.T) {
	assert.Equal(t, "custom-agent-status", normalizeAgentStatus("custom-agent-status"))
}

func TestNormalizeAgentStatus_EmptyString(t *testing.T) {
	assert.Equal(t, "", normalizeAgentStatus(""))
}

func TestSnapshot_StatusNormalization(t *testing.T) {
	payload := `{
		"version": "0.4.0",
		"updated_at": "2026-09-17T21:19:00Z",
		"active_workflows": 1,
		"workflows": [
			{
				"id": "run_norm",
				"title": "Test normalization",
				"status": "running",
				"repo": "/repo",
				"worktree_path": "/wt",
				"branch": "main",
				"default_agent": "pi",
				"progress": {"completed": 1, "total": 5, "percent": 20},
				"github": {"issue": null, "pull_request": null},
				"agents": [
					{"id": "a1", "kind": "pi", "name": "pi-1", "status": "working", "summary": "working", "pane_id": null}
				],
				"steps": [],
				"started_at": "2026-09-17T21:00:00Z",
				"updated_at": "2026-09-17T21:19:00Z"
			}
		]
	}`

	runner := newFakeRunner()
	runner.responses["status"] = fakeResponse{stdout: []byte(payload)}

	client := newTestClient(runner)
	snap, err := client.Snapshot(context.Background(), "/repo")
	require.NoError(t, err)
	require.Len(t, snap.Workflows, 1)
	assert.Equal(t, "running", snap.Workflows[0].Status)
	require.Len(t, snap.Agents, 1)
	assert.Equal(t, "working", snap.Agents[0].Status)
}

func TestSnapshot_UnknownStatusPassthrough(t *testing.T) {
	payload := `{
		"version": "0.4.0",
		"updated_at": "2026-09-17T21:19:00Z",
		"active_workflows": 1,
		"workflows": [
			{
				"id": "run_unknown",
				"title": "Test unknown status",
				"status": "custom_future_status",
				"repo": "/repo",
				"worktree_path": "/wt",
				"branch": "main",
				"progress": {"completed": 0, "total": 1, "percent": 0},
				"github": {"issue": null, "pull_request": null},
				"agents": [
					{"id": "a1", "kind": "pi", "name": "pi-1", "status": "custom_agent_state", "summary": "custom", "pane_id": null}
				],
				"steps": [],
				"started_at": "2026-09-17T21:00:00Z",
				"updated_at": "2026-09-17T21:19:00Z"
			}
		]
	}`

	runner := newFakeRunner()
	runner.responses["status"] = fakeResponse{stdout: []byte(payload)}

	client := newTestClient(runner)
	snap, err := client.Snapshot(context.Background(), "/repo")
	require.NoError(t, err)
	require.Len(t, snap.Workflows, 1)
	assert.Equal(t, "custom_future_status", snap.Workflows[0].Status)
	require.Len(t, snap.Agents, 1)
	assert.Equal(t, "custom_agent_state", snap.Agents[0].Status)
}
