# aws-shell

Interactive ECS shell-connect tool that runs entirely in Go and uses `aws-vault` profile credentials.

## Prerequisites

- Go 1.22+
- `aws-vault` installed and available in `PATH`
- At least one configured profile in `aws-vault`
- IAM permissions for `ecs:ListClusters`, `ecs:ListServices`, `ecs:ListTasks`, `ecs:DescribeTasks`, and `ecs:ExecuteCommand`
- Session Manager permissions required by ECS Exec (for example `ssm:StartSession`)
- `session-manager-plugin` installed locally and available in `PATH`

## Build

```bash
go build ./cmd/aws-shell
```

## Runtime flow

1. Start in bootstrap mode.
2. The app checks that `aws-vault` exists.
3. It runs `aws-vault list --profiles` and prompts for profile selection.
4. It re-executes itself in credentialed mode via `aws-vault exec <profile> -- ...`.
5. In AWS mode it lists ECS clusters and prompts for one cluster.
6. It prompts for service selection in a table with service status, desired, pending, running, and creation-age columns.
7. If the selected service has zero pending and active tasks, it offers to start one task and wait for startup.
8. It prompts for task selection before container selection; task rows show ID, status, CPU, memory, and created-age columns.
9. It prompts for container selection in a table with name, image, and status columns; CPU and memory resource columns are shown only when values are available.
10. The app asks for explicit confirmation before connect; choosing Cancel returns to the previous selection step.
11. It revalidates the selected task/container and runs ECS Exec through `aws-vault exec <profile>`.
12. On shell exit, control returns to the CLI with the shell exit code.

## Usage examples

Run interactive bootstrap flow:

```bash
go run ./cmd/aws-shell
```

Run credentialed mode directly (useful for testing):

```bash
go run ./cmd/aws-shell --mode aws --profile sandbox
```

## Test

```bash
go test ./...
```

Optional smoke test instructions are in `test/smoke/README.md`.
