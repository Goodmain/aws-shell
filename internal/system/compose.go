package system

func ComposeAWSVaultExecArgs(executablePath string, profile string) []string {
	return []string{
		"exec",
		profile,
		"--",
		executablePath,
		"--mode",
		"aws",
		"--profile",
		profile,
	}
}

func ComposeAWSECSExecArgs(cluster string, task string, container string, shellCommand string) []string {
	return []string{
		"ecs",
		"execute-command",
		"--cluster",
		cluster,
		"--task",
		task,
		"--container",
		container,
		"--interactive",
		"--command",
		shellCommand,
	}
}
