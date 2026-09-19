package herdr

import (
	"fmt"
	"os"
)

type CommandRunner interface {
	Run(name string, args ...string) (stdout, stderr []byte, err error)
}

type Mode string

const (
	ModeStandalone Mode = "standalone"
	ModeConnected  Mode = "connected"
	ModeDegraded   Mode = "degraded"
)

type Detection struct {
	Mode        Mode
	WorkspaceID string
	TabID       string
	PaneID      string
	Err         error
}

func Detect(runner CommandRunner) Detection {
	if os.Getenv("HERDR_ENV") == "" {
		return Detection{Mode: ModeStandalone}
	}

	d := Detection{
		Mode:        ModeConnected,
		WorkspaceID: os.Getenv("HERDR_WORKSPACE_ID"),
		TabID:       os.Getenv("HERDR_TAB_ID"),
		PaneID:      os.Getenv("HERDR_PANE_ID"),
	}

	if runner == nil {
		d.Mode = ModeDegraded
		d.Err = fmt.Errorf("detect herdr: no command runner available")
		return d
	}

	_, stderr, err := runner.Run("herdr", "--version")
	if err != nil {
		d.Mode = ModeDegraded
		d.Err = fmt.Errorf("detect herdr: %w; %s", err, stderr)
	}

	return d
}
