package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

const maxLogSize = 10 * 1024 * 1024 // 10MB

// InitLogger initialises a JSON slog.Logger that appends to path.
// The parent directory is created if it does not exist.
// If the log file exceeds maxLogSize, it is rotated first.
// The caller is responsible for closing the returned io.Closer when done.
func InitLogger(path string) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, fmt.Errorf("create log dir: %w", err)
	}
	if err := RotateIfNeeded(path); err != nil {
		return nil, nil, fmt.Errorf("rotate log: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}
	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(handler), f, nil
}

// DefaultLogPath returns the canonical path for the application log file.
// It falls back to a relative path when the user home directory cannot be determined.
func DefaultLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".grove", "logs", "grove.log")
	}
	return filepath.Join(home, ".grove", "logs", "grove.log")
}

// RotateIfNeeded renames path to path+".1" when the file size exceeds maxLogSize.
// It is a no-op when the file does not exist.
// Only a single backup (.log.1) is kept; the previous .log.1 is overwritten on
// each rotation.
func RotateIfNeeded(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat log file: %w", err)
	}
	if info.Size() < maxLogSize {
		return nil
	}
	rotated := path + ".1"
	// Remove old rotated file if it exists.
	_ = os.Remove(rotated)
	return os.Rename(path, rotated)
}
