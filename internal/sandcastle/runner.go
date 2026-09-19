package sandcastle

import (
	"bytes"
	"context"
	osexec "os/exec"
)

// CommandRunner executes external commands and returns separated stdout, stderr,
// and any error. Implementations must respect context cancellation.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
}

// execCommandRunner is the production CommandRunner backed by os/exec.
type execCommandRunner struct{}

// Run executes the named command with the given arguments. stdout and stderr are
// captured into separate buffers. The returned error is the raw exec error; callers
// are responsible for enriching it with stderr when appropriate.
func (r *execCommandRunner) Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error) {
	cmd := osexec.CommandContext(ctx, name, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err = cmd.Run()
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
}

// NewExecCommandRunner returns a CommandRunner that executes commands via os/exec.
func NewExecCommandRunner() CommandRunner {
	return &execCommandRunner{}
}
