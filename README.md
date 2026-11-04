# aws-shell

[![Tests](https://github.com/Goodmain/aws-shell/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/Goodmain/aws-shell/actions/workflows/ci.yml)
[![Coverage](https://codecov.io/gh/Goodmain/aws-shell/graph/badge.svg)](https://codecov.io/gh/Goodmain/aws-shell)

Interactive ECS shell-connect tool that runs entirely in Go and supports `aws-vault` and AWS IAM Identity Center (SSO) credentials.

## Prerequisites

- `aws-vault` or the AWS CLI installed and available in `PATH`
- At least one configured profile for the selected authentication provider
- IAM permissions for `ecs:ListClusters`, `ecs:ListServices`, `ecs:ListTasks`, `ecs:DescribeTasks`, and `ecs:ExecuteCommand`
- Session Manager permissions required by ECS Exec (for example `ssm:StartSession`)
- `session-manager-plugin` installed locally and available in `PATH`

## Installation

### Homebrew

```bash
brew tap Goodmain/aws-shell
brew install awsh
```

### Build from source

- Go 1.22+

```bash
go build ./cmd/aws-shell
```

## Runtime flow

1. Start in bootstrap mode.
2. The app detects `aws-vault` and the AWS CLI. If both are installed, it prompts for an authentication provider.
3. It lists profiles with `aws-vault list --profiles` or `aws configure list-profiles` and prompts for profile selection.
4. For AWS SSO, it validates the profile and runs `aws sso login --profile <profile>` when credentials are missing or expired.
5. It re-executes itself in credentialed mode through `aws-vault exec` or with `AWS_PROFILE` set for AWS SSO.
6. In AWS mode it lists ECS clusters and prompts for one cluster.
7. It prompts for service selection in a table with service status, desired, pending, running, and creation-age columns.
8. If the selected service has zero pending and active tasks, it offers to start one task and wait for startup.
9. It prompts for task selection before container selection; task rows show ID, status, CPU, memory, and created-age columns.
10. It prompts for container selection in a table with name, image, and status columns; CPU and memory resource columns are shown only when values are available.
11. The app asks for explicit confirmation before connect; choosing Cancel returns to the previous selection step.
12. It revalidates the selected task/container and runs ECS Exec with the selected provider's credentials.
13. On shell exit, control returns to the CLI with the shell exit code.

## Usage examples

Run interactive bootstrap flow:

```bash
go run ./cmd/aws-shell
```

Skip provider detection and selection:

```bash
go run ./cmd/aws-shell --auth aws-vault
go run ./cmd/aws-shell --auth aws-sso
```

Run credentialed mode directly (useful for testing):

```bash
AWS_PROFILE=sandbox go run ./cmd/aws-shell --mode aws --profile sandbox
```

## Test

```bash
go test ./...
```

## Lint

Run the same linter version as CI:

```bash
go run github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8 run
```

Optional smoke test instructions are in `test/smoke/README.md`.
