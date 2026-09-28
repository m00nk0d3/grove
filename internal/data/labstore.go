package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
)

// LabDirName is the Lab's directory inside the git common directory.
const LabDirName = "grove-lab"

const (
	labIndexName    = "entries.json"
	labIndexLock    = "entries.lock"
	labIndexVersion = 1
)

// Lock timing for writes to the entry index. A write holds the lock for a
// read-modify-write of a small file, so a lock older than labLockStale was left
// by a process that exited without releasing it.
var (
	labLockWait  = 2 * time.Second
	labLockStale = 10 * time.Second
	labLockPoll  = 20 * time.Millisecond
)

// LabStore persists Lab entries for one repository under
// <git-common-dir>/grove-lab. The directory is shared by every worktree of the
// repository and is never tracked by git.
type LabStore struct {
	dir string
}

// NewLabStore returns the store for the repository whose git common directory
// is commonDir.
func NewLabStore(commonDir string) *LabStore {
	return &LabStore{dir: filepath.Join(commonDir, LabDirName)}
}

// Dir returns the Lab's root directory.
func (s *LabStore) Dir() string { return s.dir }

// EntryDir returns the directory that holds an entry's artifacts and lock.
func (s *LabStore) EntryDir(id string) string { return filepath.Join(s.dir, id) }

type labIndex struct {
	Version int               `json:"version"`
	Entries []domain.LabEntry `json:"entries"`
}

// Load returns every stored entry in capture order. A repository with no Lab
// yet has no entries.
func (s *LabStore) Load() ([]domain.LabEntry, error) {
	raw, err := os.ReadFile(filepath.Join(s.dir, labIndexName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read lab index: %w", err)
	}
	var index labIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		return nil, fmt.Errorf("parse lab index: %w", err)
	}
	if index.Version > labIndexVersion {
		return nil, fmt.Errorf("lab index version %d is newer than this Grove supports (%d)", index.Version, labIndexVersion)
	}
	return index.Entries, nil
}

// Put stores e, replacing the stored entry with the same ID or adding it.
func (s *LabStore) Put(e domain.LabEntry) error {
	if e.ID == "" {
		return errors.New("lab entry has no ID")
	}
	return s.update(func(entries []domain.LabEntry) []domain.LabEntry {
		for i := range entries {
			if entries[i].ID == e.ID {
				entries[i] = e
				return entries
			}
		}
		return append(entries, e)
	})
}

// Remove deletes the entry with the given ID and its directory. Removing an
// entry that does not exist is not an error.
func (s *LabStore) Remove(id string) error {
	if id == "" {
		return errors.New("lab entry has no ID")
	}
	if err := s.update(func(entries []domain.LabEntry) []domain.LabEntry {
		kept := entries[:0]
		for _, e := range entries {
			if e.ID != id {
				kept = append(kept, e)
			}
		}
		return kept
	}); err != nil {
		return err
	}
	if err := os.RemoveAll(s.EntryDir(id)); err != nil {
		return fmt.Errorf("remove lab entry directory: %w", err)
	}
	return nil
}

// update applies change to the current index under the index lock and writes
// the result atomically. Reading the index inside the lock means concurrent
// Grove instances each apply their change to the other's latest write.
func (s *LabStore) update(change func([]domain.LabEntry) []domain.LabEntry) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create lab directory: %w", err)
	}
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	entries, err := s.Load()
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(labIndex{Version: labIndexVersion, Entries: change(entries)}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode lab index: %w", err)
	}
	return writeFileAtomic(filepath.Join(s.dir, labIndexName), append(raw, '\n'))
}

// lock takes the index lock, waiting up to labLockWait and breaking a lock
// older than labLockStale.
func (s *LabStore) lock() (func(), error) {
	path := filepath.Join(s.dir, labIndexLock)
	deadline := time.Now().Add(labLockWait)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("lock lab index: %w", err)
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > labLockStale {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, errors.New("lock lab index: another Grove is writing the Lab; try again")
		}
		time.Sleep(labLockPoll)
	}
}

// writeFileAtomic writes data to a temporary file beside path and renames it
// into place, so readers never observe a partial write.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	// On Windows, replacing a file fails while another process has it open,
	// such as a second Grove reading the index; the reader holds it briefly.
	var renameErr error
	for attempt := 0; attempt < 10; attempt++ {
		if renameErr = os.Rename(tmpPath, path); renameErr == nil {
			return nil
		}
		time.Sleep(labLockPoll)
	}
	os.Remove(tmpPath)
	return fmt.Errorf("write %s: %w", filepath.Base(path), renameErr)
}
