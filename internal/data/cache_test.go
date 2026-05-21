package data

import (
	"testing"
	"time"

	"github.com/m00nk0d3/nexus/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheTTL_SkipsCliIfFresh(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(db *DB)
		ttl       time.Duration
		wantStale bool
	}{
		{
			name:      "empty table is always stale",
			setup:     func(db *DB) {}, // no rows inserted
			ttl:       time.Hour,
			wantStale: true,
		},
		{
			name: "just-inserted row is fresh (not stale)",
			setup: func(db *DB) {
				repo := NewGitHubRepository(db, "/repo/nexus")
				err := repo.UpsertPRs([]domain.PullRequest{
					{Number: 1, Title: "Test PR", Branch: "feat/test", Author: "user", State: "OPEN"},
				})
				require.NoError(t, err)
			},
			ttl:       time.Hour,
			wantStale: false,
		},
		{
			name: "very old synced_at is stale",
			setup: func(db *DB) {
				// Insert a row then backdated it to 2 hours ago.
				repo := NewGitHubRepository(db, "/repo/nexus")
				err := repo.UpsertPRs([]domain.PullRequest{
					{Number: 2, Title: "Old PR", Branch: "feat/old", Author: "user", State: "OPEN"},
				})
				require.NoError(t, err)
				// Manually set synced_at to 2 hours ago.
				old := time.Now().Add(-2 * time.Hour).UTC().Format("2006-01-02 15:04:05")
				_, err = db.Conn.Exec("UPDATE github_prs SET synced_at = ? WHERE repo_path = '/repo/nexus'", old)
				require.NoError(t, err)
			},
			ttl:       time.Hour,
			wantStale: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := NewDB(":memory:")
			require.NoError(t, err)
			defer db.Close()

			tt.setup(db)

			stale, err := IsCacheStale(db, CacheTablePRs, tt.ttl, "/repo/nexus")
			require.NoError(t, err)
			assert.Equal(t, tt.wantStale, stale)
		})
	}
}
