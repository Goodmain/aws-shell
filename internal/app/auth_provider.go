package app

import (
	"context"
	"fmt"

	"github.com/example/aws-shell/internal/system"
)

type awsVaultProvider struct {
	runner Runner
}

func (p awsVaultProvider) Name() string {
	return AuthAWSVault
}

func (p awsVaultProvider) Available() bool {
	_, err := p.runner.LookPath("aws-vault")
	return err == nil
}

func (p awsVaultProvider) ListProfiles(ctx context.Context) (string, error) {
	return p.runner.Output(ctx, "aws-vault", "list", "--profiles")
}

func (p awsVaultProvider) EnsureLoggedIn(context.Context, string) error {
	return nil
}

func (p awsVaultProvider) WrapExec(executable string, profile string) ExecCommand {
	return ExecCommand{
		Name: "aws-vault",
		Args: system.ComposeAWSVaultExecArgs(executable, profile),
	}
}

type awsSSOProvider struct {
	runner Runner
}

func (p awsSSOProvider) Name() string {
	return AuthAWSSSO
}

func (p awsSSOProvider) Available() bool {
	_, err := p.runner.LookPath("aws")
	return err == nil
}

func (p awsSSOProvider) ListProfiles(ctx context.Context) (string, error) {
	return p.runner.Output(ctx, "aws", "configure", "list-profiles")
}

func (p awsSSOProvider) EnsureLoggedIn(ctx context.Context, profile string) error {
	identityArgs := []string{"sts", "get-caller-identity", "--profile", profile}
	if _, err := p.runner.Output(ctx, "aws", identityArgs...); err == nil {
		return nil
	}

	if _, err := p.runner.Run(ctx, "aws", "sso", "login", "--profile", profile); err != nil {
		return fmt.Errorf("AWS SSO login failed for profile %q: %w", profile, err)
	}

	if _, err := p.runner.Output(ctx, "aws", identityArgs...); err != nil {
		return fmt.Errorf("AWS credentials are still unavailable for profile %q after SSO login: %w", profile, err)
	}
	return nil
}

func (p awsSSOProvider) WrapExec(executable string, profile string) ExecCommand {
	return ExecCommand{
		Name: executable,
		Args: []string{"--mode", ModeAWS, "--profile", profile},
		Env: map[string]string{
			"AWS_PROFILE":                 profile,
			"AWS_ACCESS_KEY":              "",
			"AWS_ACCESS_KEY_ID":           "",
			"AWS_SECRET_KEY":              "",
			"AWS_SECRET_ACCESS_KEY":       "",
			"AWS_SESSION_TOKEN":           "",
			"AWS_SECURITY_TOKEN":          "",
			"AWS_ROLE_ARN":                "",
			"AWS_WEB_IDENTITY_TOKEN_FILE": "",
			"AWS_ROLE_SESSION_NAME":       "",
		},
	}
}
