package system

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	command := ""
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			command = os.Args[i+1]
			break
		}
	}

	switch command {
	case "output-ok":
		_, _ = os.Stdout.WriteString("ok-output")
		os.Exit(0)
	case "output-fail":
		_, _ = os.Stderr.WriteString("output-error\n")
		os.Exit(2)
	case "run-ok":
		os.Exit(0)
	case "run-fail":
		_, _ = os.Stderr.WriteString("run-error\n")
		os.Exit(3)
	default:
		os.Exit(4)
	}
}

func TestNewExecRunnerAndLookPath(t *testing.T) {
	runner := NewExecRunner(os.Stderr)
	if runner == nil {
		t.Fatalf("expected runner")
	}

	path, err := runner.LookPath("go")
	if err != nil {
		t.Fatalf("expected go in PATH: %v", err)
	}
	if strings.TrimSpace(path) == "" {
		t.Fatalf("expected non-empty path")
	}
}

func TestOutputSuccess(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")

	runner := NewExecRunner(os.Stderr)
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}

	out, runErr := runner.Output(context.Background(), exe, "-test.run=^TestHelperProcess$", "--", "output-ok")
	if runErr != nil {
		t.Fatalf("unexpected error: %v", runErr)
	}
	if out != "ok-output" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestOutputReturnsStderrMessageOnExitError(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")

	runner := NewExecRunner(os.Stderr)
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}

	_, runErr := runner.Output(context.Background(), exe, "-test.run=^TestHelperProcess$", "--", "output-fail")
	if runErr == nil {
		t.Fatalf("expected error")
	}
	if runErr.Error() != "output-error\n" {
		t.Fatalf("unexpected error message: %q", runErr.Error())
	}
}

func TestRunSuccessAndExitFailure(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")

	runner := NewExecRunner(os.Stderr)
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}

	code, runErr := runner.Run(context.Background(), exe, "-test.run=^TestHelperProcess$", "--", "run-ok")
	if runErr != nil || code != 0 {
		t.Fatalf("expected successful run, got code=%d err=%v", code, runErr)
	}

	code, runErr = runner.Run(context.Background(), exe, "-test.run=^TestHelperProcess$", "--", "run-fail")
	if runErr == nil {
		t.Fatalf("expected error on non-zero exit")
	}
	if code != 3 {
		t.Fatalf("expected exit code 3, got %d", code)
	}
}

func TestRunReturnsOneForNonExitError(t *testing.T) {
	runner := NewExecRunner(os.Stderr)
	code, err := runner.Run(context.Background(), "this-command-does-not-exist-xyz")
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != 1 {
		t.Fatalf("expected code 1 for non-exit error, got %d", code)
	}

	var execErr *exec.Error
	if !errors.As(err, &execErr) {
		t.Fatalf("expected *exec.Error, got %T", err)
	}
}

func TestAttachInteractiveIOUsesProvidedAndDefaultStderr(t *testing.T) {
	cmd := exec.Command("go", "version")
	attachInteractiveIO(cmd, os.Stdout)
	if cmd.Stderr != os.Stdout {
		t.Fatalf("expected provided stderr writer")
	}

	cmd = exec.Command("go", "version")
	attachInteractiveIO(cmd, nil)
	if cmd.Stderr != os.Stderr {
		t.Fatalf("expected os.Stderr fallback")
	}
}
