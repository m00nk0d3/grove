package data

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/pelletier/go-toml/v2"
)

// DefaultConfigPath returns the default path to the Grove config file.
// Falls back to the current directory if the home directory cannot be determined.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".grove", "config.toml")
}

// LoadConfig reads and parses the TOML config at path.
// If the file does not exist, it returns the default config.
func LoadConfig(path string) (*domain.Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := domain.DefaultConfig()
	if err := toml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

// SaveConfig marshals cfg to TOML and writes it atomically to path,
// creating parent directories as needed. It writes to a temp file first,
// then renames to the target path to avoid partial writes.
func SaveConfig(cfg *domain.Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	b, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("write config tmp: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}

	return nil
}
