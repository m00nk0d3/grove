package domain_test

import (
	"testing"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ticketKeys(tickets []domain.LabTicket) []string {
	var keys []string
	for _, t := range tickets {
		keys = append(keys, t.Key)
	}
	return keys
}

func TestParseLabTickets_OrdersBlockersFirst(t *testing.T) {
	raw := `{"tickets":[
		{"key":"03","title":"Sync on reconnect","body":"b3","blocked_by":["01","02"]},
		{"key":"02","title":"Show data age","body":"b2","blocked_by":["01"]},
		{"key":"01","title":"Cache the last sync","body":"b1","blocked_by":[]},
		{"key":"04","title":"Offline badge","body":"b4"}
	]}`
	tickets, err := domain.ParseLabTickets(raw)
	require.NoError(t, err)
	assert.Equal(t, []string{"01", "02", "03", "04"}, ticketKeys(tickets),
		"keys already in dependency order keep that order")
	assert.Equal(t, "Cache the last sync", tickets[0].Title)
	assert.Equal(t, []string{"01", "02"}, tickets[2].BlockedBy)

	// Keys that contradict the blocking edges are reordered so every ticket
	// follows its blockers.
	tickets, err = domain.ParseLabTickets(`{"tickets":[
		{"key":"01","title":"Integrate","blocked_by":["03"]},
		{"key":"02","title":"Other"},
		{"key":"03","title":"Expand"}
	]}`)
	require.NoError(t, err)
	assert.Equal(t, []string{"02", "03", "01"}, ticketKeys(tickets))
}

func TestParseLabTickets_RejectsUnpublishableDrafts(t *testing.T) {
	tests := map[string]string{
		"not valid":                   `{"tickets": [`,
		"has no tickets":              `{"tickets": []}`,
		"without a key":               `{"tickets":[{"title":"x"}]}`,
		"has no title":                `{"tickets":[{"key":"01","title":" "}]}`,
		"used twice":                  `{"tickets":[{"key":"01","title":"a"},{"key":"01","title":"b"}]}`,
		"blocks itself":               `{"tickets":[{"key":"01","title":"a","blocked_by":["01"]}]}`,
		"which is not a ticket":       `{"tickets":[{"key":"01","title":"a","blocked_by":["09"]}]}`,
		"block each other in a cycle": `{"tickets":[{"key":"01","title":"a","blocked_by":["02"]},{"key":"02","title":"b","blocked_by":["01"]},{"key":"03","title":"c"}]}`,
	}
	for want, raw := range tests {
		_, err := domain.ParseLabTickets(raw)
		assert.ErrorContains(t, err, want, want)
	}
}

func TestIsLabRepositoryDocument(t *testing.T) {
	for _, doc := range []string{"CONTEXT.md", "docs/adr/0003-cache.md", "src/ordering/CONTEXT.md"} {
		assert.True(t, domain.IsLabRepositoryDocument(doc), doc)
	}
	for _, draft := range []string{"spec.md", "tickets.json", "issue.md"} {
		assert.False(t, domain.IsLabRepositoryDocument(draft), draft)
	}
}
