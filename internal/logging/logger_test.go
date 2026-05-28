package logging_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m00nk0d3/grove/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogger_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "grove.log")

	logger, closer, err := logging.InitLogger(logPath)
	require.NoError(t, err)
	require.NotNil(t, logger)
	t.Cleanup(func() { _ = closer.Close() })

	logger.Info("test message", "key", "value")

	content, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(content), "test message"),
		"log file should contain the written message")
	assert.True(t, strings.Contains(string(content), `"key":"value"`),
		"log file should contain structured fields")
}

func TestLogger_CreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "subdir", "nested", "grove.log")

	logger, closer, err := logging.InitLogger(logPath)
	require.NoError(t, err)
	require.NotNil(t, logger)
	t.Cleanup(func() { _ = closer.Close() })

	_, err = os.Stat(logPath)
	assert.NoError(t, err, "log file should be created")
}

func TestLogger_RotatesAt10MB(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "grove.log")

	// Write > 10MB to the log file
	bigData := make([]byte, 11*1024*1024)
	for i := range bigData {
		bigData[i] = 'x'
	}
	err := os.WriteFile(logPath, bigData, 0o644)
	require.NoError(t, err)

	err = logging.RotateIfNeeded(logPath)
	require.NoError(t, err)

	// Original file should be renamed to grove.log.1
	_, err = os.Stat(logPath + ".1")
	assert.NoError(t, err, "rotated file should exist at grove.log.1")

	// Original path should no longer exist
	_, err = os.Stat(logPath)
	assert.True(t, os.IsNotExist(err), "original log file should be removed after rotation")
}

func TestLogger_NoRotateUnder10MB(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "grove.log")

	// Write < 10MB
	smallData := []byte("small log content")
	err := os.WriteFile(logPath, smallData, 0o644)
	require.NoError(t, err)

	err = logging.RotateIfNeeded(logPath)
	require.NoError(t, err)

	// File should still exist at original path
	_, err = os.Stat(logPath)
	assert.NoError(t, err, "small log file should not be rotated")

	// .1 file should NOT exist
	_, err = os.Stat(logPath + ".1")
	assert.True(t, os.IsNotExist(err), "rotated file should not exist for small log")
}

func TestLogger_NonExistentFile_NoRotationNeeded(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nonexistent.log")

	err := logging.RotateIfNeeded(logPath)
	assert.NoError(t, err, "should handle non-existent file gracefully")
}
