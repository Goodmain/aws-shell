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
	return r.RunWithEnv(ctx, nil, name, args...)
}

func (r *ExecRunner) RunWithEnv(ctx context.Context, env map[string]string, name string, args ...string) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = mergeEnv(os.Environ(), env)
	}
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

func mergeEnv(base []string, overlay map[string]string) []string {
	merged := append([]string{}, base...)
	for key, value := range overlay {
		prefix := key + "="
		replaced := false
		for i := 0; i < len(merged); i++ {
			entry := merged[i]
			if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
				if value == "" {
					merged = append(merged[:i], merged[i+1:]...)
					i--
				} else {
					merged[i] = prefix + value
				}
				replaced = true
			}
		}
		if !replaced && value != "" {
			merged = append(merged, prefix+value)
		}
	}
	return merged
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
