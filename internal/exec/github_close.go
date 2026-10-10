package exec

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// closeIssueRetryDelays is the backoff between close attempts: 1s, 2s, 4s for
// a maximum of 3 attempts. It is a variable so tests can stub it out.
var closeIssueRetryDelays = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

// CloseIssue closes issue number in repo through the gh CLI. reason is one of
// the values gh accepts ("completed", "not planned", "duplicate"); an empty
// reason leaves gh's default. A non-empty comment is left as the closing
// comment. Transient network failures are retried with exponential backoff;
// terminal failures (auth, permission, rate limit, not found) return
// immediately so callers can classify them.
func (w *GitHubWriter) CloseIssue(repo string, number int, reason, comment string) error {
	if strings.TrimSpace(repo) == "" {
		return fmt.Errorf("close issue: %q is not owner/name", repo)
	}
	if number <= 0 {
		return fmt.Errorf("close issue: #%d is not a valid issue number", number)
	}
	args := []string{"issue", "close", strconv.Itoa(number), "--repo", repo}
	if strings.TrimSpace(reason) != "" {
		args = append(args, "--reason", strings.TrimSpace(reason))
	}
	if strings.TrimSpace(comment) != "" {
		args = append(args, "--comment", strings.TrimSpace(comment))
	}
	// Classify the runner's raw error, before wrapping it with the issue
	// number, so the number itself can never look like an HTTP status.
	rawErr := withBackoff(closeIssueRetryDelays, func() error {
		_, err := w.runner(w.repoPath, args...)
		return err
	})
	if rawErr != nil {
		return fmt.Errorf("close issue #%d: %w", number, rawErr)
	}
	return nil
}

// ViewerCanClose reports whether the gh-authenticated viewer can close issues
// in repo. Only collaborators with push access (admin, maintain, or push) may
// trigger the close flow. Any query failure returns (false, err) so the UI
// disables the action and shows the error rather than guessing.
func (w *GitHubWriter) ViewerCanClose(repo string) (bool, error) {
	if strings.TrimSpace(repo) == "" {
		return false, fmt.Errorf("check close permission: %q is not owner/name", repo)
	}
	out, err := w.runner(w.repoPath, "api", "repos/"+repo, "--jq", ".permissions")
	if err != nil {
		return false, fmt.Errorf("check close permission: %w", err)
	}
	var permissions map[string]bool
	if err := json.Unmarshal([]byte(out), &permissions); err != nil {
		return false, fmt.Errorf("check close permission: gh returned %q", strings.TrimSpace(out))
	}
	return permissions["admin"] || permissions["maintain"] || permissions["push"], nil
}

// withBackoff runs op up to len(delays)+1 times, sleeping between attempts.
// Only network-shaped failures are retried; terminal failures return
// immediately. It returns nil on success and the last error otherwise.
func withBackoff(delays []time.Duration, op func() error) error {
	var err error
	for attempt := 0; ; attempt++ {
		if err = op(); err == nil {
			return nil
		}
		if !isRetryableGhError(err) {
			return err
		}
		if attempt >= len(delays) {
			return err
		}
		time.Sleep(delays[attempt])
	}
}

// IsRateLimitError reports whether err is a GitHub rate-limit (429) failure.
// Rate-limited closes must surface as inline errors, never retried.
func IsRateLimitError(err error) bool {
	return err != nil && containsFold(err.Error(),
		"rate limit", "too many requests", "http 429", "status code 429", "429 too many")
}

// IsPermissionError reports whether err is an auth or permission (401/403)
// failure. Permission failures must surface, never retried.
func IsPermissionError(err error) bool {
	if err == nil {
		return false
	}
	return containsFold(err.Error(),
		"permission", "unauthorized", "forbidden", "http 401", "http 403",
		"status code 401", "status code 403", "requires authentication",
		"not a collaborator", "must have push access", "resource not accessible")
}

// isNotFoundError reports whether err is a missing-repo/missing-issue failure.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return containsFold(err.Error(),
		"not found", "http 404", "status code 404", "could not resolve to an issue")
}

// isRetryableGhError reports whether err looks like a transient network or
// server failure worth retrying. Everything else — auth, permission,
// rate-limit, not-found, validation — is terminal.
func isRetryableGhError(err error) bool {
	if err == nil {
		return false
	}
	if IsRateLimitError(err) || IsPermissionError(err) || isNotFoundError(err) {
		return false
	}
	return containsFold(err.Error(),
		"timeout", "timed out", "connection reset", "connection refused",
		"connection aborted", "network unreachable", "no such host", "dns",
		"socket", "broken pipe", "eof", "temporary failure",
		"http 500", "http 502", "http 503", "http 504",
		"internal server error", "bad gateway", "service unavailable", "gateway timeout")
}

func containsFold(s string, subs ...string) bool {
	lowered := strings.ToLower(s)
	for _, sub := range subs {
		if strings.Contains(lowered, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
