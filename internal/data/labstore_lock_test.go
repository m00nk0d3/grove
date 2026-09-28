package data

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func alwaysAlive(int) bool { return true }
func neverAlive(int) bool  { return false }

func writeLock(t *testing.T, s *LabStore, id string, owner LabLockOwner) {
	t.Helper()
	require.NoError(t, os.MkdirAll(s.EntryDir(id), 0o755))
	raw, err := json.Marshal(owner)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(s.EntryDir(id), labSessionLockName), raw, 0o644))
}

func TestLabStore_LockEntryIsExclusiveUntilReleased(t *testing.T) {
	s := newTestLabStore(t)
	release, err := s.LockEntry("e1", alwaysAlive)
	require.NoError(t, err)

	// Another Grove on this host with a live process is refused.
	host, _ := os.Hostname()
	writeOther := func() {
		writeLock(t, s, "e1", LabLockOwner{PID: os.Getpid() + 1, Host: host, Since: time.Now()})
	}
	release()
	writeOther()
	_, err = s.LockEntry("e1", alwaysAlive)
	var held *LabLockHeldError
	require.True(t, errors.As(err, &held))
	assert.Equal(t, os.Getpid()+1, held.Owner.PID)
	assert.Contains(t, err.Error(), "Entry is in use by Grove on "+host)

	require.NoError(t, s.ClearEntryLock("e1"))
	release, err = s.LockEntry("e1", alwaysAlive)
	require.NoError(t, err)
	release()
	assert.NoFileExists(t, filepath.Join(s.EntryDir("e1"), labSessionLockName))
}

func TestLabStore_LockEntryTakesOverDeadLocalOwner(t *testing.T) {
	s := newTestLabStore(t)
	host, _ := os.Hostname()
	writeLock(t, s, "e1", LabLockOwner{PID: 999999, Host: host, Since: time.Now().Add(-time.Hour)})

	release, err := s.LockEntry("e1", neverAlive)
	require.NoError(t, err, "a lock whose process has exited is taken over")
	release()
}

func TestLabStore_LockEntryRefusesOtherHostEvenIfPIDLooksDead(t *testing.T) {
	s := newTestLabStore(t)
	writeLock(t, s, "e1", LabLockOwner{PID: 1, Host: "another-machine", Since: time.Now()})

	_, err := s.LockEntry("e1", neverAlive)
	var held *LabLockHeldError
	require.True(t, errors.As(err, &held), "a process on another host cannot be checked, so its lock stands")
	assert.Equal(t, "another-machine", held.Owner.Host)
}

func TestLabStore_LockEntryReplacesUnreadableLock(t *testing.T) {
	s := newTestLabStore(t)
	require.NoError(t, os.MkdirAll(s.EntryDir("e1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(s.EntryDir("e1"), labSessionLockName), []byte("{"), 0o644))

	release, err := s.LockEntry("e1", alwaysAlive)
	require.NoError(t, err)
	release()
}

func TestLabStore_CloseSessionWritesMarker(t *testing.T) {
	s := newTestLabStore(t)
	require.NoError(t, s.CloseSession("e1"))
	assert.FileExists(t, filepath.Join(s.EntryDir("e1"), labSessionCloseName))
}
