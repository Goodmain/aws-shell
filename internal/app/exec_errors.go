package app

import "strings"

func FormatConnectError(profile string, err error) string {
	if err == nil {
		return ""
	}

	message := strings.TrimSpace(err.Error())
	lower := strings.ToLower(message)

	switch {
	case strings.Contains(lower, "execute command was not enabled") || strings.Contains(lower, "ecs exec") && strings.Contains(lower, "not enabled"):
		return "ECS Exec is not enabled for the selected workload. Enable ECS Exec on the service/task definition and retry."
	case strings.Contains(lower, "accessdenied") || strings.Contains(lower, "not authorized") || strings.Contains(lower, "ssm:startsession"):
		return "Access denied while starting ECS Exec. Verify IAM and Session Manager permissions for profile \"" + profile + "\"."
	case strings.Contains(lower, "session-manager-plugin") && strings.Contains(lower, "not found"):
		return "Local prerequisite missing: session-manager-plugin is required for ECS Exec. Install it and retry."
	case strings.Contains(lower, "executable file not found"):
		return "Local prerequisite missing: required executable was not found in PATH. Verify aws/aws-vault/session-manager-plugin installation."
	default:
		return message
	}
}
