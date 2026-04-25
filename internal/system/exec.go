package system

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type ExecRunner struct {
	stderr io.Writer
}

func NewExecRunner(stderr io.Writer) *ExecRunner {
	return &ExecRunner{stderr: stderr}
}

func (r *ExecRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (r *ExecRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.Output()
	if err == nil {
		return string(output), nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		if len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("%s", string(exitErr.Stderr))
		}
	}

	return "", err
}

func (r *ExecRunner) Run(ctx context.Context, name string, args ...string) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	attachInteractiveIO(cmd, r.stderr)

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), err
	}

	return 1, err
}

func attachInteractiveIO(cmd *exec.Cmd, stderr io.Writer) {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	if stderr != nil {
		cmd.Stderr = stderr
	} else {
		cmd.Stderr = os.Stderr
	}
}
