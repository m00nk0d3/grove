package data

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
)

func writeLabFile(t *testing.T, s *LabStore, id, rel, content string) {
	t.Helper()
	path := filepath.Join(s.EntryDir(id), rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLabStore_QuestionsReadsCardsAnswersAndReceipts(t *testing.T) {
	s := newTestLabStore(t)
	writeLabFile(t, s, "e1", "questions/001.json", `{"id":1,"kind":"choice","question":"Keep it?","context":"c","options":["Yes","No"],"recommended":0,"why":"w"}`)
	writeLabFile(t, s, "e1", "questions/001.answer.json", `{"id":1,"choices":[1],"text":"n","answered_at":"2026-10-09T10:00:00Z"}`)
	writeLabFile(t, s, "e1", "questions/001.sent", `{"revision":0,"via":"grove","sent_at":"2026-10-09T10:00:01Z"}`)
	writeLabFile(t, s, "e1", "questions/002.json", `{ broken`)
	writeLabFile(t, s, "e1", "questions/notes.txt", `ignored`)

	records, err := s.Questions("e1")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records", len(records))
	}
	first := records[0]
	if first.Question == nil || first.Question.Question != "Keep it?" || first.Answer == nil || !reflect.DeepEqual(first.Answer.Choices, []int{1}) || first.Receipt == nil {
		t.Errorf("first = %+v", first)
	}
	if records[1].Question != nil || records[1].Err == "" {
		t.Errorf("an invalid card carries its reason: %+v", records[1])
	}
	if none, err := s.Questions("missing"); err != nil || len(none) != 0 {
		t.Errorf("an entry without questions has none: %v, %v", none, err)
	}
}

func TestLabStore_AnswerQuestionAnswersOnce(t *testing.T) {
	s := newTestLabStore(t)
	writeLabFile(t, s, "e1", "questions/001.json", `{}`)
	now := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = s.AnswerQuestion("e1", 1, []int{i % 2}, "", now)
		}(i)
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, ErrLabAlreadyAnswered):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d answers recorded, want exactly 1", won)
	}
}

func TestLabStore_AnswerQuestionRefusesACardAnsweredInThePane(t *testing.T) {
	s := newTestLabStore(t)
	writeLabFile(t, s, "e1", "questions/001.sent", `{"revision":0,"via":"pane"}`)
	if err := s.AnswerQuestion("e1", 1, []int{0}, "", time.Now()); !errors.Is(err, ErrLabAlreadyAnswered) {
		t.Errorf("err = %v", err)
	}
}

func TestLabStore_ReviseAnswerKeepsThePreviousAnswer(t *testing.T) {
	s := newTestLabStore(t)
	t0 := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	if err := s.AnswerQuestion("e1", 1, []int{0}, "first", t0); err != nil {
		t.Fatal(err)
	}
	if err := s.ReviseAnswer("e1", 1, []int{1}, "", t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var answer domain.LabAnswer
	if !readJSONFile(filepath.Join(s.EntryDir("e1"), "questions", "001.answer.json"), &answer) {
		t.Fatal("answer unreadable")
	}
	if !reflect.DeepEqual(answer.Choices, []int{1}) || len(answer.Revisions) != 1 || answer.Revisions[0].Text != "first" {
		t.Errorf("answer = %+v", answer)
	}
	if err := s.ReviseAnswer("e1", 2, nil, "x", t0); err == nil {
		t.Error("revising an unanswered question is refused")
	}
}

func TestLabStore_SendRequestNumbersRequests(t *testing.T) {
	s := newTestLabStore(t)
	allow := true
	var wg sync.WaitGroup
	numbers := make([]int, 6)
	for i := range numbers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			n, err := s.SendRequest("e1", domain.LabRequest{Kind: domain.LabRequestPermission, Allow: &allow}, time.Now())
			if err != nil {
				t.Error(err)
			}
			numbers[i] = n
		}(i)
	}
	wg.Wait()
	seen := map[int]bool{}
	for _, n := range numbers {
		if seen[n] {
			t.Errorf("number %d used twice: %v", n, numbers)
		}
		seen[n] = true
	}
	var req domain.LabRequest
	if !readJSONFile(filepath.Join(s.EntryDir("e1"), "requests", "001.json"), &req) || req.Kind != domain.LabRequestPermission || req.Allow == nil || !*req.Allow {
		t.Errorf("request = %+v", req)
	}
}

func TestLabStore_SessionReadsTheRuntimeState(t *testing.T) {
	s := newTestLabStore(t)
	if _, ok := s.Session("e1"); ok {
		t.Error("no session without session.json")
	}
	writeLabFile(t, s, "e1", "session.json", `{"phase":"fallback","stage":"interview","pending":[],"output":"Which DB?","counts":{"repairs":1,"fallbacks":2}}`)
	session, ok := s.Session("e1")
	if !ok || session.Phase != domain.LabPhaseFallback || session.Output != "Which DB?" || session.Counts.Fallbacks != 2 {
		t.Errorf("session = %+v, %v", session, ok)
	}
}
