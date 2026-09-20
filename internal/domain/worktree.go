package domain

import (
	"encoding/json"
	"fmt"
)

// Worktree represents a git worktree managed by Nexus
type Worktree struct {
	Path       string
	Branch     string
	CommitSHA  string
	IsClean    bool
	IsLocked   bool
	LinkedPR   *PullRequest
	LinkedIssue *Issue
}

// OpenWorktreeResult represents the response from herdr worktree open.
type OpenWorktreeResult struct {
	Type     string `json:"type"`
	RootPane struct {
		AgentStatus   string `json:"agent_status"`
		CWD           string `json:"cwd"`
		Focused       bool   `json:"focused"`
		ForegroundCwd string `json:"foreground_cwd"`
		PaneID        string `json:"pane_id"`
	} `json:"root_pane"`
	Tab struct {
		AgentStatus   string `json:"agent_status"`
		Focused       bool   `json:"focused"`
		Label         string `json:"label"`
		Number        int    `json:"number"`
		PaneCount     int    `json:"pane_count"`
		TabID         string `json:"tab_id"`
		WorkspaceID   string `json:"workspace_id"`
	} `json:"tab"`
	Worktree struct {
		Branch            string `json:"branch"`
		IsBare            bool   `json:"is_bare"`
		IsDetached        bool   `json:"is_detached"`
		IsLinkedWorktree  bool   `json:"is_linked_worktree"`
		IsPrunable        bool   `json:"is_prunable"`
		Label             string `json:"label"`
		OpenWorkspaceID   string `json:"open_workspace_id"`
		Path              string `json:"path"`
	} `json:"worktree"`
}

// CreateWorktreeResult represents the response from herdr worktree create.
type CreateWorktreeResult struct {
	Type string `json:"type"`
	RootPane struct {
		AgentStatus   string `json:"agent_status"`
		CWD           string `json:"cwd"`
		Focused       bool   `json:"focused"`
		ForegroundCwd string `json:"foreground_cwd"`
		PaneID        string `json:"pane_id"`
		Revision      int    `json:"revision"`
		Scroll struct {
			MaxOffsetFromBottom int `json:"max_offset_from_bottom"`
			OffsetFromBottom    int `json:"offset_from_bottom"`
			ViewportRows        int `json:"viewport_rows"`
		} `json:"scroll"`
		TabID          string `json:"tab_id"`
		TerminalID     string `json:"terminal_id"`
		WorkspaceID    string `json:"workspace_id"`
	} `json:"root_pane"`
	Tab struct {
		AgentStatus   string `json:"agent_status"`
		Focused       bool   `json:"focused"`
		Label         string `json:"label"`
		Number        int    `json:"number"`
		PaneCount     int    `json:"pane_count"`
		TabID         string `json:"tab_id"`
		WorkspaceID   string `json:"workspace_id"`
	} `json:"tab"`
	Workspace struct {
		ActiveTabID     string `json:"active_tab_id"`
		AgentStatus     string `json:"agent_status"`
		Focused         bool   `json:"focused"`
		Label           string `json:"label"`
		Number          int    `json:"number"`
		PaneCount       int    `json:"pane_count"`
		TabCount        int    `json:"tab_count"`
		WorkspaceID     string `json:"workspace_id"`
		Worktree struct {
			CheckoutPath   string `json:"checkout_path"`
			IsLinkedWorktree bool `json:"is_linked_worktree"`
			RepoKey        string `json:"repo_key"`
			RepoName       string `json:"repo_name"`
			RepoRoot       string `json:"repo_root"`
		} `json:"worktree"`
	} `json:"workspace"`
	Worktree struct {
		Branch            string `json:"branch"`
		IsBare            bool   `json:"is_bare"`
		IsDetached        bool   `json:"is_detached"`
		IsLinkedWorktree  bool   `json:"is_linked_worktree"`
		IsPrunable        bool   `json:"is_prunable"`
		Label             string `json:"label"`
		OpenWorkspaceID   string `json:"open_workspace_id"`
		Path              string `json:"path"`
	} `json:"worktree"`
}

// FocusPaneResult represents the response from herdr pane focus.
type FocusPaneResult struct {
	Type  string `json:"type"`
	Focus struct {
		Changed      bool   `json:"changed"`
		FocusedPaneID string `json:"focused_pane_id"`
		SourcePaneID string `json:"source_pane_id"`
	} `json:"focus"`
	Layout struct {
		Area struct {
			Height int `json:"height"`
			Width  int `json:"width"`
			X      int `json:"x"`
			Y      int `json:"y"`
		} `json:"area"`
		FocusedPaneID string `json:"focused_pane_id"`
		Panes         []struct {
			Focused bool   `json:"focused"`
			PaneID  string `json:"pane_id"`
			Rect    struct {
				Height int `json:"height"`
				Width  int `json:"width"`
				X      int `json:"x"`
				Y      int `json:"y"`
			} `json:"rect"`
		} `json:"panes"`
		Splits []struct {
			Direction string `json:"direction"`
			ID        string `json:"id"`
			Ratio     float64 `json:"ratio"`
			Rect      struct {
				Height int `json:"height"`
				Width  int `json:"width"`
				X      int `json:"x"`
				Y      int `json:"y"`
			} `json:"rect"`
		} `json:"splits"`
		TabID       string `json:"tab_id"`
		WorkspaceID string `json:"workspace_id"`
		Zoomed      bool   `json:"zoomed"`
	} `json:"layout"`
}

// ParseOpenWorktreeResult unmarshals the raw OpenWorktreeResult JSON into a struct.
func ParseOpenWorktreeResult(raw []byte) (*OpenWorktreeResult, error) {
	var result OpenWorktreeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("herdr worktree open: %w", err)
	}
	return &result, nil
}

// ParseCreateWorktreeResult unmarshals the raw CreateWorktreeResult JSON into a struct.
func ParseCreateWorktreeResult(raw []byte) (*CreateWorktreeResult, error) {
	var result CreateWorktreeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("herdr worktree create: %w", err)
	}
	return &result, nil
}

// ParseFocusPaneResult unmarshals the raw FocusPaneResult JSON into a struct.
func ParseFocusPaneResult(raw []byte) (*FocusPaneResult, error) {
	var result FocusPaneResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("herdr pane focus: %w", err)
	}
	return &result, nil
}
