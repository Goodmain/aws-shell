package system

import (
	"reflect"
	"testing"
)

func TestComposeAWSVaultExecArgsPreservesArguments(t *testing.T) {
	exe := `C:\Program Files\aws-vault-ecs\app.exe`
	profile := `team sandbox`

	got := ComposeAWSVaultExecArgs(exe, profile)
	want := []string{
		"exec",
		"team sandbox",
		"--",
		`C:\Program Files\aws-vault-ecs\app.exe`,
		"--mode",
		"aws",
		"--profile",
		"team sandbox",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestComposeAWSECSExecArgsPreservesArguments(t *testing.T) {
	got := ComposeAWSECSExecArgs("cluster-arn", "task-arn", "api", "/bin/sh")
	want := []string{
		"ecs",
		"execute-command",
		"--cluster",
		"cluster-arn",
		"--task",
		"task-arn",
		"--container",
		"api",
		"--interactive",
		"--command",
		"/bin/sh",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
