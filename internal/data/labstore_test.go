package data

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLabStore(t *testing.T) *LabStore {
	t.Helper()
	return NewLabStore(t.TempDir())
}

func TestLabStore_LoadWithoutLabReturnsNoEntries(t *testing.T) {
	entries, err := newTestLabStore(t).Load()
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestLabStore_PutAddsAndReplaces(t *testing.T) {
	s := newTestLabStore(t)
	a := domain.NewLabEntry(domain.LabKindIdea, "Plugin API", time.Now())
	b := domain.NewLabEntry(domain.LabKindBug, "Sync stalls", time.Now())

	require.NoError(t, s.Put(a))
	require.NoError(t, s.Put(b))
	a.Text = "Plugin API\n\nWith hooks."
	require.NoError(t, s.Put(a))

	entries, err := s.Load()
	require.NoError(t, err)
	require.Len(t, entries, 2, "replacing an entry must not add a second copy")
	assert.Equal(t, a.ID, entries[0].ID, "capture order is kept")
	assert.Equal(t, "Plugin API\n\nWith hooks.", entries[0].Text)
	assert.Equal(t, b.ID, entries[1].ID)
}

func TestLabStore_PutRoundTripsEveryField(t *testing.T) {
	s := newTestLabStore(t)
	epic := 251
	e := domain.NewLabEntry(domain.LabKindIdea, "Plugin API", time.Now())
	e.Status = domain.LabStatusPublished
	e.Mode = domain.LabModeGrill
	e.Archived = true
	e.Issues = domain.LabIssues{Epic: &epic, Tickets: []int{252, 253}}
	e.Runs = []string{"run_1"}
	require.NoError(t, s.Put(e))

	entries, err := s.Load()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	got := entries[0]
	assert.Equal(t, e.Status, got.Status)
	assert.Equal(t, e.Mode, got.Mode)
	assert.True(t, got.Archived)
	require.NotNil(t, got.Issues.Epic)
	assert.Equal(t, 251, *got.Issues.Epic)
	assert.Equal(t, []int{252, 253}, got.Issues.Tickets)
	assert.Equal(t, []string{"run_1"}, got.Runs)
	assert.True(t, e.Created.Equal(got.Created))
}

func TestLabStore_PutRejectsEntryWithoutID(t *testing.T) {
	assert.Error(t, newTestLabStore(t).Put(domain.LabEntry{Text: "x"}))
}

func TestLabStore_RemoveDeletesEntryAndItsDirectory(t *testing.T) {
	s := newTestLabStore(t)
	a := domain.NewLabEntry(domain.LabKindIdea, "a", time.Now())
	b := domain.NewLabEntry(domain.LabKindIdea, "b", time.Now())
	require.NoError(t, s.Put(a))
	require.NoError(t, s.Put(b))
	require.NoError(t, os.MkdirAll(filepath.Join(s.EntryDir(a.ID), "artifacts"), 0o755))

	require.NoError(t, s.Remove(a.ID))

	entries, err := s.Load()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, b.ID, entries[0].ID)
	assert.NoDirExists(t, s.EntryDir(a.ID))
	assert.NoError(t, s.Remove("missing"), "removing an unknown entry is not an error")
}

func TestLabStore_IndexIsVersioned(t *testing.T) {
	s := newTestLabStore(t)
	require.NoError(t, s.Put(domain.NewLabEntry(domain.LabKindIdea, "a", time.Now())))

	raw, err := os.ReadFile(filepath.Join(s.Dir(), labIndexName))
	require.NoError(t, err)
	var index struct {
		Version int `json:"version"`
	}
	require.NoError(t, json.Unmarshal(raw, &index))
	assert.Equal(t, labIndexVersion, index.Version)

	require.NoError(t, os.WriteFile(filepath.Join(s.Dir(), labIndexName), []byte(`{"version":99,"entries":[]}`), 0o644))
	_, err = s.Load()
	assert.ErrorContains(t, err, "newer than this Grove supports")
}

func TestLabStore_ConcurrentPutsKeepEveryEntry(t *testing.T) {
	commonDir := t.TempDir()
	const writers = 8
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Each writer uses its own store, as separate Grove instances do.
			errs <- NewLabStore(commonDir).Put(domain.NewLabEntry(domain.LabKindIdea, fmt.Sprintf("entry %d", i), time.Now()))
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	entries, err := NewLabStore(commonDir).Load()
	require.NoError(t, err)
	assert.Len(t, entries, writers, "no write may overwrite another")
}

func TestLabStore_BreaksStaleLock(t *testing.T) {
	s := newTestLabStore(t)
	require.NoError(t, os.MkdirAll(s.Dir(), 0o755))
	lockPath := filepath.Join(s.Dir(), labIndexLock)
	require.NoError(t, os.WriteFile(lockPath, []byte("1\n"), 0o644))
	old := time.Now().Add(-2 * labLockStale)
	require.NoError(t, os.Chtimes(lockPath, old, old))

	require.NoError(t, s.Put(domain.NewLabEntry(domain.LabKindIdea, "a", time.Now())))
	assert.NoFileExists(t, lockPath, "the lock is released after the write")
}

func TestLabStore_HeldLockTimesOut(t *testing.T) {
	s := newTestLabStore(t)
	require.NoError(t, os.MkdirAll(s.Dir(), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(s.Dir(), labIndexLock), []byte("1\n"), 0o644))
	wait := labLockWait
	labLockWait = 50 * time.Millisecond
	t.Cleanup(func() { labLockWait = wait })

	err := s.Put(domain.NewLabEntry(domain.LabKindIdea, "a", time.Now()))
	assert.ErrorContains(t, err, "another Grove is writing the Lab")
}
