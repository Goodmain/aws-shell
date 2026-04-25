package app

import (
	"reflect"
	"testing"
	"time"
)

func TestSelectionLabelFormattingParity(t *testing.T) {
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	started := now.Add(-5 * time.Minute)

	if got := formatClusterLabel("arn:aws:ecs:us-east-1:123456789012:cluster/dev"); got != "dev" {
		t.Fatalf("cluster label mismatch: got %q want %q", got, "dev")
	}

	if got := formatServiceLabel("api", ServiceDetail{PendingCount: 1, RunningCount: 3}); got != "api (1/3)" {
		t.Fatalf("service label mismatch: got %q want %q", got, "api (1/3)")
	}

	if got := formatContainerLabel("worker", ""); got != "worker (unknown)" {
		t.Fatalf("container label mismatch: got %q want %q", got, "worker (unknown)")
	}

	if got := formatTaskLabel("task-123", &started, now); got != "task-123 (5 minutes ago)" {
		t.Fatalf("task label mismatch: got %q want %q", got, "task-123 (5 minutes ago)")
	}
}

func TestSelectionSortingParity(t *testing.T) {
	options := []Option{{Label: "prod", Value: "prod"}, {Label: "Dev", Value: "Dev"}, {Label: "alpha", Value: "alpha"}}
	sortOptionsByLabel(options)
	wantOptions := []Option{{Label: "alpha", Value: "alpha"}, {Label: "Dev", Value: "Dev"}, {Label: "prod", Value: "prod"}}
	if !reflect.DeepEqual(options, wantOptions) {
		t.Fatalf("options sort mismatch: got %#v want %#v", options, wantOptions)
	}

	containers := []ContainerSelection{{Name: "worker", Label: "worker (unknown)"}, {Name: "api", Label: "Api (unknown)"}, {Name: "alpha", Label: "alpha (unknown)"}}
	sortContainerSelectionsByLabel(containers)
	wantContainers := []ContainerSelection{{Name: "alpha", Label: "alpha (unknown)"}, {Name: "api", Label: "Api (unknown)"}, {Name: "worker", Label: "worker (unknown)"}}
	if !reflect.DeepEqual(containers, wantContainers) {
		t.Fatalf("container sort mismatch: got %#v want %#v", containers, wantContainers)
	}

	tasks := []TaskSelection{{ID: "task-9", ARN: "9"}, {ID: "Task-1", ARN: "1"}, {ID: "task-2", ARN: "2"}}
	sortTaskSelectionsByID(tasks)
	wantTasks := []TaskSelection{{ID: "Task-1", ARN: "1"}, {ID: "task-2", ARN: "2"}, {ID: "task-9", ARN: "9"}}
	if !reflect.DeepEqual(tasks, wantTasks) {
		t.Fatalf("task sort mismatch: got %#v want %#v", tasks, wantTasks)
	}
}

func TestFormatRelativeServiceAge(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	fiveMinutes := now.Add(-5 * time.Minute)
	threeHours := now.Add(-3 * time.Hour)
	twoDays := now.Add(-48 * time.Hour)

	if got := formatRelativeServiceAge(nil, now); got != "-" {
		t.Fatalf("got %q want %q", got, "-")
	}
	if got := formatRelativeServiceAge(&fiveMinutes, now); got != "5m ago" {
		t.Fatalf("got %q want %q", got, "5m ago")
	}
	if got := formatRelativeServiceAge(&threeHours, now); got != "3h ago" {
		t.Fatalf("got %q want %q", got, "3h ago")
	}
	if got := formatRelativeServiceAge(&twoDays, now); got != "2d ago" {
		t.Fatalf("got %q want %q", got, "2d ago")
	}
}

func TestExtractContainerImageName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: "-"},
		{name: "full ecr with tag", input: "470708207499.dkr.ecr.us-east-1.amazonaws.com/myapp-frontend:e643abdc4ebb74c23910d265d74ce38d1e6474b1", want: "myapp-frontend"},
		{name: "with digest", input: "repo/path/api@sha256:abcdef", want: "api"},
		{name: "simple tag", input: "nginx:1.27", want: "nginx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractContainerImageName(tt.input); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}
