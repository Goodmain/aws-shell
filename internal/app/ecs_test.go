package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type pagingECS struct {
	clusterPages    [][]string
	servicePages    [][]string
	taskPages       [][]string
	tasks           []TaskDetail
	services        []ServiceDetail
	clusterErrAt    int
	serviceErrAt    int
	taskErrAt       int
	describeTaskErr error
	describeSvcErr  error
	clusterCalls    int
	serviceCalls    int
	taskCalls       int
}

type batchingECS struct {
	describeCalls   int
	describeBatches [][]string
}

type repeatingClusterTokenECS struct {
	calls int
}

func (b *batchingECS) ListClusters(_ context.Context, _ *string) ([]string, *string, error) {
	return nil, nil, nil
}

func (b *batchingECS) ListServices(_ context.Context, _ string, _ *string) ([]string, *string, error) {
	return nil, nil, nil
}

func (b *batchingECS) ListTasks(_ context.Context, _ string, _ string, _ *string) ([]string, *string, error) {
	return nil, nil, nil
}

func (b *batchingECS) DescribeTasks(_ context.Context, _ string, _ []string) ([]TaskDetail, error) {
	return nil, nil
}

func (b *batchingECS) DescribeServices(_ context.Context, _ string, serviceArns []string) ([]ServiceDetail, error) {
	b.describeCalls++
	b.describeBatches = append(b.describeBatches, append([]string{}, serviceArns...))

	details := make([]ServiceDetail, 0, len(serviceArns))
	for _, arn := range serviceArns {
		details = append(details, ServiceDetail{ARN: arn, PendingCount: 1, RunningCount: 2, DesiredCount: 2})
	}

	return details, nil
}

func (b *batchingECS) UpdateServiceDesiredCount(_ context.Context, _ string, _ string, _ int32) error {
	return nil
}

func (r *repeatingClusterTokenECS) ListClusters(_ context.Context, _ *string) ([]string, *string, error) {
	r.calls++
	next := "repeat"
	return []string{"cluster"}, &next, nil
}

func (r *repeatingClusterTokenECS) ListServices(_ context.Context, _ string, _ *string) ([]string, *string, error) {
	return nil, nil, nil
}

func (r *repeatingClusterTokenECS) ListTasks(_ context.Context, _ string, _ string, _ *string) ([]string, *string, error) {
	return nil, nil, nil
}

func (r *repeatingClusterTokenECS) DescribeTasks(_ context.Context, _ string, _ []string) ([]TaskDetail, error) {
	return nil, nil
}

func (r *repeatingClusterTokenECS) DescribeServices(_ context.Context, _ string, _ []string) ([]ServiceDetail, error) {
	return nil, nil
}

func (r *repeatingClusterTokenECS) UpdateServiceDesiredCount(_ context.Context, _ string, _ string, _ int32) error {
	return nil
}

func (p *pagingECS) ListClusters(_ context.Context, _ *string) ([]string, *string, error) {
	p.clusterCalls++
	if p.clusterErrAt > 0 && p.clusterCalls == p.clusterErrAt {
		return nil, nil, errors.New("cluster api error")
	}
	if p.clusterCalls > len(p.clusterPages) {
		return nil, nil, nil
	}
	page := p.clusterPages[p.clusterCalls-1]
	if p.clusterCalls < len(p.clusterPages) {
		next := "next"
		return page, &next, nil
	}
	return page, nil, nil
}

func (p *pagingECS) ListServices(_ context.Context, _ string, _ *string) ([]string, *string, error) {
	p.serviceCalls++
	if p.serviceErrAt > 0 && p.serviceCalls == p.serviceErrAt {
		return nil, nil, errors.New("service api error")
	}
	if p.serviceCalls > len(p.servicePages) {
		return nil, nil, nil
	}
	page := p.servicePages[p.serviceCalls-1]
	if p.serviceCalls < len(p.servicePages) {
		next := "next"
		return page, &next, nil
	}
	return page, nil, nil
}

func (p *pagingECS) ListTasks(_ context.Context, _ string, _ string, _ *string) ([]string, *string, error) {
	p.taskCalls++
	if p.taskErrAt > 0 && p.taskCalls == p.taskErrAt {
		return nil, nil, errors.New("task api error")
	}
	if p.taskCalls > len(p.taskPages) {
		return nil, nil, nil
	}
	page := p.taskPages[p.taskCalls-1]
	if p.taskCalls < len(p.taskPages) {
		next := "next"
		return page, &next, nil
	}
	return page, nil, nil
}

func (p *pagingECS) DescribeTasks(_ context.Context, _ string, _ []string) ([]TaskDetail, error) {
	if p.describeTaskErr != nil {
		return nil, p.describeTaskErr
	}

	return p.tasks, nil
}

func (p *pagingECS) DescribeServices(_ context.Context, _ string, _ []string) ([]ServiceDetail, error) {
	if p.describeSvcErr != nil {
		return nil, p.describeSvcErr
	}

	return p.services, nil
}

func (p *pagingECS) UpdateServiceDesiredCount(_ context.Context, _ string, _ string, _ int32) error {
	return nil
}

func TestListAllClustersPaginates(t *testing.T) {
	client := &pagingECS{clusterPages: [][]string{{"a", "b"}, {"c"}}}
	got, err := ListAllClusters(context.Background(), client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestListAllClustersReturnsAPIErrors(t *testing.T) {
	client := &pagingECS{clusterPages: [][]string{{"a"}}, clusterErrAt: 1}
	_, err := ListAllClusters(context.Background(), client)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestListAllClustersFailsOnRepeatedPaginationToken(t *testing.T) {
	client := &repeatingClusterTokenECS{}
	_, err := ListAllClusters(context.Background(), client)
	if err == nil {
		t.Fatalf("expected repeated token error")
	}
	if !strings.Contains(err.Error(), "repeated pagination token") {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("expected early stop after repeated token, got %d calls", client.calls)
	}
}

func TestListAllServicesPaginates(t *testing.T) {
	client := &pagingECS{servicePages: [][]string{{"svc1"}, {"svc2", "svc3"}}}
	got, err := ListAllServices(context.Background(), client, "cluster")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"svc1", "svc2", "svc3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestListAllServicesReturnsAPIErrors(t *testing.T) {
	client := &pagingECS{servicePages: [][]string{{"svc1"}}, serviceErrAt: 1}
	_, err := ListAllServices(context.Background(), client, "cluster")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestListAllTasksPaginates(t *testing.T) {
	client := &pagingECS{taskPages: [][]string{{"task1"}, {"task2"}}}
	got, err := ListAllTasks(context.Background(), client, "cluster", "service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"task1", "task2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestListAllTasksReturnsAPIErrors(t *testing.T) {
	client := &pagingECS{taskPages: [][]string{{"task1"}}, taskErrAt: 1}
	_, err := ListAllTasks(context.Background(), client, "cluster", "service")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestDescribeAllServicesBatchesRequests(t *testing.T) {
	client := &batchingECS{}
	serviceARNs := []string{
		"svc-1", "svc-2", "svc-3", "svc-4", "svc-5",
		"svc-6", "svc-7", "svc-8", "svc-9", "svc-10",
		"svc-11",
	}

	details, err := DescribeAllServices(context.Background(), client, "cluster", serviceARNs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.describeCalls != 2 {
		t.Fatalf("got %d describe calls want %d", client.describeCalls, 2)
	}
	if len(client.describeBatches[0]) != 10 || len(client.describeBatches[1]) != 1 {
		t.Fatalf("unexpected batch sizes: %#v", client.describeBatches)
	}
	if len(details) != len(serviceARNs) {
		t.Fatalf("got %d details want %d", len(details), len(serviceARNs))
	}
}

func TestDescribeClusterStatsAggregatesServiceAndTaskCounts(t *testing.T) {
	client := &pagingECS{
		servicePages: [][]string{{"svc-a", "svc-b"}},
		services: []ServiceDetail{
			{ARN: "svc-a", PendingCount: 1, RunningCount: 2},
			{ARN: "svc-b", PendingCount: 3, RunningCount: 4},
		},
	}

	got, err := DescribeClusterStats(context.Background(), client, "cluster")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := ClusterStats{ServiceCount: 2, PendingTasks: 4, RunningTasks: 6}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestDiscoverServiceContainersDeduplicates(t *testing.T) {
	client := &pagingECS{
		taskPages: [][]string{{"task-1", "task-2"}},
		tasks: []TaskDetail{
			{ARN: "task-1", LastStatus: "RUNNING", Containers: []ContainerDetail{{Name: "api", LastStatus: "RUNNING"}}},
			{ARN: "task-2", LastStatus: "RUNNING", Containers: []ContainerDetail{{Name: "api", LastStatus: "RUNNING"}, {Name: "worker", LastStatus: "RUNNING"}}},
		},
	}

	got, err := DiscoverServiceContainers(context.Background(), client, ClusterSelection{ARN: "cluster"}, ServiceSelection{ARN: "arn:...:service/dev/api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []ContainerSelection{{Name: "api", Label: "api (unknown)"}, {Name: "worker", Label: "worker (unknown)"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestDiscoverServiceContainersUsesContainerIDsInLabels(t *testing.T) {
	client := &pagingECS{
		taskPages: [][]string{{"task-1", "task-2"}},
		tasks: []TaskDetail{
			{ARN: "task-1", LastStatus: "RUNNING", Containers: []ContainerDetail{{Name: "api", ID: "c1", LastStatus: "RUNNING"}}},
			{ARN: "task-2", LastStatus: "RUNNING", Containers: []ContainerDetail{{Name: "api", ID: "c2", LastStatus: "RUNNING"}}},
		},
	}

	got, err := DiscoverServiceContainers(context.Background(), client, ClusterSelection{ARN: "cluster"}, ServiceSelection{ARN: "service"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected deduplicated container options, got %#v", got)
	}
	if got[0].Label != "api (c1)" {
		t.Fatalf("got %q want %q", got[0].Label, "api (c1)")
	}
}

func TestDiscoverServiceContainersNoResults(t *testing.T) {
	client := &pagingECS{taskPages: [][]string{{}}}
	got, err := DiscoverServiceContainers(context.Background(), client, ClusterSelection{ARN: "cluster"}, ServiceSelection{ARN: "service"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty containers, got %v", got)
	}
}

func TestDiscoverTaskContainersIncludesImageStatusAndOptionalResources(t *testing.T) {
	client := &pagingECS{
		tasks: []TaskDetail{{
			ARN:        "task-1",
			LastStatus: "RUNNING",
			Containers: []ContainerDetail{
				{Name: "api", LastStatus: "RUNNING", Image: "repo/api:latest", CPU: "10%", Memory: "25%"},
				{Name: "worker", LastStatus: "RUNNING", Image: "repo/worker@sha256:abcdef"},
				{Name: "stopped", LastStatus: "STOPPED", Image: "repo/stopped:1"},
			},
		}},
	}

	got, err := DiscoverTaskContainers(context.Background(), client, ClusterSelection{ARN: "cluster"}, "task-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []ContainerSelection{
		{Name: "api", Image: "repo/api:latest", Status: "RUNNING", CPUResource: "10%", MemoryResource: "25%", Label: "api (unknown)"},
		{Name: "worker", Image: "repo/worker@sha256:abcdef", Status: "RUNNING", CPUResource: "", MemoryResource: "", Label: "worker (unknown)"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestDiscoverRunnableTasksFiltersByContainerAndRunning(t *testing.T) {
	startedAt := time.Now().Add(-5 * time.Minute)
	client := &pagingECS{
		taskPages: [][]string{{"task-1", "task-2"}},
		tasks: []TaskDetail{
			{ARN: "arn:task/1", LastStatus: "RUNNING", StartedAt: &startedAt, Containers: []ContainerDetail{{Name: "api", LastStatus: "RUNNING"}}},
			{ARN: "arn:task/2", LastStatus: "STOPPED", Containers: []ContainerDetail{{Name: "api", LastStatus: "RUNNING"}}},
		},
	}

	got, err := DiscoverRunnableTasks(context.Background(), client, ClusterSelection{ARN: "cluster"}, ServiceSelection{ARN: "service"}, ContainerSelection{Name: "api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ARN != "arn:task/1" {
		t.Fatalf("unexpected tasks: %#v", got)
	}
	if got[0].Label != "1 (5 minutes ago)" {
		t.Fatalf("expected task label format, got %q", got[0].Label)
	}
}

func TestFormatRelativeStartTime(t *testing.T) {
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	unknown := formatRelativeStartTime(nil, now)
	if unknown != "unknown" {
		t.Fatalf("got %q want %q", unknown, "unknown")
	}

	fiveMinutes := now.Add(-5 * time.Minute)
	if got := formatRelativeStartTime(&fiveMinutes, now); got != "5 minutes ago" {
		t.Fatalf("got %q want %q", got, "5 minutes ago")
	}
}

func TestServiceExecEnabled(t *testing.T) {
	client := &pagingECS{services: []ServiceDetail{{ARN: "svc", EnableExecuteCommand: true}}}
	enabled, err := ServiceExecEnabled(context.Background(), client, "cluster", "svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Fatalf("expected exec enabled")
	}
}

func TestParseProfilesDeduplicatesAndTrims(t *testing.T) {
	output := " dev \nprod\n\ndev\n"
	profiles := ParseProfiles(output)
	want := []string{"dev", "prod"}
	if !reflect.DeepEqual(profiles, want) {
		t.Fatalf("got %v want %v", profiles, want)
	}
}

func TestResourceName(t *testing.T) {
	got := ResourceName("arn:aws:ecs:us-east-1:123456789012:service/dev/api")
	if got != "api" {
		t.Fatalf("got %q want %q", got, "api")
	}
}
