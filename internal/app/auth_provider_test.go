package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSelectAuthProviderDetection(t *testing.T) {
	tests := []struct {
		name      string
		available map[string]bool
		selected  Option
		want      string
		wantErr   string
	}{
		{name: "vault only", available: map[string]bool{"aws-vault": true}, want: AuthAWSVault},
		{name: "AWS CLI only", available: map[string]bool{"aws": true}, want: AuthAWSSSO},
		{name: "both prompt", available: map[string]bool{"aws-vault": true, "aws": true}, selected: Option{Value: AuthAWSVault}, want: AuthAWSVault},
		{name: "neither", available: map[string]bool{}, wantErr: "install aws-vault or the AWS CLI"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := &fakeSelector{selected: tt.selected}
			cli := newTestCLI(Dependencies{
				Runner:   &fakeRunner{available: tt.available},
				Selector: selector,
				Stdout:   &strings.Builder{},
				Stderr:   &strings.Builder{},
			})

			provider, err := cli.selectAuthProvider("")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got error %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if provider.Name() != tt.want {
				t.Fatalf("got provider %q want %q", provider.Name(), tt.want)
			}
		})
	}
}

func TestSelectAuthProviderExplicitMissingTool(t *testing.T) {
	cli := newTestCLI(Dependencies{
		Runner:   &fakeRunner{available: map[string]bool{}},
		Selector: &fakeSelector{},
		Stdout:   &strings.Builder{},
		Stderr:   &strings.Builder{},
	})

	_, err := cli.selectAuthProvider(AuthAWSSSO)
	if err == nil || !strings.Contains(err.Error(), "AWS CLI") {
		t.Fatalf("expected actionable AWS CLI error, got %v", err)
	}
}

func TestAWSSSOProviderSkipsLoginWithValidIdentity(t *testing.T) {
	runner := &fakeRunner{outputs: map[string][]outputResult{
		"aws sts get-caller-identity --profile dev": {{output: "{}"}},
	}}

	err := (awsSSOProvider{runner: runner}).EnsureLoggedIn(context.Background(), "dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.runCalls != 0 {
		t.Fatalf("expected login to be skipped, got %d run calls", runner.runCalls)
	}
}

func TestAWSSSOProviderLogsInAndRechecksIdentity(t *testing.T) {
	runner := &fakeRunner{outputs: map[string][]outputResult{
		"aws sts get-caller-identity --profile dev": {
			{err: errors.New("expired")},
			{output: "{}"},
		},
	}}

	err := (awsSSOProvider{runner: runner}).EnsureLoggedIn(context.Background(), "dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.runName != "aws" || !reflect.DeepEqual(runner.runArgs, []string{"sso", "login", "--profile", "dev"}) {
		t.Fatalf("unexpected login command: %s %v", runner.runName, runner.runArgs)
	}
	if runner.outputCalls["aws sts get-caller-identity --profile dev"] != 2 {
		t.Fatalf("expected identity to be checked twice")
	}
}

func TestAWSSSOProviderFailsWhenIdentityStillInvalid(t *testing.T) {
	runner := &fakeRunner{outputs: map[string][]outputResult{
		"aws sts get-caller-identity --profile dev": {
			{err: errors.New("expired")},
			{err: errors.New("still expired")},
		},
	}}

	err := (awsSSOProvider{runner: runner}).EnsureLoggedIn(context.Background(), "dev")
	if err == nil || !strings.Contains(err.Error(), "still unavailable") {
		t.Fatalf("expected post-login identity error, got %v", err)
	}
}

func TestAuthProviderExecComposition(t *testing.T) {
	runner := &fakeRunner{}
	vault := (awsVaultProvider{runner: runner}).WrapExec("/tmp/aws shell", "team dev")
	if vault.Name != "aws-vault" || vault.Env != nil {
		t.Fatalf("unexpected vault command: %#v", vault)
	}
	if !reflect.DeepEqual(vault.Args, []string{"exec", "team dev", "--", "/tmp/aws shell", "--mode", "aws", "--profile", "team dev"}) {
		t.Fatalf("unexpected vault args: %v", vault.Args)
	}

	sso := (awsSSOProvider{runner: runner}).WrapExec("/tmp/aws shell", "team dev")
	if sso.Name != "/tmp/aws shell" || sso.Env["AWS_PROFILE"] != "team dev" {
		t.Fatalf("unexpected SSO command: %#v", sso)
	}
	for _, variable := range []string{"AWS_ACCESS_KEY", "AWS_ACCESS_KEY_ID", "AWS_SECRET_KEY", "AWS_SECRET_ACCESS_KEY"} {
		if value, ok := sso.Env[variable]; !ok || value != "" {
			t.Fatalf("SSO command must clear inherited %s: %#v", variable, sso.Env)
		}
	}
	if !reflect.DeepEqual(sso.Args, []string{"--mode", "aws", "--profile", "team dev"}) {
		t.Fatalf("unexpected SSO args: %v", sso.Args)
	}
}

func TestRunBootstrapAWSSSOHandoffSetsProfileEnvironment(t *testing.T) {
	runner := &fakeRunner{
		available: map[string]bool{"aws": true},
		outputs: map[string][]outputResult{
			"aws configure list-profiles":               {{output: "dev\n"}},
			"aws sts get-caller-identity --profile dev": {{output: "{}"}},
		},
	}
	cli := newTestCLI(Dependencies{
		Runner:   runner,
		Selector: &fakeSelector{},
		Stdout:   &strings.Builder{},
		Stderr:   &strings.Builder{},
	})

	code := cli.Run(context.Background(), Config{Mode: ModeBootstrap, Auth: AuthAWSSSO, Executable: "/tmp/aws-shell"})
	if code != 0 {
		t.Fatalf("expected success, got %d", code)
	}
	if runner.runName != "/tmp/aws-shell" || runner.runEnv["AWS_PROFILE"] != "dev" {
		t.Fatalf("unexpected handoff command: %s env=%v", runner.runName, runner.runEnv)
	}
}
