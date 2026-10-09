package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The shared fixtures keep Grove and the session runtime accepting exactly
// the same question cards.
func TestParseLabQuestion_MatchesSharedFixtures(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "runtime", "sandcastle", "testdata", "lab-question-cards.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Cases []struct {
			Name  string          `json:"name"`
			ID    int             `json:"id"`
			Valid bool            `json:"valid"`
			Card  json.RawMessage `json:"card"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures.Cases) == 0 {
		t.Fatal("no fixtures")
	}
	for _, c := range fixtures.Cases {
		_, err := ParseLabQuestion(c.Card, c.ID)
		if (err == nil) != c.Valid {
			t.Errorf("%s: valid = %v, want %v (err %v)", c.Name, err == nil, c.Valid, err)
		}
	}
}

func TestParseLabQuestion_ReadsRecommendations(t *testing.T) {
	q, err := ParseLabQuestion([]byte(`{"id":2,"kind":"multi","question":" Which? ","context":"c","options":["A","B","C"],"recommended":[2,0],"why":"w"}`), 2)
	if err != nil {
		t.Fatal(err)
	}
	if q.Question != "Which?" || !reflect.DeepEqual(q.Recommended, []int{2, 0}) {
		t.Errorf("got %+v", q)
	}
	q, err = ParseLabQuestion([]byte(`{"id":1,"kind":"text","question":"OS?","context":"c","recommended":"Linux","why":"w"}`), 1)
	if err != nil || q.RecommendedText != "Linux" {
		t.Errorf("got %+v, %v", q, err)
	}
}

func TestParseLabQuestion_ErrorsMatchTheRuntime(t *testing.T) {
	_, err := ParseLabQuestion([]byte(`{"id":1,"kind":"choice","question":"q","context":"c","options":["A","B"],"recommended":3,"why":"w"}`), 1)
	if err == nil || err.Error() != `"recommended" is 3 but must be an option index from 0 to 1` {
		t.Errorf("err = %v", err)
	}
}

func TestLabQuestionRecord_PendingUntilAnswered(t *testing.T) {
	q := &LabQuestion{ID: 1}
	if !(LabQuestionRecord{Question: q}).Pending() {
		t.Error("an unanswered valid card is pending")
	}
	if (LabQuestionRecord{Question: q, Answer: &LabAnswer{}}).Pending() {
		t.Error("an answered card is not pending")
	}
	inPane := LabQuestionRecord{Question: q, Receipt: &LabReceipt{Via: LabReceiptViaPane}}
	if inPane.Pending() || !inPane.AnsweredInPane() {
		t.Error("a card closed in the pane is answered there")
	}
	if (LabQuestionRecord{Err: "bad"}).Pending() {
		t.Error("an invalid card is never pending")
	}
}
