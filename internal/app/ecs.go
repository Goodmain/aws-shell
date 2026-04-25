package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrTaskStartupTimedOut = errors.New("timed out waiting for task startup")
	ErrTaskStartupFailed   = errors.New("service task startup failed")
)

func ListAllClusters(ctx context.Context, client ECSClient) ([]string, error) {
	clusters := make([]string, 0)
	var nextToken *string
	seenTokens := make(map[string]struct{})

	for {
		arns, next, err := client.ListClusters(ctx, nextToken)
		if err != nil {
			return nil, err
		}

		clusters = append(clusters, arns...)
		if next == nil || strings.TrimSpace(*next) == "" {
			break
		}

		token := strings.TrimSpace(*next)
		if _, seen := seenTokens[token]; seen {
			return nil, fmt.Errorf("received repeated pagination token while listing ECS clusters")
		}
		seenTokens[token] = struct{}{}
		nextToken = &token
	}

	return clusters, nil
}

func ListAllServices(ctx context.Context, client ECSClient, clusterArn string) ([]string, error) {
	services := make([]string, 0)
	var nextToken *string
	seenTokens := make(map[string]struct{})

	for {
		arns, next, err := client.ListServices(ctx, clusterArn, nextToken)
		if err != nil {
			return nil, err
		}

		services = append(services, arns...)
		if next == nil || strings.TrimSpace(*next) == "" {
			break
		}

		token := strings.TrimSpace(*next)
		if _, seen := seenTokens[token]; seen {
			return nil, fmt.Errorf("received repeated pagination token while listing ECS services")
		}
		seenTokens[token] = struct{}{}
		nextToken = &token
	}

	return services, nil
}

func DescribeAllServices(ctx context.Context, client ECSClient, clusterArn string, serviceARNs []string) (map[string]ServiceDetail, error) {
	const describeBatchSize = 10

	serviceDetails := make(map[string]ServiceDetail, len(serviceARNs))
	for idx := 0; idx < len(serviceARNs); idx += describeBatchSize {
		end := idx + describeBatchSize
		if end > len(serviceARNs) {
			end = len(serviceARNs)
		}

		batch := serviceARNs[idx:end]
		details, err := client.DescribeServices(ctx, clusterArn, batch)
		if err != nil {
			return nil, err
		}

		for _, detail := range details {
			serviceDetails[detail.ARN] = detail
		}
	}

	return serviceDetails, nil
}

func DescribeClusterStats(ctx context.Context, client ECSClient, clusterARN string) (ClusterStats, error) {
	services, err := ListAllServices(ctx, client, clusterARN)
	if err != nil {
		return ClusterStats{}, err
	}

	stats := ClusterStats{ServiceCount: len(services)}
	if len(services) == 0 {
		return stats, nil
	}

	serviceDetails, err := DescribeAllServices(ctx, client, clusterARN, services)
	if err != nil {
		return ClusterStats{}, err
	}

	for _, serviceARN := range services {
		detail, ok := serviceDetails[serviceARN]
		if !ok {
			continue
		}

		stats.PendingTasks += int(detail.PendingCount)
		stats.RunningTasks += int(detail.RunningCount)
	}

	return stats, nil
}

func ListAllTasks(ctx context.Context, client ECSClient, clusterArn string, serviceName string) ([]string, error) {
	tasks := make([]string, 0)
	var nextToken *string
	seenTokens := make(map[string]struct{})

	for {
		arns, next, err := client.ListTasks(ctx, clusterArn, serviceName, nextToken)
		if err != nil {
			return nil, err
		}

		tasks = append(tasks, arns...)
		if next == nil || strings.TrimSpace(*next) == "" {
			break
		}

		token := strings.TrimSpace(*next)
		if _, seen := seenTokens[token]; seen {
			return nil, fmt.Errorf("received repeated pagination token while listing ECS tasks")
		}
		seenTokens[token] = struct{}{}
		nextToken = &token
	}

	return tasks, nil
}

func DiscoverServiceContainers(ctx context.Context, client ECSClient, cluster ClusterSelection, service ServiceSelection) ([]ContainerSelection, error) {
	serviceName := ResourceName(service.ARN)
	if serviceName == "" {
		serviceName = service.ARN
	}

	taskArns, err := ListAllTasks(ctx, client, cluster.ARN, serviceName)
	if err != nil {
		return nil, err
	}
	if len(taskArns) == 0 {
		return []ContainerSelection{}, nil
	}

	tasks, err := client.DescribeTasks(ctx, cluster.ARN, taskArns)
	if err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	containers := make([]ContainerSelection, 0)
	for _, task := range tasks {
		if !isRunning(task.LastStatus) {
			continue
		}

		for _, container := range task.Containers {
			if strings.TrimSpace(container.Name) == "" {
				continue
			}

			if _, ok := seen[container.Name]; ok {
				continue
			}

			seen[container.Name] = struct{}{}
			containers = append(containers, ContainerSelection{Name: container.Name, Label: formatContainerLabel(container.Name, container.ID)})
		}
	}

	sortContainerSelectionsByLabel(containers)

	return containers, nil
}

func DiscoverRunnableTasks(ctx context.Context, client ECSClient, cluster ClusterSelection, service ServiceSelection, container ContainerSelection) ([]TaskSelection, error) {
	serviceName := ResourceName(service.ARN)
	if serviceName == "" {
		serviceName = service.ARN
	}

	taskArns, err := ListAllTasks(ctx, client, cluster.ARN, serviceName)
	if err != nil {
		return nil, err
	}
	if len(taskArns) == 0 {
		return []TaskSelection{}, nil
	}

	tasks, err := client.DescribeTasks(ctx, cluster.ARN, taskArns)
	if err != nil {
		return nil, err
	}

	selections := make([]TaskSelection, 0)
	for _, task := range tasks {
		if !isRunning(task.LastStatus) {
			continue
		}

		if !taskHasContainer(task, container.Name) {
			continue
		}

		id := ResourceName(task.ARN)
		if id == "" {
			id = task.ARN
		}

		selections = append(selections, TaskSelection{
			ARN:      task.ARN,
			ID:       id,
			Status:   task.LastStatus,
			CPU:      task.CPU,
			Memory:   task.Memory,
			CreatedAt: preferredTaskCreatedAt(task),
			Label:    formatTaskLabel(id, task.StartedAt, time.Now()),
		})
	}

	sortTaskSelectionsByID(selections)

	return selections, nil
}

func DiscoverTaskContainers(ctx context.Context, client ECSClient, cluster ClusterSelection, taskARN string) ([]ContainerSelection, error) {
	trimmedTaskARN := strings.TrimSpace(taskARN)
	if trimmedTaskARN == "" {
		return []ContainerSelection{}, nil
	}

	tasks, err := client.DescribeTasks(ctx, cluster.ARN, []string{trimmedTaskARN})
	if err != nil {
		return nil, err
	}

	if len(tasks) == 0 {
		return []ContainerSelection{}, nil
	}

	containers := make([]ContainerSelection, 0)
	seen := make(map[string]struct{})
	for _, task := range tasks {
		if !isRunning(task.LastStatus) {
			continue
		}
		for _, container := range task.Containers {
			name := strings.TrimSpace(container.Name)
			if name == "" || !isRunning(container.LastStatus) {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			containers = append(containers, ContainerSelection{
				Name:              name,
				Image:             container.Image,
				Status:            container.LastStatus,
				CPUResource:       container.CPU,
				MemoryResource:    container.Memory,
				Label:             formatContainerLabel(name, container.ID),
			})
		}
	}

	sortContainerSelectionsByLabel(containers)
	return containers, nil
}

func StartServiceTaskAndWait(ctx context.Context, client ECSClient, cluster ClusterSelection, service ServiceSelection, serviceDetail ServiceDetail, timeout time.Duration, pollInterval time.Duration) (TaskSelection, error) {
	targetDesired := serviceDetail.DesiredCount + 1
	if targetDesired < 1 {
		targetDesired = 1
	}

	if err := client.UpdateServiceDesiredCount(ctx, cluster.ARN, service.ARN, targetDesired); err != nil {
		return TaskSelection{}, err
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	observedProgress := false
	for {
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return TaskSelection{}, ErrTaskStartupTimedOut
			}
			return TaskSelection{}, waitCtx.Err()
		default:
		}

		services, err := client.DescribeServices(waitCtx, cluster.ARN, []string{service.ARN})
		if err != nil {
			return TaskSelection{}, err
		}

		detail := serviceDetail
		for _, candidate := range services {
			if candidate.ARN == service.ARN {
				detail = candidate
				break
			}
		}

		if detail.PendingCount > 0 || detail.RunningCount > 0 {
			observedProgress = true
		}
		if detail.RunningCount > 0 {
			tasks, err := ListStartedTasks(waitCtx, client, cluster, service)
			if err != nil {
				return TaskSelection{}, err
			}
			if len(tasks) > 0 {
				return tasks[0], nil
			}
		}
		if observedProgress && detail.PendingCount == 0 && detail.RunningCount == 0 {
			return TaskSelection{}, ErrTaskStartupFailed
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return TaskSelection{}, ErrTaskStartupTimedOut
			}
			return TaskSelection{}, waitCtx.Err()
		case <-timer.C:
		}
	}
}

func ListStartedTasks(ctx context.Context, client ECSClient, cluster ClusterSelection, service ServiceSelection) ([]TaskSelection, error) {
	tasks, err := DiscoverRunnableTasks(ctx, client, cluster, service, ContainerSelection{})
	if err != nil {
		return nil, err
	}

	return tasks, nil
}

func IsServiceZeroCapacity(detail ServiceDetail) bool {
	return detail.PendingCount == 0 && detail.RunningCount == 0
}

func IsTaskStillRunnable(tasks []TaskSelection, taskARN string) bool {
	for _, task := range tasks {
		if task.ARN == taskARN {
			return true
		}
	}

	return false
}

func ServiceExecEnabled(ctx context.Context, client ECSClient, clusterArn string, serviceArn string) (bool, error) {
	services, err := client.DescribeServices(ctx, clusterArn, []string{serviceArn})
	if err != nil {
		return false, err
	}

	for _, service := range services {
		if service.ARN == serviceArn {
			return service.EnableExecuteCommand, nil
		}
	}

	return false, nil
}

func ParseProfiles(output string) []string {
	seen := make(map[string]struct{})
	profiles := make([]string, 0)

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}

		seen[trimmed] = struct{}{}
		profiles = append(profiles, trimmed)
	}

	return profiles
}

func ResourceName(arn string) string {
	trimmed := strings.TrimSpace(arn)
	if trimmed == "" {
		return ""
	}

	segments := strings.Split(trimmed, "/")
	if len(segments) == 0 {
		return trimmed
	}

	return segments[len(segments)-1]
}

func taskHasContainer(task TaskDetail, containerName string) bool {
	if strings.TrimSpace(containerName) == "" {
		return true
	}

	for _, container := range task.Containers {
		if strings.EqualFold(container.Name, containerName) && isRunning(container.LastStatus) {
			return true
		}
	}

	return false
}

func isRunning(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "running")
}

func formatRelativeStartTime(startedAt *time.Time, now time.Time) string {
	if startedAt == nil {
		return "unknown"
	}

	delta := now.Sub(*startedAt)
	if delta < 0 {
		delta = 0
	}

	if delta < time.Minute {
		seconds := int(delta.Seconds())
		if seconds <= 1 {
			return "1 second ago"
		}
		return fmt.Sprintf("%d seconds ago", seconds)
	}

	if delta < time.Hour {
		minutes := int(delta.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	}

	hours := int(delta.Hours())
	if hours == 1 {
		return "1 hour ago"
	}

	return fmt.Sprintf("%d hours ago", hours)
}

func preferredTaskCreatedAt(task TaskDetail) *time.Time {
	if task.CreatedAt != nil {
		return task.CreatedAt
	}

	return task.StartedAt
}
