package data

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/m00nk0d3/grove/internal/domain"
)

const (
	labsStoreDir = ".grove/labs" // Per-repo lab storage under ~/.grove/labs/{repo-path}
)

// LabsPath returns the path to the labs.json file for a given repository.
func LabsPath(repoPath string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	storeDir := filepath.Join(home, labsStoreDir)
	repoDir := filepath.Join(storeDir, repoPath)
	return filepath.Join(repoDir, "labs.json")
}

// LoadLabs loads Lab entries from the repository-specific labs.json file.
func LoadLabs(repoPath string) ([]domain.LabEntry, error) {
	path := LabsPath(repoPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var labEntries []domain.LabEntry
	if err := json.Unmarshal(data, &labEntries); err != nil {
		return nil, err
	}
	return labEntries, nil
}

// SaveLabs saves Lab entries to the repository-specific labs.json file.
func SaveLabs(repoPath string, labs []domain.LabEntry) error {
	path := LabsPath(repoPath)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(labs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ClearLabs removes the labs.json file for a repository.
func ClearLabs(repoPath string) error {
	path := LabsPath(repoPath)
	return os.Remove(path)
}
