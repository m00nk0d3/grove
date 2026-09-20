package main

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/herdr"
	"github.com/m00nk0d3/grove/internal/logging"
	"github.com/m00nk0d3/grove/internal/sandcastle"
	"github.com/m00nk0d3/grove/internal/updater"
	"github.com/m00nk0d3/grove/internal/version"
)

// herdrExecRunner implements herdr.CommandRunner backed by os/exec.
type herdrExecRunner struct{}

func (herdrExecRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func run() error {
	// Clean up any leftover .old binary from a previous Windows self-update.
	updater.CleanupOldBinary()
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
	herdrClient := herdr.NewClient(herdrExecRunner{})
	sandcastleBinary := m.Config.Sandcastle.Binary
	if sandcastleBinary == "" || sandcastleBinary == "sandcastle" {
		if bundledBinary, lookupErr := exec.LookPath("grove-sandcastle"); lookupErr == nil {
			sandcastleBinary = bundledBinary
		}
	}
	sandcastleClient := sandcastle.NewClient(sandcastle.ClientConfig{
		Binary:       sandcastleBinary,
		DefaultAgent: m.Config.Sandcastle.DefaultAgent,
	}, sandcastle.NewExecCommandRunner())
	m.healthChecker = &defaultHealthChecker{
		herdr:      herdrClient,
		sandcastle: sandcastleClient,
		repoPath:   cwd,
	}
	m.herdrNavigator = herdrClient
	m.workflowStarter = sandcastleClient

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
		fmt.Printf("grove version %s\n", version.Version)
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
