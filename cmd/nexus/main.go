package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/nexus/internal/data"
	"github.com/m00nk0d3/nexus/internal/logging"
	"github.com/m00nk0d3/nexus/internal/version"
)

func run() error {
	// Initialise structured logger; non-fatal if it fails (falls back to discard).
	// Sets it as the slog default so any log.slog.Info/Warn/Error calls in the
	// codebase are automatically routed to the log file.
	logger, logCloser, err := logging.InitLogger(logging.DefaultLogPath())
	if err != nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	} else {
		defer logCloser.Close()
	}
	slog.SetDefault(logger)

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	m := NewModel()
	m.RepoPath = cwd

	// Open DB best-effort: non-fatal if it fails (e.g. no write permission).
	// When db is nil, agent runs are not logged but everything else works.
	dbPath := data.DefaultDBPath()
	// Ensure the parent directory exists before opening the DB.
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	if db, err := data.NewDB(dbPath); err == nil {
		m.db = db
		defer db.Close()
		// Best-effort cleanup: remove sessions whose processes are no longer alive.
		_ = data.DeleteDeadSessions(db)
	}

	opts := []tea.ProgramOption{tea.WithAltScreen()}

	// Git Bash (mintty) doesn't support Windows Console APIs that Bubbletea
	// uses by default on Windows. When MSYSTEM is set we're in a MINGW/MSYS2
	// environment, so open /dev/tty directly for proper PTY-based I/O.
	if os.Getenv("MSYSTEM") != "" {
		tty, ttyErr := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if ttyErr == nil {
			opts = append(opts, tea.WithInput(tty), tea.WithOutput(tty))
		}
	}

	p := tea.NewProgram(m, opts...)
	installSIGTSTP(p)
	_, err = p.Run()
	return err
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("nexus version %s\n", version.Version)
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
