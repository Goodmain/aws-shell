package app

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildOptionsAlphabeticalOrdering(t *testing.T) {
	tests := []struct {
		name string
		got  []Option
		want []Option
	}{
		{
			name: "profiles mixed case",
			got:  BuildProfileOptions([]string{"prod", "Dev", "alpha"}),
			want: []Option{{Label: "alpha", Value: "alpha"}, {Label: "Dev", Value: "Dev"}, {Label: "prod", Value: "prod"}},
		},
		{
			name: "clusters already sorted stays sorted",
			got: BuildClusterOptions([]string{
				"arn:aws:ecs:us-east-1:123456789012:cluster/alpha",
				"arn:aws:ecs:us-east-1:123456789012:cluster/bravo",
			}),
			want: []Option{
				{Label: "alpha", Value: "arn:aws:ecs:us-east-1:123456789012:cluster/alpha"},
				{Label: "bravo", Value: "arn:aws:ecs:us-east-1:123456789012:cluster/bravo"},
			},
		},
		{
			name: "tasks already sorted stays sorted",
			got: BuildTaskOptions([]TaskSelection{
				{ARN: "arn:task/1", ID: "1", Status: "RUNNING", CPU: "256", Memory: "512"},
				{ARN: "arn:task/2", ID: "2", Status: "RUNNING", CPU: "256", Memory: "512"},
			}),
			want: []Option{{Label: "Task | Status  | CPU | Memory | Created", Value: WizardTableHeaderValue}, {Label: "1    | RUNNING | 256 |    512 |       -", Value: "arn:task/1"}, {Label: "2    | RUNNING | 256 |    512 |       -", Value: "arn:task/2"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Fatalf("got %#v want %#v", tt.got, tt.want)
			}
		})
	}
}

func TestBuildServiceOptionsFormatsTableCountsAndSorts(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	apiCreated := now.Add(-5 * time.Minute)
	ordersCreated := now.Add(-3 * time.Hour)

	arns := []string{
		"arn:aws:ecs:us-east-1:123456789012:service/dev/orders",
		"arn:aws:ecs:us-east-1:123456789012:service/dev/api",
	}
	details := map[string]ServiceDetail{
		arns[0]: {ARN: arns[0], DesiredCount: 2, PendingCount: 0, RunningCount: 0, CreatedAt: &ordersCreated},
		arns[1]: {ARN: arns[1], DesiredCount: 4, PendingCount: 1, RunningCount: 3, CreatedAt: &apiCreated},
	}

	got := buildServiceOptionsAt(arns, details, now)
	if len(got) != 3 {
		t.Fatalf("got %d options want %d", len(got), 3)
	}
	if got[0].Value != WizardTableHeaderValue {
		t.Fatalf("expected header row first, got %#v", got[0])
	}
	if got[1].Value != arns[1] || got[2].Value != arns[0] {
		t.Fatalf("unexpected option order/value: %#v", got)
	}
	if !strings.Contains(got[0].Label, "Service") || !strings.Contains(got[0].Label, "Desired") || !strings.Contains(got[0].Label, "Pending") || !strings.Contains(got[0].Label, "Running") || !strings.Contains(got[0].Label, "Created") {
		t.Fatalf("unexpected header label: %q", got[0].Label)
	}
	if !strings.Contains(got[1].Label, "api") || !strings.Contains(got[1].Label, "4") || !strings.Contains(got[1].Label, "1") || !strings.Contains(got[1].Label, "3") || !strings.Contains(got[1].Label, "5m ago") {
		t.Fatalf("unexpected first service row: %q", got[1].Label)
	}
	if !strings.Contains(got[2].Label, "orders") || !strings.Contains(got[2].Label, "2") || !strings.Contains(got[2].Label, "0") || !strings.Contains(got[2].Label, "3h ago") {
		t.Fatalf("unexpected second service row: %q", got[2].Label)
	}
}

func TestBuildContainerOptionsFormatsTableWithoutResourceColumns(t *testing.T) {
	got := BuildContainerOptions([]ContainerSelection{
		{Name: "worker", Image: "470708207499.dkr.ecr.us-east-1.amazonaws.com/platform/worker:abc", Status: "RUNNING"},
		{Name: "api", Image: "470708207499.dkr.ecr.us-east-1.amazonaws.com/myapp-frontend:e643abdc4ebb74c23910d265d74ce38d1e6474b1", Status: ""},
	})

	if len(got) != 3 {
		t.Fatalf("got %d options want %d", len(got), 3)
	}
	if got[0].Value != WizardTableHeaderValue {
		t.Fatalf("expected header row first, got %#v", got[0])
	}
	if !strings.Contains(got[0].Label, "Container") || !strings.Contains(got[0].Label, "Image") || !strings.Contains(got[0].Label, "Status") {
		t.Fatalf("unexpected container header label: %q", got[0].Label)
	}
	if strings.Contains(got[0].Label, "CPU") || strings.Contains(got[0].Label, "Memory") {
		t.Fatalf("did not expect CPU/memory resource columns when unavailable: %q", got[0].Label)
	}

	first := got[1]
	second := got[2]
	if first.Value != "api" || second.Value != "worker" {
		t.Fatalf("unexpected container option ordering: %#v", got)
	}
	if !strings.Contains(first.Label, "myapp-frontend") || strings.Contains(first.Label, "470708207499.dkr.ecr.us-east-1.amazonaws.com") {
		t.Fatalf("expected extracted image name only, got %q", first.Label)
	}
	if !strings.Contains(first.Label, "-") {
		t.Fatalf("expected missing status fallback '-', got %q", first.Label)
	}
}

func TestBuildContainerOptionsFormatsTableWithResourceColumns(t *testing.T) {
	got := BuildContainerOptions([]ContainerSelection{
		{Name: "worker", Image: "repo/worker:1", Status: "RUNNING", CPUResource: "22%", MemoryResource: "38%"},
		{Name: "api", Image: "repo/api@sha256:abcdef", Status: "RUNNING", CPUResource: "10%", MemoryResource: "25%"},
	})

	if len(got) != 3 {
		t.Fatalf("got %d options want %d", len(got), 3)
	}
	if !strings.Contains(got[0].Label, "CPU") || !strings.Contains(got[0].Label, "Memory") {
		t.Fatalf("expected CPU/memory resource columns in header, got %q", got[0].Label)
	}
	if !strings.Contains(got[1].Label, "10%") || !strings.Contains(got[1].Label, "25%") {
		t.Fatalf("expected resource values in first row, got %q", got[1].Label)
	}
	if !strings.Contains(got[2].Label, "22%") || !strings.Contains(got[2].Label, "38%") {
		t.Fatalf("expected resource values in second row, got %q", got[2].Label)
	}
}

func TestDefaultOptionIndex(t *testing.T) {
	options := []Option{{Label: "alpha", Value: "alpha"}, {Label: "bravo", Value: "bravo"}}

	if got := DefaultOptionIndex(options, "bravo"); got != 1 {
		t.Fatalf("got %d want %d", got, 1)
	}
	if got := DefaultOptionIndex(options, "missing"); got != 0 {
		t.Fatalf("got %d want %d", got, 0)
	}
}

func TestBuildClusterOptionsWithStatsFormatsTableRows(t *testing.T) {
	arns := []string{
		"arn:aws:ecs:us-east-1:123456789012:cluster/dev",
		"arn:aws:ecs:us-east-1:123456789012:cluster/prod",
	}
	stats := map[string]ClusterStats{
		arns[0]: {ServiceCount: 2, PendingTasks: 1, RunningTasks: 15},
		arns[1]: {ServiceCount: 10, PendingTasks: 3, RunningTasks: 220},
	}

	got := BuildClusterOptionsWithStats(arns, stats)
	if len(got) != 3 {
		t.Fatalf("got %d options want %d", len(got), 3)
	}
	if got[0].Value != WizardTableHeaderValue {
		t.Fatalf("expected header row first, got %#v", got[0])
	}
	if got[1].Value != arns[0] || got[2].Value != arns[1] {
		t.Fatalf("unexpected option order/value: %#v", got)
	}
	if got[0].Label != "Cluster | Services | Pending | Running" {
		t.Fatalf("unexpected header label: %q", got[0].Label)
	}
	if got[1].Label != "dev     |        2 |       1 |      15" {
		t.Fatalf("unexpected first cluster label: %q", got[1].Label)
	}
	if got[2].Label != "prod    |       10 |       3 |     220" {
		t.Fatalf("unexpected second cluster label: %q", got[2].Label)
	}
}
