package app

import (
	"context"
	"errors"
	"time"
)

const (
	ModeBootstrap = "bootstrap"
	ModeAWS       = "aws"
	AuthAWSVault  = "aws-vault"
	AuthAWSSSO    = "aws-sso"
)

var ErrSelectionCanceled = errors.New("selection canceled")

type Runner interface {
	LookPath(file string) (string, error)
	Output(ctx context.Context, name string, args ...string) (string, error)
	Run(ctx context.Context, name string, args ...string) (int, error)
	RunWithEnv(ctx context.Context, env map[string]string, name string, args ...string) (int, error)
}

type ExecCommand struct {
	Name string
	Args []string
	Env  map[string]string
}

type AuthProvider interface {
	Name() string
	Available() bool
	ListProfiles(ctx context.Context) (string, error)
	EnsureLoggedIn(ctx context.Context, profile string) error
	WrapExec(executable string, profile string) ExecCommand
}

type ClusterSelection struct {
	ARN   string
	Label string
}

type ServiceSelection struct {
	ARN   string
	Label string
}

type ContainerSelection struct {
	Name           string
	Image          string
	Status         string
	CPUResource    string
	MemoryResource string
	Label          string
}

type TaskSelection struct {
	ARN       string
	ID        string
	Status    string
	CPU       string
	Memory    string
	CreatedAt *time.Time
	Label     string
}

type Option struct {
	Label string
	Value string
}

type Selector interface {
	Select(label string, options []Option, defaultIndex int) (Option, error)
}

type RetryBackAction string

const (
	RetryBackActionRetry RetryBackAction = "retry"
	RetryBackActionBack  RetryBackAction = "back"
)

type UIAdapter interface {
	Select(label string, options []Option, defaultIndex int) (Option, error)
	Confirm(label string, confirmLabel string, cancelLabel string, defaultConfirm bool) (bool, error)
	Message(message string)
	Error(message string)
	RetryOrBack(label string, allowBack bool) (RetryBackAction, error)
	Quit() error
}

type ProfilePreferenceStore interface {
	LoadLastUsedProfile(provider string) (string, error)
	SaveLastUsedProfile(provider string, profile string) error
	LoadLastUsedProvider() (string, error)
	SaveLastUsedProvider(provider string) error
}

type ECSClient interface {
	ListClusters(ctx context.Context, nextToken *string) ([]string, *string, error)
	ListServices(ctx context.Context, clusterArn string, nextToken *string) ([]string, *string, error)
	ListTasks(ctx context.Context, clusterArn string, serviceName string, nextToken *string) ([]string, *string, error)
	DescribeTasks(ctx context.Context, clusterArn string, taskArns []string) ([]TaskDetail, error)
	DescribeServices(ctx context.Context, clusterArn string, serviceArns []string) ([]ServiceDetail, error)
	UpdateServiceDesiredCount(ctx context.Context, clusterArn string, serviceArn string, desiredCount int32) error
}

type ECSFactory interface {
	New(ctx context.Context) (ECSClient, error)
}

type TaskDetail struct {
	ARN        string
	LastStatus string
	CreatedAt  *time.Time
	StartedAt  *time.Time
	CPU        string
	Memory     string
	StopReason string
	Containers []ContainerDetail
}

type ContainerDetail struct {
	Name       string
	ID         string
	Image      string
	LastStatus string
	CPU        string
	Memory     string
}

type ServiceDetail struct {
	ARN                  string
	Status               string
	EnableExecuteCommand bool
	CreatedAt            *time.Time
	PendingCount         int32
	RunningCount         int32
	DesiredCount         int32
}
