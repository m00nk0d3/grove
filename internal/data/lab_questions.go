package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
)

// Files of the question protocol under an entry's directory. See
// docs/LAB_DESIGN.md, "Question protocol".
const (
	labQuestionsDir = "questions"
	labRequestsDir  = "requests"
	labSessionFile  = "session.json"
)

// ErrLabAlreadyAnswered reports an answer to a question that already has one,
// given in another Grove instance or closed as answered in the pane.
var ErrLabAlreadyAnswered = errors.New("this question has already been answered")

var labCardFile = regexp.MustCompile(`^(\d{3,})\.json$`)

func labFileName(n int, suffix string) string {
	return fmt.Sprintf("%03d%s", n, suffix)
}

// numberedFiles returns the numbers of dir's NNN.json files, ascending.
func numberedFiles(dir string) ([]int, error) {
	names, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []int
	for _, e := range names {
		if m := labCardFile.FindStringSubmatch(e.Name()); m != nil {
			n, _ := strconv.Atoi(m[1])
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out, nil
}

func readJSONFile(path string, v any) bool {
	raw, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(raw, v) == nil
}

// Questions returns an entry's question cards in order, with their answers
// and delivery receipts. An invalid card is returned with the reason.
func (s *LabStore) Questions(id string) ([]domain.LabQuestionRecord, error) {
	dir := filepath.Join(s.EntryDir(id), labQuestionsDir)
	numbers, err := numberedFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("read questions: %w", err)
	}
	records := make([]domain.LabQuestionRecord, 0, len(numbers))
	for _, n := range numbers {
		r := domain.LabQuestionRecord{Number: n}
		raw, err := os.ReadFile(filepath.Join(dir, labFileName(n, ".json")))
		if err == nil {
			r.Question, err = domain.ParseLabQuestion(raw, n)
		}
		if err != nil {
			r.Err = err.Error()
		}
		var answer domain.LabAnswer
		if readJSONFile(filepath.Join(dir, labFileName(n, ".answer.json")), &answer) {
			r.Answer = &answer
		}
		var receipt domain.LabReceipt
		if readJSONFile(filepath.Join(dir, labFileName(n, ".sent")), &receipt) {
			r.Receipt = &receipt
		}
		records = append(records, r)
	}
	return records, nil
}

// AnswerQuestion records the user's answer to question n. The answer file is
// created exclusively, so a question is answered once even across Grove
// instances; a question already answered, here or in the pane, is refused
// with ErrLabAlreadyAnswered.
func (s *LabStore) AnswerQuestion(id string, n int, choices []int, text string, now time.Time) error {
	dir := filepath.Join(s.EntryDir(id), labQuestionsDir)
	if _, err := os.Stat(filepath.Join(dir, labFileName(n, ".sent"))); err == nil {
		return ErrLabAlreadyAnswered
	}
	raw, err := json.MarshalIndent(domain.LabAnswer{ID: n, Choices: choices, Text: text, AnsweredAt: now.UTC()}, "", "  ")
	if err != nil {
		return err
	}
	if err := createExclusive(filepath.Join(dir, labFileName(n, ".answer.json")), append(raw, '\n')); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrLabAlreadyAnswered
		}
		return fmt.Errorf("record the answer: %w", err)
	}
	return nil
}

// ReviseAnswer replaces the answer to question n, keeping the previous one in
// its revisions so the agent is told what changed.
func (s *LabStore) ReviseAnswer(id string, n int, choices []int, text string, now time.Time) error {
	path := filepath.Join(s.EntryDir(id), labQuestionsDir, labFileName(n, ".answer.json"))
	var answer domain.LabAnswer
	if !readJSONFile(path, &answer) {
		return fmt.Errorf("question %d has no answer to revise", n)
	}
	answer.Revisions = append(answer.Revisions, domain.LabAnswerRevision{
		Choices: answer.Choices, Text: answer.Text, AnsweredAt: answer.AnsweredAt,
	})
	answer.Choices, answer.Text, answer.AnsweredAt = choices, text, now.UTC()
	raw, err := json.MarshalIndent(answer, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(raw, '\n'))
}

// SendRequest writes a message for the session's agent under the next free
// number and returns that number. The runtime delivers it.
func (s *LabStore) SendRequest(id string, req domain.LabRequest, now time.Time) (int, error) {
	dir := filepath.Join(s.EntryDir(id), labRequestsDir)
	for attempt := 0; attempt < 20; attempt++ {
		numbers, err := numberedFiles(dir)
		if err != nil {
			return 0, fmt.Errorf("read requests: %w", err)
		}
		n := 1
		if len(numbers) > 0 {
			n = numbers[len(numbers)-1] + 1
		}
		req.ID, req.CreatedAt = n, now.UTC()
		raw, err := json.MarshalIndent(req, "", "  ")
		if err != nil {
			return 0, err
		}
		err = createExclusive(filepath.Join(dir, labFileName(n, ".json")), append(raw, '\n'))
		if err == nil {
			return n, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return 0, fmt.Errorf("send the request: %w", err)
		}
		// Another Grove took this number first; take the next one.
	}
	return 0, errors.New("send the request: could not claim a request number")
}

// Session returns the live session state the runtime last recorded, if a
// session is running.
func (s *LabStore) Session(id string) (domain.LabSession, bool) {
	var session domain.LabSession
	ok := readJSONFile(filepath.Join(s.EntryDir(id), labSessionFile), &session)
	return session, ok
}

// createExclusive writes a new file, failing with os.ErrExist when it already
// exists. The content is written to a temporary file first and linked into
// place, so a reader never sees it half written.
func createExclusive(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(tmpPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return os.ErrExist
		}
		return err
	}
	return nil
}

// Coverage returns the interview's topic coverage, if the agent has written
// it.
func (s *LabStore) Coverage(id string) (domain.LabCoverage, bool) {
	raw, err := os.ReadFile(filepath.Join(s.EntryDir(id), "coverage.json"))
	if err != nil {
		return domain.LabCoverage{}, false
	}
	c, err := domain.ParseLabCoverage(raw)
	return c, err == nil
}
