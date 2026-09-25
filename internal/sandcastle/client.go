package sandcastle

import (
	"context"
	"encoding/json"
	"fmt"
	osexec "os/exec"
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
)

// Client provides access to the Sandcastle CLI for reading workflow state
// and starting new workflows. All methods respect context cancellation.
type Client interface {
	Available(ctx context.Context) domain.ExternalIntegration
	Snapshot(ctx context.Context, repoPath string) (Snapshot, error)
	StartWorkflow(ctx context.Context, req StartWorkflowRequest) (domain.WorkflowRunRef, error)
	RemoveWorkflow(ctx context.Context, repoPath, runID string, stop bool) error
}

// ClientConfig holds the configuration for creating a Sandcastle client.
type ClientConfig struct {
	Binary       string
	DefaultAgent string
	Timeout      time.Duration
	// LookPath returns the path to the named executable. When nil,
	// os/exec.LookPath is used. Tests may supply a stub.
	LookPath func(name string) (string, error)
}

// StartWorkflowRequest describes a workflow start request to Sandcastle.
type StartWorkflowRequest struct {
	Kind         string
	RepoPath     string
	WorktreePath string
	Branch       string
	IssueNumber  *int
	PRNumber     *int
	AgentKind    string
	Source       string
}

type sandcastleClient struct {
	config ClientConfig
	runner CommandRunner
}

// NewClient creates a Sandcastle Client with the given config and command runner.
// A zero Timeout defaults to 10s, a zero DefaultAgent to "opencode", and a
// zero Binary to "grove-sandcastle", the command the installers provide.
func NewClient(config ClientConfig, runner CommandRunner) Client {
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.DefaultAgent == "" {
		config.DefaultAgent = "opencode"
	}
	if config.Binary == "" {
		config.Binary = "grove-sandcastle"
	}
	if config.LookPath == nil {
		config.LookPath = osexec.LookPath
	}
	return &sandcastleClient{config: config, runner: runner}
}

// Available checks whether the Sandcastle binary is reachable on PATH.
func (c *sandcastleClient) Available(_ context.Context) domain.ExternalIntegration {
	_, err := c.config.LookPath(c.config.Binary)
	if err != nil {
		return domain.ExternalIntegration{
			Name:      "sandcastle",
			Mode:      "missing",
			Available: false,
			Enabled:   true,
			Error:     "sandcastle binary not found",
		}
	}
	return domain.ExternalIntegration{
		Name:      "sandcastle",
		Mode:      "available",
		Available: true,
		Enabled:   true,
	}
}

// Snapshot fetches the current Sandcastle status and returns a normalized snapshot
// suitable for mission-control consumption. When the binary is unavailable the
// snapshot carries an unavailable integration and no workflows; it never returns
// an error for that condition.
func (c *sandcastleClient) Snapshot(ctx context.Context, _ string) (Snapshot, error) {
	info := c.Available(ctx)
	if !info.Available {
		return Snapshot{Integration: info}, nil
	}

	stdout, stderr, err := c.runCommand(ctx, "status")
	if err != nil {
		return Snapshot{
			Integration: domain.ExternalIntegration{
				Name:      "sandcastle",
				Mode:      "degraded",
				Available: true,
				Enabled:   true,
				Error:     preserveStderr(stderr, err),
			},
		}, fmt.Errorf("sandcastle status: %w", err)
	}

	var resp statusResponse
	if err := json.Unmarshal(stdout, &resp); err != nil {
		return Snapshot{
			Integration: domain.ExternalIntegration{
				Name:      "sandcastle",
				Mode:      "degraded",
				Available: true,
				Enabled:   true,
				Error:     "malformed JSON from sandcastle status",
			},
		}, fmt.Errorf("parse sandcastle status: %w", err)
	}

	integration := domain.ExternalIntegration{
		Name:      "sandcastle",
		Mode:      "connected",
		Available: true,
		Enabled:   true,
		Version:   resp.Version,
		LastSync:  time.Now(),
	}

	workflows := make([]domain.WorkflowRunRef, 0, len(resp.Workflows))
	var agents []domain.AgentRef
	for _, w := range resp.Workflows {
		wf, ags := normalizeWorkflow(w)
		workflows = append(workflows, wf)
		agents = append(agents, ags...)
	}

	return Snapshot{
		Integration: integration,
		Workflows:   workflows,
		Agents:      agents,
		CapturedAt:  time.Now(),
	}, nil
}

// StartWorkflow requests Sandcastle to start a new workflow run.
// When AgentKind is empty the configured default agent is used.
func (c *sandcastleClient) StartWorkflow(ctx context.Context, req StartWorkflowRequest) (domain.WorkflowRunRef, error) {
	info := c.Available(ctx)
	if !info.Available {
		return domain.WorkflowRunRef{}, fmt.Errorf("sandcastle not available: %s", info.Error)
	}

	agent := req.AgentKind
	if agent == "" {
		agent = c.config.DefaultAgent
	}
	source := req.Source
	if source == "" {
		source = "grove"
	}
	kind := req.Kind
	if kind == "" {
		kind = "imp"
	}

	args := []string{
		"workflow", "start", "--json",
		"--kind", kind,
		"--repo", req.RepoPath,
		"--worktree", req.WorktreePath,
		"--agent", agent,
		"--source", source,
	}
	if req.IssueNumber != nil {
		args = append(args, "--issue", fmt.Sprintf("%d", *req.IssueNumber))
	}
	if req.PRNumber != nil {
		args = append(args, "--pr", fmt.Sprintf("%d", *req.PRNumber))
	}

	stdout, stderr, err := c.runCommand(ctx, args...)
	if err != nil {
		return domain.WorkflowRunRef{}, fmt.Errorf("start workflow: %s: %w", preserveStderr(stderr, err), err)
	}

	var resp startWorkflowResponse
	if err := json.Unmarshal(stdout, &resp); err != nil {
		return domain.WorkflowRunRef{}, fmt.Errorf("parse start response: %w", err)
	}

	wf, _ := normalizeWorkflow(resp.Workflow)
	return wf, nil
}

// RemoveWorkflow removes a tracked workflow. Active runs are stopped first
// only when stop is explicitly true.
func (c *sandcastleClient) RemoveWorkflow(ctx context.Context, repoPath, runID string, stop bool) error {
	if runID == "" {
		return fmt.Errorf("remove workflow: run ID is required")
	}
	args := []string{"workflow", "remove", runID, "--json", "--repo", repoPath}
	if stop {
		args = append(args, "--stop")
	}
	_, stderr, err := c.runCommand(ctx, args...)
	if err != nil {
		return fmt.Errorf("remove workflow: %s: %w", preserveStderr(stderr, err), err)
	}
	return nil
}

func (c *sandcastleClient) runCommand(ctx context.Context, args ...string) ([]byte, []byte, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()
	return c.runner.Run(timeoutCtx, c.config.Binary, args...)
}

func preserveStderr(stderr []byte, err error) string {
	s := string(stderr)
	if s == "" {
		return err.Error()
	}
	return s
}

// --- JSON shapes from the Sandcastle CLI contract ---

type statusResponse struct {
	Version         string           `json:"version"`
	UpdatedAt       string           `json:"updated_at"`
	ActiveWorkflows int              `json:"active_workflows"`
	Workflows       []workflowRunRaw `json:"workflows"`
}

type workflowListResponse struct {
	Workflows []workflowRunRaw `json:"workflows"`
}

type startWorkflowResponse struct {
	Workflow workflowRunRaw `json:"workflow"`
}

type workflowRunRaw struct {
	ID           string      `json:"id"`
	Kind         string      `json:"kind"`
	Title        string      `json:"title"`
	Status       string      `json:"status"`
	Repo         string      `json:"repo"`
	WorktreePath string      `json:"worktree_path"`
	Branch       string      `json:"branch"`
	DefaultAgent string      `json:"default_agent"`
	CurrentStep  string      `json:"current_step"`
	Progress     progressRaw `json:"progress"`
	Github       githubRaw   `json:"github"`
	Agents       []agentRaw  `json:"agents"`
	Steps        []stepRaw   `json:"steps"`
	StartedAt    string      `json:"started_at"`
	UpdatedAt    string      `json:"updated_at"`
	Error        string      `json:"error"`
}

type progressRaw struct {
	Completed int `json:"completed"`
	Total     int `json:"total"`
	Percent   int `json:"percent"`
}

type githubRaw struct {
	Issue       *int `json:"issue"`
	PullRequest *int `json:"pull_request"`
}

type agentRaw struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	PaneID  string `json:"pane_id"`
}

type stepRaw struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	Summary        string `json:"summary"`
	StartedAt      string `json:"started_at"`
	CompletedAt    string `json:"completed_at"`
	DurationMillis int64  `json:"duration_ms"`
}

// normalizeWorkflow converts a raw Sandcastle workflow JSON shape into
// a domain WorkflowRunRef and associated AgentRefs. Unknown enum values
// are preserved literally (rule 8 of the contract).
func normalizeWorkflow(w workflowRunRaw) (domain.WorkflowRunRef, []domain.AgentRef) {
	agent := w.DefaultAgent
	if agent == "" {
		agent = "opencode"
	}

	wf := domain.WorkflowRunRef{
		WorkflowID:   w.ID,
		RunID:        w.ID,
		Kind:         w.Kind,
		Title:        w.Title,
		Repo:         w.Repo,
		WorktreePath: w.WorktreePath,
		Branch:       w.Branch,
		Status:       normalizeWorkflowStatus(w.Status),
		DefaultAgent: agent,
		CurrentStep:  w.CurrentStep,
		Progress: domain.WorkflowProgress{
			Completed: w.Progress.Completed,
			Total:     w.Progress.Total,
			Percent:   w.Progress.Percent,
		},
		IssueNumber: w.Github.Issue,
		PRNumber:    w.Github.PullRequest,
		StartedAt:   parseRFC3339(w.StartedAt),
		UpdatedAt:   parseRFC3339(w.UpdatedAt),
		Error:       w.Error,
	}
	for _, step := range w.Steps {
		wf.Steps = append(wf.Steps, domain.WorkflowStep{
			ID:             step.ID,
			Title:          step.Title,
			Status:         step.Status,
			Summary:        step.Summary,
			StartedAt:      parseRFC3339(step.StartedAt),
			CompletedAt:    parseRFC3339(step.CompletedAt),
			DurationMillis: step.DurationMillis,
		})
	}

	agents := make([]domain.AgentRef, 0, len(w.Agents))
	for _, a := range w.Agents {
		agents = append(agents, domain.AgentRef{
			AgentID:       a.ID,
			Kind:          a.Kind,
			Name:          a.Name,
			WorkflowRunID: w.ID,
			Status:        normalizeAgentStatus(a.Status),
			Summary:       a.Summary,
			PaneID:        a.PaneID,
		})
	}

	return wf, agents
}

func parseRFC3339(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// normalizeWorkflowStatus maps raw Sandcastle workflow status strings to
// domain constants. Unknown values are preserved literally (rule 8).
func normalizeWorkflowStatus(raw string) string {
	switch raw {
	case domain.WorkflowQueued:
		return domain.WorkflowQueued
	case domain.WorkflowRunning:
		return domain.WorkflowRunning
	case domain.WorkflowBlocked:
		return domain.WorkflowBlocked
	case domain.WorkflowFailed:
		return domain.WorkflowFailed
	case domain.WorkflowSucceeded:
		return domain.WorkflowSucceeded
	default:
		return raw
	}
}

// normalizeAgentStatus maps raw Sandcastle agent status strings to
// domain constants. Unknown values are preserved literally (rule 8).
func normalizeAgentStatus(raw string) string {
	switch raw {
	case domain.AgentWorking:
		return domain.AgentWorking
	case domain.AgentIdle:
		return domain.AgentIdle
	case domain.AgentBlocked:
		return domain.AgentBlocked
	case domain.AgentFailed:
		return domain.AgentFailed
	case domain.AgentDone:
		return domain.AgentDone
	default:
		return raw
	}
}
