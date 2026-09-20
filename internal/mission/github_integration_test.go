package mission

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGitHubIntegrationReflectsSuccessfulSync(t *testing.T) {
	syncedAt := time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC)

	state := BuildState(BuildInput{
		RepoPath:       "/repo/grove",
		GitHubLastSync: syncedAt,
	})

	assert.True(t, state.Integrations.GitHub.Available)
	assert.True(t, state.Integrations.GitHub.Enabled)
	assert.Equal(t, "connected", state.Integrations.GitHub.Mode)
	assert.Equal(t, syncedAt, state.Integrations.GitHub.LastSync)
}
