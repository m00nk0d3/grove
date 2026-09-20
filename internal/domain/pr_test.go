package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPullRequestAttentionReasons(t *testing.T) {
	tests := []struct {
		name string
		pr   PullRequest
		want []string
	}{
		{
			name: "authored PR aggregates actionable failures",
			pr: PullRequest{
				IsMine:            true,
				ReviewDecision:    "CHANGES_REQUESTED",
				UnresolvedThreads: 2,
				ChecksFailing:     true,
				MergeConflict:     true,
			},
			want: []string{"changes requested", "2 unresolved threads", "checks failing", "merge conflict"},
		},
		{
			name: "requested review is actionable on a teammate PR",
			pr:   PullRequest{ReviewRequested: true},
			want: []string{"review requested"},
		},
		{
			name: "teammate PR failures do not enter my attention inbox",
			pr: PullRequest{
				ReviewDecision:    "CHANGES_REQUESTED",
				UnresolvedThreads: 1,
				ChecksFailing:     true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.pr.AttentionReasons())
			assert.Equal(t, len(tt.want) > 0, tt.pr.NeedsAttention())
		})
	}
}
