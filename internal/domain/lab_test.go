package domain_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestNewLabEntry(t *testing.T) {
	now := time.Date(2026, 9, 28, 8, 15, 30, 0, time.FixedZone("WEST", 3600))
	e := domain.NewLabEntry(domain.LabKindBug, "\r\n  Sync stalls\r\n\r\nWhen the token expires.  \n", now)

	assert.Regexp(t, regexp.MustCompile(`^20260928-071530-[0-9a-f]{6}$`), e.ID, "the ID is the UTC capture time plus a random suffix")
	assert.Equal(t, domain.LabKindBug, e.Kind)
	assert.Equal(t, "Sync stalls\n\nWhen the token expires.", e.Text)
	assert.Equal(t, domain.LabStatusDraft, e.Status)
	assert.Equal(t, now.UTC(), e.Created)
	assert.Equal(t, e.Created, e.Updated)
	assert.True(t, e.Issues.Empty())
}

func TestNewLabEntry_IDsAreUniqueWithinASecond(t *testing.T) {
	now := time.Now()
	a := domain.NewLabEntry(domain.LabKindIdea, "a", now)
	b := domain.NewLabEntry(domain.LabKindIdea, "b", now)
	assert.NotEqual(t, a.ID, b.ID)
}

func TestLabEntry_TitleAndBody(t *testing.T) {
	tests := []struct {
		name, text, title, body string
	}{
		{"single line", "Plugin API", "Plugin API", ""},
		{"title and body", "Plugin API\n\nLet users extend views.", "Plugin API", "Let users extend views."},
		{"leading blank lines", "\n\n  Plugin API  \nmore", "Plugin API", "more"},
		{"empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := domain.LabEntry{Text: tt.text}
			assert.Equal(t, tt.title, e.Title())
			assert.Equal(t, tt.body, e.Body())
		})
	}
}

func TestLabEntry_EditableAndDeletable(t *testing.T) {
	draft := domain.LabEntry{Status: domain.LabStatusDraft}
	grilling := domain.LabEntry{Status: domain.LabStatusGrilling}
	archived := domain.LabEntry{Status: domain.LabStatusPublished, Archived: true}

	assert.True(t, draft.Editable())
	assert.False(t, grilling.Editable(), "a run's input stays fixed")
	assert.True(t, draft.Deletable())
	assert.False(t, grilling.Deletable())
	assert.True(t, archived.Deletable())
}

func TestLabFilter_Matches(t *testing.T) {
	idea := domain.LabEntry{Kind: domain.LabKindIdea}
	bug := domain.LabEntry{Kind: domain.LabKindBug}

	assert.True(t, domain.LabFilterAll.Matches(idea))
	assert.True(t, domain.LabFilterAll.Matches(bug))
	assert.True(t, domain.LabFilterIdea.Matches(idea))
	assert.False(t, domain.LabFilterIdea.Matches(bug))
	assert.True(t, domain.LabFilterBug.Matches(bug))
	assert.False(t, domain.LabFilterBug.Matches(idea))
}

func TestLabKind_Valid(t *testing.T) {
	assert.True(t, domain.LabKindIdea.Valid())
	assert.True(t, domain.LabKindBug.Valid())
	assert.False(t, domain.LabKind("").Valid())
	assert.False(t, domain.LabKind("task").Valid())
}

func TestLabEntry_StagesFollowModeThenKind(t *testing.T) {
	grill := []string{"Interview", "Spec", "Tickets", "Publish"}
	shape := []string{"Shape", "Publish"}

	assert.Equal(t, grill, domain.LabEntry{Kind: domain.LabKindIdea}.LabStages())
	assert.Equal(t, shape, domain.LabEntry{Kind: domain.LabKindBug}.LabStages(), "a bug is shaped by default")
	assert.Equal(t, grill, domain.LabEntry{Kind: domain.LabKindBug, Mode: domain.LabModeGrill}.LabStages(), "an escalated bug is grilled")
}

func TestLabEntry_Stage(t *testing.T) {
	tests := []struct {
		entry domain.LabEntry
		want  int
	}{
		{domain.LabEntry{Kind: domain.LabKindIdea, Status: domain.LabStatusDraft}, 0},
		{domain.LabEntry{Kind: domain.LabKindIdea, Status: domain.LabStatusGrilling}, 1},
		{domain.LabEntry{Kind: domain.LabKindIdea, Status: domain.LabStatusSpecced}, 2},
		{domain.LabEntry{Kind: domain.LabKindIdea, Status: domain.LabStatusTicketed}, 3},
		{domain.LabEntry{Kind: domain.LabKindIdea, Mode: domain.LabModeGrill, Status: domain.LabStatusPublished}, 4},
		{domain.LabEntry{Kind: domain.LabKindBug, Status: domain.LabStatusShaping}, 1},
		{domain.LabEntry{Kind: domain.LabKindBug, Mode: domain.LabModeShape, Status: domain.LabStatusPublished}, 2},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.entry.Stage(), "%s/%s", tt.entry.Kind, tt.entry.Status)
	}
}

func TestLabEntry_ReviewFollowsContent(t *testing.T) {
	e := domain.LabEntry{}
	issue := domain.LabArtifact{Path: "issue.md", Body: "# Sync stalls"}
	assert.Equal(t, domain.LabReviewDraft, e.ReviewOf(issue), "an artifact starts as a draft")

	e.SetReview(issue, domain.LabReviewApproved)
	assert.Equal(t, domain.LabReviewApproved, e.ReviewOf(issue))

	revised := domain.LabArtifact{Path: "issue.md", Body: "# Sync stalls on token expiry"}
	assert.Equal(t, domain.LabReviewDraft, e.ReviewOf(revised), "a revision is reviewed again")

	e.SetReview(revised, domain.LabReviewDiscarded)
	assert.Equal(t, domain.LabReviewDiscarded, e.ReviewOf(revised))
	assert.Equal(t, domain.LabReviewDraft, e.ReviewOf(domain.LabArtifact{Path: "spec.md"}))
}

func TestParseIssueDraft(t *testing.T) {
	title, body := domain.ParseIssueDraft("\r\n# Sync stalls on token expiry\r\n\r\n## Summary\r\nIt stalls.\r\n")
	assert.Equal(t, "Sync stalls on token expiry", title)
	assert.Equal(t, "## Summary\nIt stalls.", body)

	title, body = domain.ParseIssueDraft("## Summary\nNo title heading.")
	assert.Empty(t, title)
	assert.Equal(t, "## Summary\nNo title heading.", body)
}
