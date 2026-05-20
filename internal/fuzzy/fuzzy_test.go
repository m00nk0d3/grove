package fuzzy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/m00nk0d3/nexus/internal/domain"
	"github.com/m00nk0d3/nexus/internal/fuzzy"
)

// ---------------------------------------------------------------------------
// Score tests
// ---------------------------------------------------------------------------

func TestScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		query     string
		candidate string
		wantGT    int  // score must be > this value (use -1 to skip)
		wantEQ    *int // score must equal this value (nil to skip)
		wantZero  bool // score must be 0
	}{
		{
			name:      "exact match gives positive score",
			query:     "abc",
			candidate: "abc",
			wantGT:    0,
		},
		{
			name:      "subsequence match gives positive score",
			query:     "abc",
			candidate: "aXbXc",
			wantGT:    0,
		},
		{
			name:      "non-match returns zero",
			query:     "xyz",
			candidate: "abc",
			wantZero:  true,
		},
		{
			name:      "empty query returns zero",
			query:     "",
			candidate: "anything",
			wantZero:  true,
		},
		{
			name:      "case insensitive: upper query matches lower candidate",
			query:     "ABC",
			candidate: "abc",
			wantGT:    0,
		},
		{
			name:      "case insensitive: same score regardless of case",
			query:     "ABC",
			candidate: "abc",
			// verified via the consecutive-match test below
			wantGT: 0,
		},
		{
			name:      "consecutive chars score higher than spread out",
			query:     "abc",
			candidate: "abcXX",
			// score must be > Score("abc","aXbXc") — tested separately below
			wantGT: 0,
		},
		{
			name:      "query longer than candidate is no match",
			query:     "abcdef",
			candidate: "abc",
			wantZero:  true,
		},
		{
			name:      "single char match",
			query:     "a",
			candidate: "banana",
			wantGT:    0,
		},
		{
			name:      "query not a subsequence returns zero",
			query:     "ba",
			candidate: "abc", // 'b' is at index 1; 'a' must appear at index >=2 — it does not
			wantZero:  true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := fuzzy.Score(tc.query, tc.candidate)
			if tc.wantZero {
				assert.Equal(t, 0, got, "Score(%q, %q) should be 0", tc.query, tc.candidate)
				return
			}
			if tc.wantEQ != nil {
				assert.Equal(t, *tc.wantEQ, got, "Score(%q, %q) unexpected value", tc.query, tc.candidate)
				return
			}
			if tc.wantGT >= 0 {
				assert.Greater(t, got, tc.wantGT, "Score(%q, %q) should be > %d", tc.query, tc.candidate, tc.wantGT)
			}
		})
	}
}

func TestScore_CaseInsensitiveSymmetry(t *testing.T) {
	t.Parallel()
	lower := fuzzy.Score("abc", "abc")
	upper := fuzzy.Score("ABC", "abc")
	assert.Equal(t, lower, upper, "Score should be case-insensitive")
}

func TestScore_ConsecutiveBeatsSpread(t *testing.T) {
	t.Parallel()
	consecutive := fuzzy.Score("abc", "abcXX")
	spread := fuzzy.Score("abc", "aXbXc")
	assert.Greater(t, consecutive, spread,
		"consecutive match Score(%q,%q)=%d should beat spread Score(%q,%q)=%d",
		"abc", "abcXX", consecutive,
		"abc", "aXbXc", spread,
	)
}

func TestScore_NotSubsequence(t *testing.T) {
	t.Parallel()
	// "ba" is not a subsequence of "abc": 'b' is at index 1, then 'a' must
	// appear at index >=2 — it does not.
	assert.Equal(t, 0, fuzzy.Score("ba", "abc"))
}

// ---------------------------------------------------------------------------
// FilterAndRank tests
// ---------------------------------------------------------------------------

func makeResults(labels ...string) []domain.SearchResult {
	out := make([]domain.SearchResult, len(labels))
	for i, l := range labels {
		out[i] = domain.SearchResult{Label: l, Kind: domain.KindFile}
	}
	return out
}

func TestFilterAndRank_EmptyQueryReturnsAll(t *testing.T) {
	t.Parallel()
	items := makeResults("foo", "bar", "baz")
	got := fuzzy.FilterAndRank("", items)
	assert.Equal(t, items, got, "empty query should return all items in original order")
}

func TestFilterAndRank_FiltersNonMatches(t *testing.T) {
	t.Parallel()
	items := makeResults("apple", "banana", "cherry")
	got := fuzzy.FilterAndRank("ban", items)
	assert.Len(t, got, 1)
	assert.Equal(t, "banana", got[0].Label)
}

func TestFilterAndRank_SortsByScoreDescending(t *testing.T) {
	t.Parallel()
	// "abc" consecutive match beats "aXbXc" spread match
	items := makeResults("aXbXc", "abcXX")
	got := fuzzy.FilterAndRank("abc", items)
	assert.Len(t, got, 2)
	assert.Equal(t, "abcXX", got[0].Label, "consecutive match should rank first")
	assert.Equal(t, "aXbXc", got[1].Label, "spread match should rank second")
}

func TestFilterAndRank_NoMatchReturnsEmpty(t *testing.T) {
	t.Parallel()
	items := makeResults("foo", "bar")
	got := fuzzy.FilterAndRank("zzz", items)
	assert.Empty(t, got)
}

func TestFilterAndRank_EmptyItemsReturnsEmpty(t *testing.T) {
	t.Parallel()
	got := fuzzy.FilterAndRank("abc", nil)
	assert.Empty(t, got)
}

func TestFilterAndRank_EmptyQueryEmptyItemsReturnsEmpty(t *testing.T) {
	t.Parallel()
	got := fuzzy.FilterAndRank("", nil)
	assert.Empty(t, got)
}

func TestFilterAndRank_PreservesOrderOnEmptyQuery(t *testing.T) {
	t.Parallel()
	items := makeResults("zzz", "aaa", "mmm")
	got := fuzzy.FilterAndRank("", items)
	labels := make([]string, len(got))
	for i, r := range got {
		labels[i] = r.Label
	}
	assert.Equal(t, []string{"zzz", "aaa", "mmm"}, labels)
}
