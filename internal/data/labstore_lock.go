package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const labSessionLockName = "session.lock"

// LabLockOwner identifies the Grove instance holding an entry's lock.
type LabLockOwner struct {
	PID   int       `json:"pid"`
	Host  string    `json:"host"`
	Since time.Time `json:"since"`
}

// LabLockHeldError reports that another Grove instance holds an entry's lock.
type LabLockHeldError struct {
	Owner LabLockOwner
}

func (e *LabLockHeldError) Error() string {
	return fmt.Sprintf("Entry is in use by Grove on %s (pid %d)", e.Owner.Host, e.Owner.PID)
}

// LockEntry takes an entry's session lock, held while a run is started for it
// or it is being published so two Grove instances cannot duplicate that work.
// A lock left by a process that is no longer running on this host is taken
// over; alive reports whether a process on this host is running. A lock held
// elsewhere returns a *LabLockHeldError.
func (s *LabStore) LockEntry(id string, alive func(pid int) bool) (release func(), err error) {
	if id == "" {
		return nil, errors.New("lab entry has no ID")
	}
	dir := s.EntryDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create lab entry directory: %w", err)
	}
	host, _ := os.Hostname()
	path := filepath.Join(dir, labSessionLockName)
	owner := LabLockOwner{PID: os.Getpid(), Host: host, Since: time.Now().UTC()}
	raw, err := json.Marshal(owner)
	if err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, writeErr := f.Write(raw)
			closeErr := f.Close()
			if writeErr != nil || closeErr != nil {
				os.Remove(path)
				return nil, fmt.Errorf("write lab entry lock: %w", errors.Join(writeErr, closeErr))
			}
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("lock lab entry: %w", err)
		}
		held, readErr := readLabLock(path)
		if readErr != nil {
			// An unreadable lock was never finished by its writer.
			_ = os.Remove(path)
			continue
		}
		if held.Host == host && (held.PID == os.Getpid() || !alive(held.PID)) {
			_ = os.Remove(path)
			continue
		}
		return nil, &LabLockHeldError{Owner: held}
	}
	return nil, errors.New("lock lab entry: the lock changed while it was being taken; try again")
}

// ClearEntryLock removes an entry's lock whatever holds it, for a lock left
// by a Grove on another host that will not release it.
func (s *LabStore) ClearEntryLock(id string) error {
	err := os.Remove(filepath.Join(s.EntryDir(id), labSessionLockName))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear lab entry lock: %w", err)
	}
	return nil
}

func readLabLock(path string) (LabLockOwner, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return LabLockOwner{}, err
	}
	var owner LabLockOwner
	if err := json.Unmarshal(raw, &owner); err != nil {
		return LabLockOwner{}, err
	}
	return owner, nil
}

// labSessionCloseName is the file that tells an entry's session to end. The
// Sandcastle runtime watches for it.
const labSessionCloseName = "session.close"

// CloseSession tells the entry's session, if one is running, to close its
// agent pane and finish.
func (s *LabStore) CloseSession(id string) error {
	dir := s.EntryDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("close lab session: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, labSessionCloseName), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
		return fmt.Errorf("close lab session: %w", err)
	}
	return nil
}
