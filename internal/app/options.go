package app

import (
	"fmt"
	"strings"
	"time"
)

type ClusterStats struct {
	ServiceCount int
	PendingTasks int
	RunningTasks int
}

func BuildProfileOptions(profiles []string) []Option {
	options := make([]Option, 0, len(profiles))
	for _, profile := range profiles {
		options = append(options, Option{Label: profile, Value: profile})
	}

	sortOptionsByLabel(options)
	return options
}

func BuildClusterOptions(clusterARNs []string) []Option {
	return BuildClusterOptionsWithStats(clusterARNs, nil)
}

func BuildClusterOptionsWithStats(clusterARNs []string, statsByARN map[string]ClusterStats) []Option {
	if statsByARN == nil {
		options := make([]Option, 0, len(clusterARNs))
		for _, clusterARN := range clusterARNs {
			options = append(options, Option{Label: formatClusterLabel(clusterARN), Value: clusterARN})
		}

		sortOptionsByLabel(options)
		return options
	}

	options := make([]Option, 0, len(clusterARNs))

	nameWidth := len("Cluster")
	serviceWidth := len("Services")
	pendingWidth := len("Pending")
	runningWidth := len("Running")

	for _, clusterARN := range clusterARNs {
		label := formatClusterLabel(clusterARN)
		if len(label) > nameWidth {
			nameWidth = len(label)
		}

		stats, ok := statsByARN[clusterARN]
		if !ok {
			stats = ClusterStats{}
		}

		serviceWidth = maxInt(serviceWidth, len(fmt.Sprintf("%d", stats.ServiceCount)))
		pendingWidth = maxInt(pendingWidth, len(fmt.Sprintf("%d", stats.PendingTasks)))
		runningWidth = maxInt(runningWidth, len(fmt.Sprintf("%d", stats.RunningTasks)))
	}

	for _, clusterARN := range clusterARNs {
		clusterLabel := formatClusterLabel(clusterARN)
		stats, ok := statsByARN[clusterARN]
		if !ok {
			stats = ClusterStats{}
		}

		label := fmt.Sprintf("%-*s | %*d | %*d | %*d", nameWidth, clusterLabel, serviceWidth, stats.ServiceCount, pendingWidth, stats.PendingTasks, runningWidth, stats.RunningTasks)
		options = append(options, Option{Label: label, Value: clusterARN})
	}

	sortOptionsByLabel(options)

	header := Option{
		Label: fmt.Sprintf("%-*s | %*s | %*s | %*s", nameWidth, "Cluster", serviceWidth, "Services", pendingWidth, "Pending", runningWidth, "Running"),
		Value: WizardTableHeaderValue,
	}

	return append([]Option{header}, options...)
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}

	return right
}

func BuildServiceOptions(serviceARNs []string, detailsByARN map[string]ServiceDetail) []Option {
	return buildServiceOptionsAt(serviceARNs, detailsByARN, time.Now())
}

func buildServiceOptionsAt(serviceARNs []string, detailsByARN map[string]ServiceDetail, now time.Time) []Option {
	if detailsByARN == nil {
		options := make([]Option, 0, len(serviceARNs))
		for _, serviceARN := range serviceARNs {
			serviceName := ResourceName(serviceARN)
			if serviceName == "" {
				serviceName = serviceARN
			}
			options = append(options, Option{Label: serviceName, Value: serviceARN})
		}

		sortOptionsByLabel(options)
		return options
	}

	options := make([]Option, 0, len(serviceARNs))

	nameWidth := len("Service")
	statusWidth := len("Status")
	desiredWidth := len("Desired")
	pendingWidth := len("Pending")
	runningWidth := len("Running")
	createdWidth := len("Created")

	for _, serviceARN := range serviceARNs {
		serviceName := ResourceName(serviceARN)
		if serviceName == "" {
			serviceName = serviceARN
		}

		detail, ok := detailsByARN[serviceARN]
		if !ok {
			detail = ServiceDetail{}
		}

		nameWidth = maxInt(nameWidth, len(serviceName))
		statusWidth = maxInt(statusWidth, len(formatServiceStatus(detail.Status)))
		desiredWidth = maxInt(desiredWidth, len(fmt.Sprintf("%d", detail.DesiredCount)))
		pendingWidth = maxInt(pendingWidth, len(fmt.Sprintf("%d", detail.PendingCount)))
		runningWidth = maxInt(runningWidth, len(fmt.Sprintf("%d", detail.RunningCount)))
		createdWidth = maxInt(createdWidth, len(formatRelativeServiceAge(detail.CreatedAt, now)))
	}

	for _, serviceARN := range serviceARNs {
		serviceName := ResourceName(serviceARN)
		if serviceName == "" {
			serviceName = serviceARN
		}

		detail, ok := detailsByARN[serviceARN]
		if !ok {
			detail = ServiceDetail{}
		}

		label := fmt.Sprintf("%-*s | %-*s | %*d | %*d | %*d | %*s", nameWidth, serviceName, statusWidth, formatServiceStatus(detail.Status), desiredWidth, detail.DesiredCount, pendingWidth, detail.PendingCount, runningWidth, detail.RunningCount, createdWidth, formatRelativeServiceAge(detail.CreatedAt, now))

		options = append(options, Option{
			Label: label,
			Value: serviceARN,
		})
	}

	sortOptionsByLabel(options)

	header := Option{
		Label: fmt.Sprintf("%-*s | %-*s | %*s | %*s | %*s | %*s", nameWidth, "Service", statusWidth, "Status", desiredWidth, "Desired", pendingWidth, "Pending", runningWidth, "Running", createdWidth, "Created"),
		Value: WizardTableHeaderValue,
	}

	return append([]Option{header}, options...)
}

func BuildContainerOptions(containers []ContainerSelection) []Option {
	options := make([]Option, 0, len(containers))
	if len(containers) == 0 {
		return options
	}

	nameWidth := len("Container")
	imageWidth := len("Image")
	statusWidth := len("Status")
	cpuWidth := len("CPU")
	memoryWidth := len("Memory")
	includeResourceColumns := containerResourceColumnsEnabled(containers)

	for _, container := range containers {
		nameWidth = maxInt(nameWidth, len(strings.TrimSpace(container.Name)))
		imageWidth = maxInt(imageWidth, len(extractContainerImageName(container.Image)))
		statusWidth = maxInt(statusWidth, len(formatContainerStatus(container.Status)))
		if includeResourceColumns {
			cpuWidth = maxInt(cpuWidth, len(formatContainerResourceValue(container.CPUResource)))
			memoryWidth = maxInt(memoryWidth, len(formatContainerResourceValue(container.MemoryResource)))
		}
	}

	for _, container := range containers {
		name := strings.TrimSpace(container.Name)
		if name == "" {
			name = "-"
		}

		row := fmt.Sprintf("%-*s | %-*s | %-*s", nameWidth, name, imageWidth, extractContainerImageName(container.Image), statusWidth, formatContainerStatus(container.Status))
		if includeResourceColumns {
			row = fmt.Sprintf("%s | %*s | %*s", row, cpuWidth, formatContainerResourceValue(container.CPUResource), memoryWidth, formatContainerResourceValue(container.MemoryResource))
		}

		options = append(options, Option{Label: row, Value: container.Name})
	}

	sortOptionsByLabel(options)

	header := fmt.Sprintf("%-*s | %-*s | %-*s", nameWidth, "Container", imageWidth, "Image", statusWidth, "Status")
	if includeResourceColumns {
		header = fmt.Sprintf("%s | %*s | %*s", header, cpuWidth, "CPU", memoryWidth, "Memory")
	}

	return append([]Option{{Label: header, Value: WizardTableHeaderValue}}, options...)
}

func containerResourceColumnsEnabled(containers []ContainerSelection) bool {
	if len(containers) == 0 {
		return false
	}

	for _, container := range containers {
		if strings.TrimSpace(container.CPUResource) == "" || strings.TrimSpace(container.MemoryResource) == "" {
			return false
		}
	}

	return true
}

func BuildTaskOptions(tasks []TaskSelection) []Option {
	return buildTaskOptionsAt(tasks, time.Now())
}

func buildTaskOptionsAt(tasks []TaskSelection, now time.Time) []Option {
	options := make([]Option, 0, len(tasks))

	idWidth := len("Task")
	statusWidth := len("Status")
	cpuWidth := len("CPU")
	memoryWidth := len("Memory")
	createdWidth := len("Created")

	for _, task := range tasks {
		taskID := strings.TrimSpace(task.ID)
		if taskID == "" {
			taskID = ResourceName(task.ARN)
		}
		if taskID == "" {
			taskID = task.ARN
		}

		idWidth = maxInt(idWidth, len(taskID))
		statusWidth = maxInt(statusWidth, len(formatTaskStatus(task.Status)))
		cpuWidth = maxInt(cpuWidth, len(formatTaskResourceValue(task.CPU)))
		memoryWidth = maxInt(memoryWidth, len(formatTaskResourceValue(task.Memory)))
		createdWidth = maxInt(createdWidth, len(formatRelativeTaskAge(task.CreatedAt, now)))
	}

	for _, task := range tasks {
		taskID := strings.TrimSpace(task.ID)
		if taskID == "" {
			taskID = ResourceName(task.ARN)
		}
		if taskID == "" {
			taskID = task.ARN
		}

		label := fmt.Sprintf("%-*s | %-*s | %*s | %*s | %*s", idWidth, taskID, statusWidth, formatTaskStatus(task.Status), cpuWidth, formatTaskResourceValue(task.CPU), memoryWidth, formatTaskResourceValue(task.Memory), createdWidth, formatRelativeTaskAge(task.CreatedAt, now))
		options = append(options, Option{Label: label, Value: task.ARN})
	}

	sortOptionsByLabel(options)

	header := Option{
		Label: fmt.Sprintf("%-*s | %-*s | %*s | %*s | %*s", idWidth, "Task", statusWidth, "Status", cpuWidth, "CPU", memoryWidth, "Memory", createdWidth, "Created"),
		Value: WizardTableHeaderValue,
	}

	return append([]Option{header}, options...)
}

func DefaultOptionIndex(options []Option, defaultValue string) int {
	for idx, option := range options {
		if option.Value == defaultValue {
			return idx
		}
	}

	return 0
}
