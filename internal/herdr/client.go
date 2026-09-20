package herdr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
)

const herdrBinary = "herdr"

var agentArgs = []string{"agent", "list"}

// Client fetches Herdr list state through a CommandRunner and normalizes it
// into a Snapshot for mission control. List subcommands are invoked bare (no
// --json): the measured herdr 0.9.1 emits JSON natively for these commands and
// rejects --json (see herdr --skill).
type Client struct {
	runner CommandRunner
}

// NewClient constructs a Client backed by the given command runner. A zero or
// nil runner does not fail construction; detection falls through to standalone
// outside Herdr or to an explicit degraded error when running inside Herdr.
func NewClient(runner CommandRunner) Client {
	return Client{runner: runner}
}

// Snapshot fetches the Herdr workspace, pane, agent, and worktree lists and
// returns a normalized Snapshot.
//
// Outside Herdr (HERDR_ENV unset) no command is executed and an empty
// standalone snapshot is returned. Inside Herdr with an unavailable binary a
// degraded snapshot is returned with an explicit error. The first failing
// command aborts the fetch with an explicit error that preserves command
// stderr; the returned snapshot carries the detected integration state with
// empty lists (per ADR, partial data is not returned on failure).
func (c Client) Snapshot() (Snapshot, error) {
	d := Detect(c.runner)

	switch d.Mode {
	case ModeStandalone:
		return Snapshot{
			Integration: domain.ExternalIntegration{Mode: "standalone"},
		}, nil
	case ModeDegraded:
		return Snapshot{
			Integration: domain.ExternalIntegration{
				Name:    "herdr",
				Mode:    string(d.Mode),
				Enabled: true,
				Error:   d.Err.Error(),
			},
		}, fmt.Errorf("herdr snapshot: %w", d.Err)
	}

	version, err := c.version()
	if err != nil {
		return c.failureSnapshot("connected", "", err), err
	}

	snap := Snapshot{
		Integration: domain.ExternalIntegration{
			Name:      "herdr",
			Mode:      "connected",
			Enabled:   true,
			Available: true,
			Version:   version,
			LastSync:  time.Now(),
		},
	}

	workspaces, err := c.listWorkspaces()
	if err != nil {
		return c.failureSnapshot("connected", version, err), err
	}
	snap.Workspaces = workspaces

	panes, err := c.listPanes(d.WorkspaceID)
	if err != nil {
		return c.failureSnapshot("connected", version, err), err
	}
	snap.Panes = panes

	agents, err := c.listAgents()
	if err != nil {
		return c.failureSnapshot("connected", version, err), err
	}
	snap.Agents = agents

	worktrees, err := c.listWorktrees(d.WorkspaceID)
	if err != nil {
		return c.failureSnapshot("connected", version, err), err
	}
	snap.Worktrees = worktrees

	return snap, nil
}

// version runs "herdr --version" and returns the version token (the last
// whitespace-separated token of the first line, e.g. "0.9.1").
func (c Client) version() (string, error) {
	stdout, stderr, err := c.run([]string{"--version"})
	if err != nil {
		return "", fmt.Errorf("herdr version: %w; %s", err, string(stderr))
	}
	return versionFrom(stdout), nil
}

// failureSnapshot returns a best-effort snapshot carrying the detected
// integration metadata for a failed fetch. Callers keep their own error.
func (c Client) failureSnapshot(mode, version string, err error) Snapshot {
	return Snapshot{
		Integration: domain.ExternalIntegration{
			Name:      "herdr",
			Mode:      mode,
			Enabled:   true,
			Available: true,
			Version:   version,
			Error:     err.Error(),
			LastSync:  time.Now(),
		},
	}
}

// listWorkspaces runs "herdr workspace list" and normalizes the workspaces.
func (c Client) listWorkspaces() ([]Workspace, error) {
	stdout, stderr, err := c.run([]string{"workspace", "list"})
	if err != nil {
		return nil, fmt.Errorf("herdr workspace list: %w; %s", err, string(stderr))
	}
	var env workspaceEnvelope
	if err := json.Unmarshal(stdout, &env); err != nil {
		return nil, fmt.Errorf("herdr workspace list: %w", err)
	}
	out := make([]Workspace, 0, len(env.Result.Workspaces))
	for _, r := range env.Result.Workspaces {
		out = append(out, Workspace{Name: r.Label})
	}
	return out, nil
}

// listPanes runs "herdr pane list" (scoped to the current workspace when
// HERDR_WORKSPACE_ID is set) and normalizes the panes.
func (c Client) listPanes(workspaceID string) ([]domain.PaneRef, error) {
	stdout, stderr, err := c.run(paneArgs(workspaceID))
	if err != nil {
		return nil, fmt.Errorf("herdr pane list: %w; %s", err, string(stderr))
	}
	var env paneEnvelope
	if err := json.Unmarshal(stdout, &env); err != nil {
		return nil, fmt.Errorf("herdr pane list: %w", err)
	}
	out := make([]domain.PaneRef, 0, len(env.Result.Panes))
	for _, r := range env.Result.Panes {
		out = append(out, domain.PaneRef{
			PaneID:  r.PaneID,
			AgentID: r.Agent,
			CWD:     fallback(r.CWD, r.ForegroundCWD),
			Command: r.TerminalTitle,
			Status:  r.AgentStatus,
		})
	}
	return out, nil
}

// listAgents runs "herdr agent list" and normalizes the agents.
func (c Client) listAgents() ([]domain.AgentRef, error) {
	stdout, stderr, err := c.run(agentArgs)
	if err != nil {
		return nil, fmt.Errorf("herdr agent list: %w; %s", err, string(stderr))
	}
	var env agentEnvelope
	if err := json.Unmarshal(stdout, &env); err != nil {
		return nil, fmt.Errorf("herdr agent list: %w", err)
	}
	out := make([]domain.AgentRef, 0, len(env.Result.Agents))
	for _, r := range env.Result.Agents {
		out = append(out, domain.AgentRef{
			AgentID: r.PaneID,
			Name:    fallback(r.Name, r.TerminalTitleStripped),
			Status:  r.AgentStatus,
		})
	}
	return out, nil
}

// listWorktrees runs "herdr worktree list" (scoped to the current workspace
// when HERDR_WORKSPACE_ID is set) and normalizes the worktrees.
func (c Client) listWorktrees(workspaceID string) ([]Worktree, error) {
	stdout, stderr, err := c.run(worktreeArgs(workspaceID))
	if err != nil {
		return nil, fmt.Errorf("herdr worktree list: %w; %s", err, string(stderr))
	}
	var env worktreeEnvelope
	if err := json.Unmarshal(stdout, &env); err != nil {
		return nil, fmt.Errorf("herdr worktree list: %w", err)
	}
	out := make([]Worktree, 0, len(env.Result.Worktrees))
	for _, r := range env.Result.Worktrees {
		out = append(out, Worktree{
			Path:            r.Path,
			Branch:          r.Branch,
			Label:           r.Label,
			OpenWorkspaceID: r.OpenWorkspaceID,
		})
	}
	return out, nil
}

// OpenWorktree opens an existing worktree in Herdr without focusing the pane.
// Returns a PaneRef if the worktree was already open; returns an error with
// empty snapshot if the worktree does not exist or cannot be opened.
func (c Client) OpenWorktree(ctx context.Context, req OpenWorktreeRequest) (*domain.PaneRef, error) {
	if err := c.checkHerdrAvailable(); err != nil {
		return nil, fmt.Errorf("open worktree: %w", err)
	}

	args := []string{"worktree", "open", "--path", req.Path, "--no-focus", "--json"}
	stdout, stderr, err := c.run(args)
	if err != nil {
		return nil, fmt.Errorf("herdr worktree open: %w; %s", err, string(stderr))
	}

	var env openWorktreeEnvelope
	if err := json.Unmarshal(stdout, &env); err != nil {
		return nil, fmt.Errorf("parse herdr worktree open: %w", err)
	}

	// If the worktree is not already open, return nil (user must create it first).
	if !env.Result.OpenStatus.IsOpen {
		return nil, fmt.Errorf("worktree %q does not exist or is not open", req.Path)
	}

	return &domain.PaneRef{
		PaneID: env.Result.OpenStatus.PaneID,
		CWD:    env.Result.Path,
		Status: "open",
	}, nil
}

func (c Client) checkHerdrAvailable() error {
	if c.runner == nil {
		return fmt.Errorf("no command runner available")
	}
	return nil
}

// CreateWorktree creates a new worktree in Herdr without focusing the pane.
// Returns a WorktreeRef with the created path and branch.
func (c Client) CreateWorktree(ctx context.Context, req CreateWorktreeRequest) (*domain.Worktree, error) {
	if err := c.checkHerdrAvailable(); err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}

	args := []string{"worktree", "create", "--branch", req.Branch, "--base", req.Base, "--path", req.Path, "--no-focus", "--json"}
	stdout, stderr, err := c.run(args)
	if err != nil {
		return nil, fmt.Errorf("herdr worktree create: %w; %s", err, string(stderr))
	}

	var env createWorktreeEnvelope
	if err := json.Unmarshal(stdout, &env); err != nil {
		return nil, fmt.Errorf("parse herdr worktree create: %w", err)
	}

	if !env.Result.CreateStatus.Created {
		return nil, fmt.Errorf("worktree creation failed: %s", env.Result.CreateStatus.Error)
	}

	return &domain.Worktree{
		Path:        env.Result.Path,
		Branch:      env.Result.Branch,
		CommitSHA:   "",
		IsClean:     false,
		IsLocked:    false,
		LinkedPR:    nil,
		LinkedIssue: nil,
	}, nil
}

// FocusPane focuses the given Herdr pane. Uses `herdr focus-pane` for 0.9.x
// or `herdr focus-pane` if available (future-proofing). The focus command
// does not return JSON; it returns success on exit code 0 and failure on non-zero.
func (c Client) FocusPane(ctx context.Context, paneID string) error {
	if err := c.checkHerdrAvailable(); err != nil {
		return fmt.Errorf("focus pane: %w", err)
	}

	// Try the v1 focus command first (if available), fall back to v0.9.x command.
	_, stderr, err := c.run([]string{"focus-pane", paneID})
	if err == nil {
		return nil
	}

	// Fall back to the older focus command
	_, stderr, err = c.run([]string{"pane", "focus", paneID})
	if err != nil {
		return fmt.Errorf("herdr pane focus: %w; %s", err, string(stderr))
	}

	return nil
}

// run executes a single herdr command via the CommandRunner.
func (c Client) run(args []string) ([]byte, []byte, error) {
	return c.runner.Run(herdrBinary, args...)
}

// fallback returns a when non-empty, otherwise b.
func fallback(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

// paneArgs builds the pane list args, adding --workspace when the current
// workspace ID is known (documented in herdr --skill).
func paneArgs(workspaceID string) []string {
	args := []string{"pane", "list"}
	if workspaceID != "" {
		args = append(args, "--workspace", workspaceID)
	}
	return args
}

// worktreeArgs builds the worktree list args, adding --workspace when the
// current workspace ID is known (documented in herdr --skill).
func worktreeArgs(workspaceID string) []string {
	args := []string{"worktree", "list"}
	if workspaceID != "" {
		args = append(args, "--workspace", workspaceID)
	}
	return args
}

// versionFrom returns the last whitespace token of the first line of raw.
func versionFrom(raw []byte) string {
	line := strings.Split(string(raw), "\n")[0]
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// --- raw JSON envelopes -----------------------------------------------------

// workspaceEnvelope is the "herdr workspace list" JSON output.
type workspaceEnvelope struct {
	ID     string          `json:"id"`
	Result workspaceResult `json:"result"`
}

type workspaceResult struct {
	Type       string         `json:"type"`
	Workspaces []rawWorkspace `json:"workspaces"`
}

type rawWorkspace struct {
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
}

// paneEnvelope is the "herdr pane list" JSON output.
type paneEnvelope struct {
	ID     string     `json:"id"`
	Result paneResult `json:"result"`
}

type paneResult struct {
	Type  string    `json:"type"`
	Panes []rawPane `json:"panes"`
}

type rawPane struct {
	PaneID                string `json:"pane_id"`
	CWD                   string `json:"cwd"`
	ForegroundCWD         string `json:"foreground_cwd"`
	Agent                 string `json:"agent"`
	AgentStatus           string `json:"agent_status"`
	TerminalTitle         string `json:"terminal_title"`
	TerminalTitleStripped string `json:"terminal_title_stripped"`
}

// agentEnvelope is the "herdr agent list" JSON output.
type agentEnvelope struct {
	ID     string      `json:"id"`
	Result agentResult `json:"result"`
}

type agentResult struct {
	Type   string     `json:"type"`
	Agents []rawAgent `json:"agents"`
}

type rawAgent struct {
	Agent                 string `json:"agent"`
	AgentStatus           string `json:"agent_status"`
	PaneID                string `json:"pane_id"`
	Name                  string `json:"name"`
	TerminalTitleStripped string `json:"terminal_title_stripped"`
}

// worktreeEnvelope is the "herdr worktree list" JSON output.
type worktreeEnvelope struct {
	ID     string         `json:"id"`
	Result worktreeResult `json:"result"`
}

type worktreeResult struct {
	Type      string        `json:"type"`
	Worktrees []rawWorktree `json:"worktrees"`
}

type rawWorktree struct {
	Path            string `json:"path"`
	Branch          string `json:"branch"`
	Label           string `json:"label"`
	OpenWorkspaceID string `json:"open_workspace_id"`
}

// --- request types for worktree open/create -------------------------------

// OpenWorktreeRequest describes a worktree open request to Herdr.
type OpenWorktreeRequest struct {
	Path string // Worktree path relative to repo root
}

// CreateWorktreeRequest describes a worktree create request to Herdr.
type CreateWorktreeRequest struct {
	Branch  string // Branch name for the new worktree
	Base    string // Base branch (usually main)
	Path    string // Worktree path relative to repo root
}

// --- response envelopes for worktree open/create --------------------------

// openWorktreeEnvelope is the "herdr worktree open" JSON output.
type openWorktreeEnvelope struct {
	ID     string             `json:"id"`
	Result openWorktreeResult `json:"result"`
}

type openWorktreeResult struct {
	Type       string             `json:"type"`
	OpenStatus openWorktreeStatus `json:"open_status"`
	Path       string             `json:"path"`
	Branch     string             `json:"branch"`
	Label      string             `json:"label"`
}

// openWorktreeStatus describes whether the worktree was already open.
type openWorktreeStatus struct {
	IsOpen bool   `json:"is_open"`
	PaneID string `json:"pane_id"`
}

// createWorktreeEnvelope is the "herdr worktree create" JSON output.
type createWorktreeEnvelope struct {
	ID     string               `json:"id"`
	Result createWorktreeResult `json:"result"`
}

type createWorktreeResult struct {
	Type         string               `json:"type"`
	CreateStatus createWorktreeStatus `json:"create_status"`
	Path         string               `json:"path"`
	Branch       string               `json:"branch"`
	Label        string               `json:"label"`
}

// createWorktreeStatus describes whether the worktree was created successfully.
type createWorktreeStatus struct {
	Created bool   `json:"created"`
	Error   string `json:"error"`
}
