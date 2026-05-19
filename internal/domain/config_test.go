package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGitHubConfig_SyncInterval(t *testing.T) {
	tests := []struct {
		name    string
		minutes int
		wantDur time.Duration
	}{
		{
			name:    "zero falls back to 5 minutes",
			minutes: 0,
			wantDur: 5 * time.Minute,
		},
		{
			name:    "negative falls back to 5 minutes",
			minutes: -1,
			wantDur: 5 * time.Minute,
		},
		{
			name:    "positive value is used as-is",
			minutes: 10,
			wantDur: 10 * time.Minute,
		},
		{
			name:    "one minute is the minimum valid positive value",
			minutes: 1,
			wantDur: 1 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GitHubConfig{SyncIntervalMinutes: tt.minutes}
			assert.Equal(t, tt.wantDur, cfg.SyncInterval())
		})
	}
}
