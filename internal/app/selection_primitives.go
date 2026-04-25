package app

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func sortOptionsByLabel(options []Option) {
	sort.SliceStable(options, func(i, j int) bool {
		return alphabeticalLess(options[i].Label, options[j].Label)
	})
}

func sortContainerSelectionsByLabel(containers []ContainerSelection) {
	sort.SliceStable(containers, func(i, j int) bool {
		return alphabeticalLess(containers[i].Label, containers[j].Label)
	})
}

func sortTaskSelectionsByID(tasks []TaskSelection) {
	sort.SliceStable(tasks, func(i, j int) bool {
		return alphabeticalLess(tasks[i].ID, tasks[j].ID)
	})
}

func alphabeticalLess(left string, right string) bool {
	leftLower := strings.ToLower(left)
	rightLower := strings.ToLower(right)
	if leftLower == rightLower {
		return left < right
	}

	return leftLower < rightLower
}

func formatClusterLabel(clusterARN string) string {
	label := ResourceName(clusterARN)
	if label == "" {
		label = clusterARN
	}

	return label
}

func formatServiceLabel(serviceName string, detail ServiceDetail) string {
	return fmt.Sprintf("%s (%d/%d)", serviceName, detail.PendingCount, detail.RunningCount)
}

func formatServiceStatus(status string) string {
	trimmed := strings.TrimSpace(status)
	if trimmed == "" {
		return "-"
	}

	return trimmed
}

func formatRelativeServiceAge(createdAt *time.Time, now time.Time) string {
	if createdAt == nil {
		return "-"
	}

	delta := now.Sub(*createdAt)
	if delta < 0 {
		delta = 0
	}

	if delta < time.Minute {
		return "<1m ago"
	}
	if delta < time.Hour {
		return fmt.Sprintf("%dm ago", int(delta.Minutes()))
	}
	if delta < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(delta.Hours()))
	}

	return fmt.Sprintf("%dd ago", int(delta.Hours()/24))
}

func formatTaskStatus(status string) string {
	trimmed := strings.TrimSpace(status)
	if trimmed == "" {
		return "-"
	}

	return trimmed
}

func formatTaskResourceValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "-"
	}

	return trimmed
}

func formatContainerStatus(status string) string {
	trimmed := strings.TrimSpace(status)
	if trimmed == "" {
		return "-"
	}

	return trimmed
}

func formatContainerResourceValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "-"
	}

	return trimmed
}

func extractContainerImageName(imageRef string) string {
	trimmed := strings.TrimSpace(imageRef)
	if trimmed == "" {
		return "-"
	}

	segment := trimmed
	if strings.Contains(segment, "/") {
		parts := strings.Split(segment, "/")
		segment = parts[len(parts)-1]
	}

	if beforeDigest, _, found := strings.Cut(segment, "@"); found {
		segment = beforeDigest
	}
	if beforeTag, _, found := strings.Cut(segment, ":"); found {
		segment = beforeTag
	}

	segment = strings.TrimSpace(segment)
	if segment == "" {
		return "-"
	}

	return segment
}

func formatRelativeTaskAge(createdAt *time.Time, now time.Time) string {
	if createdAt == nil {
		return "-"
	}

	delta := now.Sub(*createdAt)
	if delta < 0 {
		delta = 0
	}

	if delta < time.Minute {
		return "<1m ago"
	}
	if delta < time.Hour {
		return fmt.Sprintf("%dm ago", int(delta.Minutes()))
	}
	if delta < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(delta.Hours()))
	}

	return fmt.Sprintf("%dd ago", int(delta.Hours()/24))
}

func formatContainerLabel(name string, id string) string {
	containerID := strings.TrimSpace(id)
	if containerID == "" {
		containerID = "unknown"
	}

	return fmt.Sprintf("%s (%s)", name, containerID)
}

func formatTaskLabel(taskID string, startedAt *time.Time, now time.Time) string {
	return fmt.Sprintf("%s (%s)", taskID, formatRelativeStartTime(startedAt, now))
}
