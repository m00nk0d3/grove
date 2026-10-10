package exec

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubWriter_CloseIssue_WithReasonAndComment(t *testing.T) {
	gh := newFakeGh()
	w := gh.writer()

	require.NoError(t, w.CloseIssue("o/r", 7, "completed", "shipped in v2"))
	require.Len(t, gh.calls, 1)
	assert.Equal(t,
		[]string{"issue", "close", "7", "--repo", "o/r", "--reason", "completed", "--comment", "shipped in v2"},
		gh.calls[0])
}

func TestGitHubWriter_CloseIssue_WithoutReasonOrComment(t *testing.T) {
	gh := newFakeGh()
	w := gh.writer()

	require.NoError(t, w.CloseIssue("o/r", 7, "", "  "))
	require.Len(t, gh.calls, 1)
	assert.Equal(t,
		[]string{"issue", "close", "7", "--repo", "o/r"},
		gh.calls[0], "empty reason/comment add no flags")
}

func TestGitHubWriter_CloseIssue_Validation(t *testing.T) {
	gh := newFakeGh()
	w := gh.writer()

	assert.ErrorContains(t, w.CloseIssue("", 7, "", ""), "not owner/name")
	assert.ErrorContains(t, w.CloseIssue("o/r", 0, "", ""), "not a valid issue number")
	assert.Empty(t, gh.calls, "nothing is sent without repo and number")
}

func TestGitHubWriter_CloseIssue_RunnerError_NamesIssue(t *testing.T) {
	gh := newFakeGh()
	gh.errs["issue close"] = errors.New("boom")
	w := gh.writer()

	err := w.CloseIssue("o/r", 7, "", "")
	assert.ErrorContains(t, err, "close issue #7")
	assert.ErrorContains(t, err, "boom")
}

func TestGitHubWriter_CloseIssue_RetriesNetworkFailures(t *testing.T) {
	orig := closeIssueRetryDelays
	closeIssueRetryDelays = []time.Duration{0, 0}
	t.Cleanup(func() { closeIssueRetryDelays = orig })

	calls := 0
	w := NewGitHubWriterWithRunner("/repo", func(string, ...string) (string, error) {
		calls++
		if calls < 3 {
			return "", errors.New("connection reset by peer")
		}
		return "", nil
	})

	require.NoError(t, w.CloseIssue("o/r", 7, "", ""))
	assert.Equal(t, 3, calls, "flaky network failures are retried")
}

func TestGitHubWriter_CloseIssue_TerminalError_ReturnsImmediately(t *testing.T) {
	orig := closeIssueRetryDelays
	closeIssueRetryDelays = nil
	t.Cleanup(func() { closeIssueRetryDelays = orig })

	calls := 0
	w := NewGitHubWriterWithRunner("/repo", func(string, ...string) (string, error) {
		calls++
		return "", errors.New("HTTP 403: Resource not accessible by integration")
	})

	err := w.CloseIssue("o/r", 7, "", "")
	assert.ErrorContains(t, err, "close issue #7")
	assert.Equal(t, 1, calls, "permission failures are never retried")
}

func TestGitHubWriter_CloseIssue_NetworkError_ForIssue429_IsRetried(t *testing.T) {
	orig := closeIssueRetryDelays
	closeIssueRetryDelays = []time.Duration{0, 0, 0}
	t.Cleanup(func() { closeIssueRetryDelays = orig })

	// Regression: the wrapped issue number must not read as an HTTP 429.
	calls := 0
	w := NewGitHubWriterWithRunner("/repo", func(string, ...string) (string, error) {
		calls++
		return "", errors.New("connection reset by peer")
	})

	err := w.CloseIssue("o/r", 429, "", "")
	require.Error(t, err)
	assert.Equal(t, 4, calls, "three attempts plus the initial try")
}

func TestGitHubWriter_ViewerCanClose(t *testing.T) {
	for _, tt := range []struct {
		name        string
		permissions string
		want        bool
	}{
		{name: "push access", permissions: `{"admin":false,"maintain":false,"push":true,"triage":false,"pull":true}`, want: true},
		{name: "admin", permissions: `{"admin":true,"maintain":false,"push":false,"triage":false,"pull":true}`, want: true},
		{name: "read only", permissions: `{"admin":false,"maintain":false,"push":false,"triage":false,"pull":true}`, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gh := newFakeGh()
			gh.replies["api repos/o/r"] = tt.permissions
			canClose, err := gh.writer().ViewerCanClose("o/r")
			require.NoError(t, err)
			assert.Equal(t, tt.want, canClose)
			require.Len(t, gh.calls, 1)
			assert.Equal(t, []string{"api", "repos/o/r", "--jq", ".permissions"}, gh.calls[0])
		})
	}
}

func TestGitHubWriter_ViewerCanClose_Errors(t *testing.T) {
	gh := newFakeGh()
	_, err := gh.writer().ViewerCanClose("  ")
	assert.ErrorContains(t, err, "not owner/name")
	assert.Empty(t, gh.calls)

	gh.errs["api repos/o/r"] = errors.New("HTTP 404: Not Found")
	_, err = gh.writer().ViewerCanClose("o/r")
	assert.ErrorContains(t, err, "check close permission")

	gh2 := newFakeGh()
	gh2.replies["api repos/o/r"] = "not json"
	_, err = gh2.writer().ViewerCanClose("o/r")
	assert.ErrorContains(t, err, "gh returned")
}

func TestWithBackoff(t *testing.T) {
	t.Run("succeeds first try", func(t *testing.T) {
		calls := 0
		require.NoError(t, withBackoff(nil, func() error { calls++; return nil }))
		assert.Equal(t, 1, calls)
	})

	t.Run("flaky twice then succeeds", func(t *testing.T) {
		calls := 0
		err := withBackoff([]time.Duration{0, 0}, func() error {
			calls++
			if calls < 3 {
				return errors.New(" dial tcp: i/o timeout")
			}
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 3, calls)
	})

	t.Run("terminal error returns immediately", func(t *testing.T) {
		calls := 0
		err := withBackoff([]time.Duration{time.Millisecond}, func() error {
			calls++
			return errors.New("API rate limit exceeded for user")
		})
		require.Error(t, err)
		assert.Equal(t, 1, calls)
	})

	t.Run("persistent network error exhausts attempts", func(t *testing.T) {
		calls := 0
		err := withBackoff([]time.Duration{0, 0}, func() error {
			calls++
			return errors.New("no such host")
		})
		require.Error(t, err)
		assert.Equal(t, 3, calls, "initial try plus one per delay")
	})

	t.Run("server errors are retryable, client errors are not", func(t *testing.T) {
		assert.True(t, isRetryableGhError(errors.New("gh: Internal Server Error (HTTP 503)")))
		assert.False(t, isRetryableGhError(errors.New("gh: Not Found (HTTP 404)")))
		assert.False(t, isRetryableGhError(errors.New("validation failed: title is empty")))
		assert.False(t, isRetryableGhError(nil))
	})
}

func TestGhErrorClassification(t *testing.T) {
	assert.True(t, IsRateLimitError(errors.New("API rate limit exceeded (HTTP 429)")))
	assert.False(t, IsRateLimitError(errors.New("connection reset by peer")))
	assert.True(t, IsPermissionError(errors.New("HTTP 403: Resource not accessible by integration")))
	assert.True(t, IsPermissionError(errors.New("viewer is not a collaborator")))
	assert.False(t, IsPermissionError(errors.New("connection reset by peer")))
}
