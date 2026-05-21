package fuzzy

import (
	"sort"
	"strings"

	"github.com/m00nk0d3/nexus/internal/domain"
)

// Score computes a fuzzy match score between query and candidate.
//
// Matching is case-insensitive and uses subsequence logic: every rune in query
// must appear in candidate in order, but need not be contiguous.
//
// Returns 0 when:
//   - query is empty
//   - query is not a subsequence of candidate
//
// A positive score reflects match quality:
//   - Consecutive character runs contribute run_length² to the score (match density).
//   - An earlier first-match position contributes a position bonus equal to
//     len(candidate) - firstMatchIndex.
func Score(query, candidate string) int {
	if query == "" {
		return 0
	}

	q := strings.ToLower(query)
	c := strings.ToLower(candidate)

	qRunes := []rune(q)
	cRunes := []rune(c)

	qi := 0          // index into query runes
	firstMatch := -1 // rune index of first matched character in candidate
	run := 0         // current consecutive run length
	totalScore := 0

	for ci := 0; ci < len(cRunes) && qi < len(qRunes); ci++ {
		if cRunes[ci] == qRunes[qi] {
			if firstMatch == -1 {
				firstMatch = ci
			}
			qi++
			run++
		} else {
			if run > 0 {
				totalScore += run * run
			}
			run = 0
		}
	}
	// flush any trailing run
	if run > 0 {
		totalScore += run * run
	}

	// not all query runes matched
	if qi < len(qRunes) {
		return 0
	}

	// position bonus: favour matches that start earlier
	positionBonus := len(cRunes) - firstMatch
	totalScore += positionBonus

	return totalScore
}

// FilterAndRank filters items by query and returns them sorted by score descending.
//
// When query is empty every item is returned in its original order. Otherwise
// items are scored via Score(query, item.Label); items that score 0 are dropped
// and the remainder are sorted highest-score-first.
//
// Note: scoring is performed against item.Label only; item.Sub matching is
// deferred to a later phase (e.g. searching "#42" won't surface issue #42 by number).
func FilterAndRank(query string, items []domain.SearchResult) []domain.SearchResult {
	if query == "" {
		out := make([]domain.SearchResult, len(items))
		copy(out, items)
		return out
	}

	type scored struct {
		item  domain.SearchResult
		score int
	}

	hits := make([]scored, 0, len(items))
	for _, item := range items {
		s := Score(query, item.Label)
		if s > 0 {
			hits = append(hits, scored{item, s})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		return hits[i].score > hits[j].score
	})

	out := make([]domain.SearchResult, len(hits))
	for i, h := range hits {
		out[i] = h.item
	}
	return out
}
