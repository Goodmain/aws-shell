package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	lookPathErr error
	output      string
	outputErr   error
	runExitCode int
	runErr      error
	runResults  []runnerResult
	runCalls    int
	runName     string
	runArgs     []string
}

type runnerResult struct {
	exitCode int
	err      error
}

func (f *fakeRunner) LookPath(_ string) (string, error) {
	if f.lookPathErr != nil {
		return "", f.lookPathErr
	}
	return "/usr/local/bin/aws-vault", nil
}

func (f *fakeRunner) Output(_ context.Context, _ string, _ ...string) (string, error) {
	if f.outputErr != nil {
		return "", f.outputErr
	}
	return f.output, nil
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (int, error) {
	f.runName = name
	f.runArgs = append([]string{}, args...)
	f.runCalls++
	if len(f.runResults) > 0 {
		idx := f.runCalls - 1
		if idx >= len(f.runResults) {
			idx = len(f.runResults) - 1
		}
		return f.runResults[idx].exitCode, f.runResults[idx].err
	}
	if f.runErr != nil {
		return f.runExitCode, f.runErr
	}
	return f.runExitCode, nil
}

type fakeSelector struct {
	selected Option
	err      error
}

func (f *fakeSelector) Select(_ string, options []Option, defaultIndex int) (Option, error) {
	if f.err != nil {
		return Option{}, f.err
	}
	if f.selected.Value != "" {
		return f.selected, nil
	}
	if len(options) == 0 {
		return Option{}, nil
	}
	if defaultIndex < 0 || defaultIndex >= len(options) {
		defaultIndex = 0
	}

	return options[defaultIndex], nil
}

type fakeECSFactory struct {
	client ECSClient
	err    error
}

func (f *fakeECSFactory) New(_ context.Context) (ECSClient, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.client, nil
}

type fakeECSClient struct {
	clustersPages                   [][]string
	servicesPages                   [][]string
	tasksPages                      [][]string
	taskDetails                     []TaskDetail
	serviceDetail                   []ServiceDetail
	serviceDetails                  [][]ServiceDetail
	clusterErr                      error
	serviceErr                      error
	taskErr                         error
	describeErr                     error
	describeSvcErr                  error
	bypassedFirstServiceDetailsCall bool
	updateSvcErr                    error
	clusterCalls                    int
	serviceCalls                    int
	taskCalls                       int
	describeCalls                   int
	updatedDesired                  []int32
}

type fakeProfilePreferenceStore struct {
	loadProfile string
	loadErr     error
	saveErr     error
	saved       string
}

func (f *fakeProfilePreferenceStore) LoadLastUsedProfile() (string, error) {
	if f.loadErr != nil {
		return "", f.loadErr
	}

	return f.loadProfile, nil
}

func (f *fakeProfilePreferenceStore) SaveLastUsedProfile(profile string) error {
	f.saved = profile
	return f.saveErr
}

type selectionCall struct {
	label        string
	options      []Option
	defaultIndex int
}

type recordingSelector struct {
	calls      []selectionCall
	selections map[string]Option
	errs       map[string]error
}

type scriptedUIAdapter struct {
	selectByLabel  map[string]Option
	confirmByLabel map[string]bool
	retryByLabel   map[string]RetryBackAction
	errsByLabel    map[string]error
	calls          []string
	suspendCalls   int
	quitCalls      int
}

func (r *recordingSelector) Select(label string, options []Option, defaultIndex int) (Option, error) {
	copiedOptions := append([]Option{}, options...)
	r.calls = append(r.calls, selectionCall{label: label, options: copiedOptions, defaultIndex: defaultIndex})

	if err, ok := r.errs[label]; ok {
		return Option{}, err
	}
	if selected, ok := r.selections[label]; ok {
		return selected, nil
	}

	return options[defaultIndex], nil
}

func (s *scriptedUIAdapter) Select(label string, options []Option, defaultIndex int) (Option, error) {
	s.calls = append(s.calls, "select:"+label)
	if err, ok := s.errsByLabel[label]; ok {
		return Option{}, err
	}
	if selected, ok := s.selectByLabel[label]; ok {
		return selected, nil
	}

	return options[defaultIndex], nil
}

func (s *scriptedUIAdapter) Confirm(label string, confirmLabel string, cancelLabel string, defaultConfirm bool) (bool, error) {
	s.calls = append(s.calls, "confirm:"+label)
	if err, ok := s.errsByLabel[label]; ok {
		return false, err
	}
	if confirmed, ok := s.confirmByLabel[label]; ok {
		return confirmed, nil
	}

	return defaultConfirm, nil
}

func (s *scriptedUIAdapter) Message(message string) {
	s.calls = append(s.calls, "message:"+message)
}

func (s *scriptedUIAdapter) Error(message string) {
	s.calls = append(s.calls, "error:"+message)
}

func (s *scriptedUIAdapter) RetryOrBack(label string, _ bool) (RetryBackAction, error) {
	s.calls = append(s.calls, "retry-or-back:"+label)
	if err, ok := s.errsByLabel[label]; ok {
		return "", err
	}
	if action, ok := s.retryByLabel[label]; ok {
		return action, nil
	}

	return RetryBackActionRetry, nil
}

func (s *scriptedUIAdapter) Quit() error {
	s.quitCalls++
	return ErrSelectionCanceled
}

func (s *scriptedUIAdapter) Suspend(run func()) {
	s.suspendCalls++
	if run != nil {
		run()
	}
}

func (f *fakeECSClient) ListClusters(_ context.Context, _ *string) ([]string, *string, error) {
	if f.clusterErr != nil {
		return nil, nil, f.clusterErr
	}
	if f.clusterCalls >= len(f.clustersPages) {
		return nil, nil, nil
	}
	page := f.clustersPages[f.clusterCalls]
	f.clusterCalls++
	if f.clusterCalls < len(f.clustersPages) {
		token := "next"
		return page, &token, nil
	}
	return page, nil, nil
}

func (f *fakeECSClient) ListServices(_ context.Context, _ string, nextToken *string) ([]string, *string, error) {
	if f.serviceErr != nil {
		return nil, nil, f.serviceErr
	}
	if len(f.servicesPages) == 0 {
		return nil, nil, nil
	}

	pageIndex := 0
	if nextToken != nil && strings.TrimSpace(*nextToken) != "" {
		token := strings.TrimSpace(*nextToken)
		if strings.HasPrefix(token, "svc:") {
			parsedIndex, parseErr := strconv.Atoi(strings.TrimPrefix(token, "svc:"))
			if parseErr == nil && parsedIndex >= 0 {
				pageIndex = parsedIndex
			}
		}
	}

	if pageIndex >= len(f.servicesPages) {
		return nil, nil, nil
	}

	page := f.servicesPages[pageIndex]
	f.serviceCalls++
	if pageIndex+1 < len(f.servicesPages) {
		token := fmt.Sprintf("svc:%d", pageIndex+1)
		return page, &token, nil
	}
	return page, nil, nil
}

func (f *fakeECSClient) ListTasks(_ context.Context, _ string, _ string, _ *string) ([]string, *string, error) {
	if f.taskErr != nil {
		return nil, nil, f.taskErr
	}
	if len(f.tasksPages) == 0 {
		return nil, nil, nil
	}
	page := f.tasksPages[len(f.tasksPages)-1]
	if f.taskCalls < len(f.tasksPages) {
		page = f.tasksPages[f.taskCalls]
	}
	f.taskCalls++
	if f.taskCalls < len(f.tasksPages) {
		token := "next"
		return page, &token, nil
	}
	return page, nil, nil
}

func (f *fakeECSClient) DescribeTasks(_ context.Context, _ string, _ []string) ([]TaskDetail, error) {
	if f.describeErr != nil {
		return nil, f.describeErr
	}
	return f.taskDetails, nil
}

func (f *fakeECSClient) DescribeServices(_ context.Context, _ string, _ []string) ([]ServiceDetail, error) {
	if f.describeSvcErr != nil {
		return nil, f.describeSvcErr
	}
	if len(f.serviceDetails) > 0 {
		if !f.bypassedFirstServiceDetailsCall {
			f.bypassedFirstServiceDetailsCall = true
			return f.serviceDetails[0], nil
		}

		idx := f.describeCalls
		if idx >= len(f.serviceDetails) {
			idx = len(f.serviceDetails) - 1
		}
		f.describeCalls++
		return f.serviceDetails[idx], nil
	}
	return f.serviceDetail, nil
}

func (f *fakeECSClient) UpdateServiceDesiredCount(_ context.Context, _ string, _ string, desiredCount int32) error {
	f.updatedDesired = append(f.updatedDesired, desiredCount)
	return f.updateSvcErr
}

func newTestCLI(deps Dependencies) *CLI {
	if deps.UI == nil {
		deps.UI = NewSelectorUIAdapter(deps.Selector, deps.Stdout, deps.Stderr)
	}

	return NewCLI(deps)
}

func TestNewCLIPanicsWhenUIAdapterMissing(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic when UI adapter is missing")
		}
		if recovered != "ui adapter is required" {
			t.Fatalf("unexpected panic message: %v", recovered)
		}
	}()

	_ = NewCLI(Dependencies{})
}

func TestRunBootstrapFailsWhenAWSVaultMissing(t *testing.T) {
	stderr := &strings.Builder{}
	cli := newTestCLI(Dependencies{
		Runner:   &fakeRunner{lookPathErr: errors.New("missing")},
		Selector: &fakeSelector{},
		ECS:      &fakeECSFactory{},
		Stdout:   &strings.Builder{},
		Stderr:   stderr,
	})

	code := cli.Run(context.Background(), Config{Mode: ModeBootstrap, Executable: "tool"})
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if !strings.Contains(stderr.String(), "aws-vault is required") {
		t.Fatalf("expected aws-vault guidance, got %q", stderr.String())
	}
}

func TestRunBootstrapFailsWhenProfilesEmpty(t *testing.T) {
	stderr := &strings.Builder{}
	cli := newTestCLI(Dependencies{
		Runner:   &fakeRunner{output: "\n \n"},
		Selector: &fakeSelector{},
		ECS:      &fakeECSFactory{},
		Stdout:   &strings.Builder{},
		Stderr:   stderr,
	})

	code := cli.Run(context.Background(), Config{Mode: ModeBootstrap, Executable: "tool"})
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if !strings.Contains(stderr.String(), "no aws-vault profiles found") {
		t.Fatalf("expected missing profile error, got %q", stderr.String())
	}
}

func TestRunBootstrapComposesAWSVaultExec(t *testing.T) {
	runner := &fakeRunner{output: "dev\nprod\n"}
	selector := &fakeSelector{selected: Option{Label: "prod", Value: "prod"}}
	cli := newTestCLI(Dependencies{
		Runner:   runner,
		Selector: selector,
		ECS:      &fakeECSFactory{},
		Stdout:   &strings.Builder{},
		Stderr:   &strings.Builder{},
	})

	code := cli.Run(context.Background(), Config{Mode: ModeBootstrap, Executable: "/tmp/app"})
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d", code)
	}

	if runner.runName != "aws-vault" {
		t.Fatalf("expected aws-vault command, got %q", runner.runName)
	}

	joined := strings.Join(runner.runArgs, " ")
	if !strings.Contains(joined, "exec prod -- /tmp/app --mode aws --profile prod") {
		t.Fatalf("unexpected command args: %q", joined)
	}
}

func TestRunBootstrapUsesConsoleSelectorWhenUIConfigured(t *testing.T) {
	runner := &fakeRunner{output: "dev\n"}
	selector := &fakeSelector{selected: Option{Label: "dev", Value: "dev"}}
	ui := &scriptedUIAdapter{
		errsByLabel: map[string]error{
			"Select AWS profile": errors.New("ui selector should not be called in bootstrap"),
		},
	}
	stdout := &strings.Builder{}
	cli := newTestCLI(Dependencies{
		Runner:   runner,
		Selector: selector,
		UI:       ui,
		ECS:      &fakeECSFactory{},
		Stdout:   stdout,
		Stderr:   &strings.Builder{},
	})

	code := cli.Run(context.Background(), Config{Mode: ModeBootstrap, Executable: "/tmp/app"})
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d", code)
	}
	if ui.quitCalls != 0 {
		t.Fatalf("expected no ui quit call in bootstrap, got %d", ui.quitCalls)
	}
	if !strings.Contains(stdout.String(), "Selected profile \"dev\". Launching aws-vault...") {
		t.Fatalf("expected console handoff message, got %q", stdout.String())
	}
}

func TestRunBootstrapReturnsExitCodeFromAWSVaultExecFailure(t *testing.T) {
	runner := &fakeRunner{
		output:      "dev\n",
		runExitCode: 42,
		runErr:      errors.New("exec failed"),
	}
	cli := newTestCLI(Dependencies{
		Runner:   runner,
		Selector: &fakeSelector{},
		ECS:      &fakeECSFactory{},
		Stdout:   &strings.Builder{},
		Stderr:   &strings.Builder{},
	})

	code := cli.Run(context.Background(), Config{Mode: ModeBootstrap, Executable: "app"})
	if code != 42 {
		t.Fatalf("expected exit code 42, got %d", code)
	}
}

func TestRunBootstrapSelectionCanceled(t *testing.T) {
	stderr := &strings.Builder{}
	cli := newTestCLI(Dependencies{
		Runner:   &fakeRunner{output: "dev\n"},
		Selector: &fakeSelector{err: ErrSelectionCanceled},
		ECS:      &fakeECSFactory{},
		Stdout:   &strings.Builder{},
		Stderr:   stderr,
	})

	code := cli.Run(context.Background(), Config{Mode: ModeBootstrap, Executable: "app"})
	if code == 0 {
		t.Fatalf("expected non-zero code")
	}
	if !strings.Contains(stderr.String(), "selection canceled") {
		t.Fatalf("expected cancel message, got %q", stderr.String())
	}
}

func TestRunBootstrapFailsWhenSelectorMissing(t *testing.T) {
	stderr := &strings.Builder{}
	cli := newTestCLI(Dependencies{
		Runner: &fakeRunner{output: "dev\n"},
		ECS:    &fakeECSFactory{},
		Stdout: &strings.Builder{},
		Stderr: stderr,
	})

	code := cli.Run(context.Background(), Config{Mode: ModeBootstrap, Executable: "app"})
	if code == 0 {
		t.Fatalf("expected non-zero code when selector missing")
	}
	if !strings.Contains(stderr.String(), "profile selector is not configured") {
		t.Fatalf("expected missing selector error, got %q", stderr.String())
	}
}

func TestRunBootstrapSortsProfilesAndPreselectsRemembered(t *testing.T) {
	selector := &recordingSelector{}
	prefs := &fakeProfilePreferenceStore{loadProfile: "Dev"}
	cli := newTestCLI(Dependencies{
		Runner:      &fakeRunner{output: "prod\nDev\nalpha\n"},
		Selector:    selector,
		ECS:         &fakeECSFactory{},
		Preferences: prefs,
		Stdout:      &strings.Builder{},
		Stderr:      &strings.Builder{},
	})

	code := cli.Run(context.Background(), Config{Mode: ModeBootstrap, Executable: "app"})
	if code != 0 {
		t.Fatalf("expected success, got %d", code)
	}

	if len(selector.calls) != 1 {
		t.Fatalf("expected one selection call, got %d", len(selector.calls))
	}
	call := selector.calls[0]
	if call.label != "Select AWS profile" {
		t.Fatalf("unexpected label: %q", call.label)
	}
	wantOptions := []Option{{Label: "alpha", Value: "alpha"}, {Label: "Dev", Value: "Dev"}, {Label: "prod", Value: "prod"}}
	if !equalOptions(call.options, wantOptions) {
		t.Fatalf("got options %#v want %#v", call.options, wantOptions)
	}
	if call.defaultIndex != 1 {
		t.Fatalf("got default index %d want %d", call.defaultIndex, 1)
	}
	if prefs.saved != "Dev" {
		t.Fatalf("expected saved profile %q, got %q", "Dev", prefs.saved)
	}
}

func TestRunAWSNoClustersExitsSuccess(t *testing.T) {
	stdout := &strings.Builder{}
	cli := newTestCLI(Dependencies{
		Runner:   &fakeRunner{},
		Selector: &fakeSelector{},
		ECS: &fakeECSFactory{client: &fakeECSClient{
			clustersPages: [][]string{{}},
		}},
		Stdout: stdout,
		Stderr: &strings.Builder{},
	})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected success exit, got %d", code)
	}
	if !strings.Contains(stdout.String(), "No ECS clusters found") {
		t.Fatalf("expected no cluster message, got %q", stdout.String())
	}
}

func TestRunAWSQuitActionAtClusterExitsCleanly(t *testing.T) {
	runner := &fakeRunner{}
	ui := &scriptedUIAdapter{
		selectByLabel: map[string]Option{
			"Select ECS cluster": {Label: "Quit", Value: WizardActionQuit},
		},
	}
	client := &fakeECSClient{
		clustersPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:cluster/dev"}},
	}
	cli := newTestCLI(Dependencies{
		Runner: runner,
		UI:     ui,
		ECS:    &fakeECSFactory{client: client},
		Stdout: &strings.Builder{},
		Stderr: &strings.Builder{},
	})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected quit to exit with code 0, got %d", code)
	}
	if runner.runCalls != 0 {
		t.Fatalf("expected no exec call on quit, got %d", runner.runCalls)
	}
}

func TestRunAWSClusterPickerHeaderShowsClusterStep(t *testing.T) {
	ui := &scriptedUIAdapter{
		errsByLabel: map[string]error{
			"Select ECS cluster": ErrSelectionCanceled,
		},
	}
	client := &fakeECSClient{
		clustersPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:cluster/dev"}},
	}
	cli := newTestCLI(Dependencies{
		Runner: &fakeRunner{},
		UI:     ui,
		ECS:    &fakeECSFactory{client: client},
		Stdout: &strings.Builder{},
		Stderr: &strings.Builder{},
	})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code == 0 {
		t.Fatalf("expected non-zero on canceled selection")
	}

	foundClusterStep := false
	for _, call := range ui.calls {
		if strings.Contains(call, "message:Step: cluster") {
			foundClusterStep = true
			break
		}
	}
	if !foundClusterStep {
		t.Fatalf("expected header step to show cluster, calls=%#v", ui.calls)
	}
}

func TestRunAWSEndToEndConnectsWithSelectedTuple(t *testing.T) {
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	runner := &fakeRunner{}
	selector := &fakeSelectorSequence{selections: []Option{
		{Label: "dev", Value: "arn:aws:ecs:us-east-1:123456789012:cluster/dev"},
		{Label: "api", Value: "arn:aws:ecs:us-east-1:123456789012:service/dev/api"},
		{Label: "task-123", Value: "arn:aws:ecs:us-east-1:123456789012:task/dev/task-123"},
		{Label: "api", Value: "api"},
		{Label: "Connect", Value: "yes"},
	}}
	client := &fakeECSClient{
		clustersPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:cluster/dev"}},
		servicesPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:service/dev/api"}},
		tasksPages:    [][]string{{"arn:aws:ecs:us-east-1:123456789012:task/dev/task-123"}},
		taskDetails: []TaskDetail{{
			ARN:        "arn:aws:ecs:us-east-1:123456789012:task/dev/task-123",
			LastStatus: "RUNNING",
			Containers: []ContainerDetail{{Name: "api", LastStatus: "RUNNING"}},
		}},
		serviceDetail: []ServiceDetail{{
			ARN:                  "arn:aws:ecs:us-east-1:123456789012:service/dev/api",
			EnableExecuteCommand: true,
			RunningCount:         1,
		}},
	}
	cli := newTestCLI(Dependencies{
		Runner:   runner,
		Selector: selector,
		ECS:      &fakeECSFactory{client: client},
		Stdout:   stdout,
		Stderr:   stderr,
	})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected success, got %d stderr=%q", code, stderr.String())
	}
	if runner.runName != "aws" {
		t.Fatalf("expected aws command, got %q", runner.runName)
	}
	joined := strings.Join(runner.runArgs, " ")
	if !strings.Contains(joined, "ecs execute-command --cluster arn:aws:ecs:us-east-1:123456789012:cluster/dev --task arn:aws:ecs:us-east-1:123456789012:task/dev/task-123 --container api --interactive --command /bin/sh") {
		t.Fatalf("unexpected command args: %q", joined)
	}
}

func TestRunAWSSuspendsUIWhileRunningExecCommand(t *testing.T) {
	runner := &fakeRunner{}
	ui := &scriptedUIAdapter{
		selectByLabel: map[string]Option{
			"Select ECS cluster":                    {Label: "dev", Value: "arn:aws:ecs:us-east-1:123456789012:cluster/dev"},
			"Select ECS service":                    {Label: "api", Value: "arn:aws:ecs:us-east-1:123456789012:service/dev/api"},
			"Select task (first option is default)": {Label: "task-123", Value: "arn:aws:ecs:us-east-1:123456789012:task/dev/task-123"},
			"Select container":                      {Label: "api", Value: "api"},
		},
		confirmByLabel: map[string]bool{
			"Connect to dev/api/api/task-123?": true,
		},
	}
	client := &fakeECSClient{
		clustersPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:cluster/dev"}},
		servicesPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:service/dev/api"}},
		tasksPages:    [][]string{{"arn:aws:ecs:us-east-1:123456789012:task/dev/task-123"}},
		taskDetails: []TaskDetail{{
			ARN:        "arn:aws:ecs:us-east-1:123456789012:task/dev/task-123",
			LastStatus: "RUNNING",
			Containers: []ContainerDetail{{Name: "api", LastStatus: "RUNNING"}},
		}},
		serviceDetail: []ServiceDetail{{
			ARN:                  "arn:aws:ecs:us-east-1:123456789012:service/dev/api",
			EnableExecuteCommand: true,
			RunningCount:         1,
		}},
	}
	cli := newTestCLI(Dependencies{
		Runner: runner,
		UI:     ui,
		ECS:    &fakeECSFactory{client: client},
		Stdout: &strings.Builder{},
		Stderr: &strings.Builder{},
	})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected success, got %d", code)
	}
	if ui.suspendCalls != 1 {
		t.Fatalf("expected suspend to be called once, got %d", ui.suspendCalls)
	}
}

func TestRunAWSNoServicesExitsSuccess(t *testing.T) {
	stdout := &strings.Builder{}
	cli := newTestCLI(Dependencies{
		Runner:   &fakeRunner{},
		Selector: &fakeSelector{},
		ECS: &fakeECSFactory{client: &fakeECSClient{
			clustersPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:cluster/dev"}},
			servicesPages: [][]string{{}},
		}},
		Stdout: stdout,
		Stderr: &strings.Builder{},
	})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected success, got %d", code)
	}
	if !strings.Contains(stdout.String(), "No ECS services found") {
		t.Fatalf("expected no services message, got %q", stdout.String())
	}
}

func TestRunAWSNoContainersExitsSuccess(t *testing.T) {
	stdout := &strings.Builder{}
	selector := &fakeSelectorSequence{selections: []Option{{Label: "dev", Value: "cluster"}, {Label: "api", Value: "service"}, {Label: "task-1", Value: "task-1"}}}
	cli := newTestCLI(Dependencies{
		Runner:   &fakeRunner{},
		Selector: selector,
		ECS: &fakeECSFactory{client: &fakeECSClient{
			clustersPages: [][]string{{"cluster"}},
			servicesPages: [][]string{{"service"}},
			tasksPages:    [][]string{{"task-1"}},
			taskDetails:   []TaskDetail{{ARN: "task-1", LastStatus: "RUNNING", Containers: []ContainerDetail{}}},
		}},
		Stdout: stdout,
		Stderr: &strings.Builder{},
	})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected success, got %d", code)
	}
	if !strings.Contains(stdout.String(), "No runnable containers found for task") {
		t.Fatalf("expected no containers message, got %q", stdout.String())
	}
}

func TestRunAWSNoTasksExitsSuccess(t *testing.T) {
	stdout := &strings.Builder{}
	selector := &fakeSelectorSequence{selections: []Option{{Label: "dev", Value: "cluster"}, {Label: "api", Value: "service"}}}
	cli := newTestCLI(Dependencies{
		Runner:   &fakeRunner{},
		Selector: selector,
		ECS: &fakeECSFactory{client: &fakeECSClient{
			clustersPages: [][]string{{"cluster"}},
			servicesPages: [][]string{{"service"}},
			tasksPages:    [][]string{{}},
		}},
		Stdout: stdout,
		Stderr: &strings.Builder{},
	})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected success, got %d", code)
	}
	if !strings.Contains(stdout.String(), "No running tasks available") {
		t.Fatalf("expected no tasks message, got %q", stdout.String())
	}
}

func TestRunAWSServiceErrorFails(t *testing.T) {
	stderr := &strings.Builder{}
	client := &fakeECSClient{
		clustersPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:cluster/dev"}},
		serviceErr:    errors.New("boom"),
	}
	cli := newTestCLI(Dependencies{
		Runner:   &fakeRunner{},
		Selector: &fakeSelector{},
		ECS:      &fakeECSFactory{client: client},
		Stdout:   &strings.Builder{},
		Stderr:   stderr,
	})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code == 0 {
		t.Fatalf("expected non-zero on service error")
	}
	if !strings.Contains(stderr.String(), "failed to summarize ECS cluster") {
		t.Fatalf("expected service listing error, got %q", stderr.String())
	}
}

func TestRunAWSServicePickerShowsSortedTableLabelsWithCounts(t *testing.T) {
	selector := &recordingSelector{errs: map[string]error{"Select ECS service": ErrSelectionCanceled}}
	client := &fakeECSClient{
		clustersPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:cluster/dev"}},
		servicesPages: [][]string{{
			"arn:aws:ecs:us-east-1:123456789012:service/dev/orders",
			"arn:aws:ecs:us-east-1:123456789012:service/dev/api",
		}},
		serviceDetail: []ServiceDetail{
			{ARN: "arn:aws:ecs:us-east-1:123456789012:service/dev/orders", PendingCount: 0, RunningCount: 0},
			{ARN: "arn:aws:ecs:us-east-1:123456789012:service/dev/api", PendingCount: 1, RunningCount: 3},
		},
	}
	cli := newTestCLI(Dependencies{
		Runner:   &fakeRunner{},
		Selector: selector,
		ECS:      &fakeECSFactory{client: client},
		Stdout:   &strings.Builder{},
		Stderr:   &strings.Builder{},
	})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code == 0 {
		t.Fatalf("expected cancel path to return non-zero")
	}

	serviceCall, ok := findCall(selector.calls, "Select ECS service")
	if !ok {
		t.Fatalf("expected service picker call")
	}
	if len(serviceCall.options) != 3 {
		t.Fatalf("expected header plus 2 service options, got %#v", serviceCall.options)
	}
	if serviceCall.options[0].Value != WizardTableHeaderValue {
		t.Fatalf("expected first option to be service table header, got %#v", serviceCall.options[0])
	}
	if !strings.Contains(serviceCall.options[0].Label, "Service") || !strings.Contains(serviceCall.options[0].Label, "Desired") || !strings.Contains(serviceCall.options[0].Label, "Pending") || !strings.Contains(serviceCall.options[0].Label, "Running") || !strings.Contains(serviceCall.options[0].Label, "Created") {
		t.Fatalf("unexpected service header label: %q", serviceCall.options[0].Label)
	}

	first := serviceCall.options[1]
	second := serviceCall.options[2]
	if first.Value != "arn:aws:ecs:us-east-1:123456789012:service/dev/api" || second.Value != "arn:aws:ecs:us-east-1:123456789012:service/dev/orders" {
		t.Fatalf("unexpected service order: %#v", serviceCall.options)
	}
	if !strings.Contains(first.Label, "api") || !strings.Contains(first.Label, "0") || !strings.Contains(first.Label, "1") || !strings.Contains(first.Label, "3") {
		t.Fatalf("unexpected first service row: %q", first.Label)
	}
	if !strings.Contains(second.Label, "orders") || !strings.Contains(second.Label, "0") {
		t.Fatalf("unexpected second service row: %q", second.Label)
	}
	if !strings.Contains(first.Label, "-") || !strings.Contains(second.Label, "-") {
		t.Fatalf("expected created-age fallback '-', got %#v", serviceCall.options)
	}
}

func TestRunAWSContainerPickerShowsTableWithImageAndStatus(t *testing.T) {
	selector := &recordingSelector{
		selections: map[string]Option{
			"Select ECS cluster":                    {Label: "dev", Value: "cluster"},
			"Select ECS service":                    {Label: "api", Value: "service"},
			"Select task (first option is default)": {Label: "task-1", Value: "task-1"},
		},
		errs: map[string]error{"Select container": ErrSelectionCanceled},
	}
	client := &fakeECSClient{
		clustersPages: [][]string{{"cluster"}},
		servicesPages: [][]string{{"service"}},
		tasksPages:    [][]string{{"task-1"}},
		taskDetails: []TaskDetail{{
			ARN:        "task-1",
			LastStatus: "RUNNING",
			Containers: []ContainerDetail{{Name: "api", LastStatus: "RUNNING", Image: "470708207499.dkr.ecr.us-east-1.amazonaws.com/myapp-frontend:e643abdc4ebb74c23910d265d74ce38d1e6474b1"}},
		}},
	}
	cli := newTestCLI(Dependencies{Runner: &fakeRunner{}, Selector: selector, ECS: &fakeECSFactory{client: client}, Stdout: &strings.Builder{}, Stderr: &strings.Builder{}})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code == 0 {
		t.Fatalf("expected cancel path to return non-zero")
	}

	containerCall, ok := findCall(selector.calls, "Select container")
	if !ok {
		t.Fatalf("expected container picker call")
	}
	if len(containerCall.options) != 2 {
		t.Fatalf("expected header plus one container option, got %#v", containerCall.options)
	}
	if containerCall.options[0].Value != WizardTableHeaderValue {
		t.Fatalf("expected first option to be container table header, got %#v", containerCall.options[0])
	}
	if !strings.Contains(containerCall.options[0].Label, "Container") || !strings.Contains(containerCall.options[0].Label, "Image") || !strings.Contains(containerCall.options[0].Label, "Status") {
		t.Fatalf("unexpected container header label: %q", containerCall.options[0].Label)
	}
	if strings.Contains(containerCall.options[0].Label, "CPU") || strings.Contains(containerCall.options[0].Label, "Memory") {
		t.Fatalf("did not expect utilization columns in header: %q", containerCall.options[0].Label)
	}
	if !strings.Contains(containerCall.options[1].Label, "myapp-frontend") || strings.Contains(containerCall.options[1].Label, "dkr.ecr") {
		t.Fatalf("expected image name only in row, got %q", containerCall.options[1].Label)
	}
	if !strings.Contains(containerCall.options[1].Label, "RUNNING") {
		t.Fatalf("expected status in container row, got %q", containerCall.options[1].Label)
	}
}

func TestRunAWSContainerPickerIncludesResourceColumnsWhenAvailable(t *testing.T) {
	selector := &recordingSelector{
		selections: map[string]Option{
			"Select ECS cluster":                    {Label: "dev", Value: "cluster"},
			"Select ECS service":                    {Label: "api", Value: "service"},
			"Select task (first option is default)": {Label: "task-1", Value: "task-1"},
		},
		errs: map[string]error{"Select container": ErrSelectionCanceled},
	}
	client := &fakeECSClient{
		clustersPages: [][]string{{"cluster"}},
		servicesPages: [][]string{{"service"}},
		tasksPages:    [][]string{{"task-1"}},
		taskDetails: []TaskDetail{{
			ARN:        "task-1",
			LastStatus: "RUNNING",
			Containers: []ContainerDetail{
				{Name: "api", LastStatus: "RUNNING", Image: "repo/api:1", CPU: "10%", Memory: "25%"},
				{Name: "worker", LastStatus: "RUNNING", Image: "repo/worker:1", CPU: "12%", Memory: "35%"},
			},
		}},
	}
	cli := newTestCLI(Dependencies{Runner: &fakeRunner{}, Selector: selector, ECS: &fakeECSFactory{client: client}, Stdout: &strings.Builder{}, Stderr: &strings.Builder{}})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code == 0 {
		t.Fatalf("expected cancel path to return non-zero")
	}

	containerCall, ok := findCall(selector.calls, "Select container")
	if !ok {
		t.Fatalf("expected container picker call")
	}
	if !strings.Contains(containerCall.options[0].Label, "CPU") || !strings.Contains(containerCall.options[0].Label, "Memory") {
		t.Fatalf("expected CPU/memory resource columns in header, got %q", containerCall.options[0].Label)
	}
	if !strings.Contains(containerCall.options[1].Label, "10%") || !strings.Contains(containerCall.options[1].Label, "25%") {
		t.Fatalf("expected CPU/memory resource values in first container row, got %q", containerCall.options[1].Label)
	}
}

func TestRunAWSCancellationBeforeConnect(t *testing.T) {
	stdout := &strings.Builder{}
	runner := &fakeRunner{}
	selector := &fakeSelectorSequence{selections: []Option{
		{Label: "dev", Value: "cluster"},
		{Label: "api", Value: "service"},
		{Label: "task-1", Value: "task-1"},
		{Label: "api", Value: "api"},
		{Label: "Cancel", Value: "no"},
		{Label: "api", Value: "api"},
		{Label: "Connect", Value: "yes"},
	}}
	client := &fakeECSClient{
		clustersPages: [][]string{{"cluster"}},
		servicesPages: [][]string{{"service"}},
		tasksPages:    [][]string{{"task-1"}},
		taskDetails:   []TaskDetail{{ARN: "task-1", LastStatus: "RUNNING", Containers: []ContainerDetail{{Name: "api", LastStatus: "RUNNING"}}}},
		serviceDetail: []ServiceDetail{{ARN: "service", EnableExecuteCommand: true, RunningCount: 1}},
	}
	cli := newTestCLI(Dependencies{Runner: runner, Selector: selector, ECS: &fakeECSFactory{client: client}, Stdout: stdout, Stderr: &strings.Builder{}})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected success cancel path, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Connection canceled") {
		t.Fatalf("expected cancel output, got %q", stdout.String())
	}
	if runner.runName != "aws" {
		t.Fatalf("expected flow to remain active and connect after cancel, got run %q", runner.runName)
	}
}

func TestRunAWSConnectFailureRetriesInsteadOfExiting(t *testing.T) {
	stderr := &strings.Builder{}
	runner := &fakeRunner{runResults: []runnerResult{{exitCode: 37, err: errors.New("exec failed")}, {exitCode: 0, err: nil}}}
	selector := &recordingSelector{selections: map[string]Option{
		"Select ECS cluster":                    {Label: "dev", Value: "cluster"},
		"Select ECS service":                    {Label: "api", Value: "service"},
		"Select container":                      {Label: "api", Value: "api"},
		"Select task (first option is default)": {Label: "task-1", Value: "task-1"},
		"Connect to cluster/service/api/task-1?": {Label: "Connect", Value: "yes"},
	}}
	client := &fakeECSClient{
		clustersPages: [][]string{{"cluster"}},
		servicesPages: [][]string{{"service"}},
		tasksPages:    [][]string{{"task-1"}},
		taskDetails: []TaskDetail{{
			ARN:        "task-1",
			LastStatus: "RUNNING",
			Containers: []ContainerDetail{{Name: "api", LastStatus: "RUNNING"}},
		}},
		serviceDetail: []ServiceDetail{{ARN: "service", EnableExecuteCommand: true, RunningCount: 1}},
	}

	cli := newTestCLI(Dependencies{Runner: runner, Selector: selector, ECS: &fakeECSFactory{client: client}, Stdout: &strings.Builder{}, Stderr: stderr})
	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected retry path to succeed, got %d", code)
	}
	if runner.runCalls != 2 {
		t.Fatalf("expected two connect attempts, got %d", runner.runCalls)
	}
	if !strings.Contains(stderr.String(), "exec failed") {
		t.Fatalf("expected formatted connect error output, got %q", stderr.String())
	}
	if countCalls(selector.calls, "Select task (first option is default)") != 2 {
		t.Fatalf("expected task picker to be shown again after failed connect")
	}
}

func TestRunAWSConnectFailureBackReturnsToContainerSelection(t *testing.T) {
	runner := &fakeRunner{runResults: []runnerResult{{exitCode: 37, err: errors.New("exec failed")}, {exitCode: 0, err: nil}}}
	selector := &recordingSelector{selections: map[string]Option{
		"Select ECS cluster":                    {Label: "dev", Value: "cluster"},
		"Select ECS service":                    {Label: "api", Value: "service"},
		"Select container":                      {Label: "api", Value: "api"},
		"Select task (first option is default)": {Label: "task-1", Value: "task-1"},
		"Connect to cluster/service/api/task-1?": {Label: "Connect", Value: "yes"},
		connectionFailureRetryBackPrompt:        {Label: "Back", Value: string(RetryBackActionBack)},
	}}
	client := &fakeECSClient{
		clustersPages: [][]string{{"cluster"}},
		servicesPages: [][]string{{"service"}},
		tasksPages:    [][]string{{"task-1"}},
		taskDetails: []TaskDetail{{
			ARN:        "task-1",
			LastStatus: "RUNNING",
			Containers: []ContainerDetail{{Name: "api", LastStatus: "RUNNING"}},
		}},
		serviceDetail: []ServiceDetail{{ARN: "service", EnableExecuteCommand: true, RunningCount: 1}},
	}

	cli := newTestCLI(Dependencies{Runner: runner, Selector: selector, ECS: &fakeECSFactory{client: client}, Stdout: &strings.Builder{}, Stderr: &strings.Builder{}})
	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected back path to eventually succeed, got %d", code)
	}
	if runner.runCalls != 2 {
		t.Fatalf("expected two connect attempts, got %d", runner.runCalls)
	}
	if countCalls(selector.calls, "Select container") != 2 {
		t.Fatalf("expected container picker to be shown again after back")
	}
}

func TestRunAWSZeroCapacityDeclineStopsGracefully(t *testing.T) {
	stdout := &strings.Builder{}
	selector := &recordingSelector{selections: map[string]Option{
		"Select ECS cluster": {Label: "dev", Value: "cluster"},
		"Select ECS service": {Label: "api (0/0)", Value: "service"},
		"Service service has no active or pending tasks. Start one task now?": {Label: "Cancel", Value: "no"},
	}}
	client := &fakeECSClient{
		clustersPages: [][]string{{"cluster"}},
		servicesPages: [][]string{{"service"}},
		serviceDetail: []ServiceDetail{{ARN: "service", PendingCount: 0, RunningCount: 0}},
	}
	cli := newTestCLI(Dependencies{Runner: &fakeRunner{}, Selector: selector, ECS: &fakeECSFactory{client: client}, Stdout: stdout, Stderr: &strings.Builder{}})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected graceful cancel, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Task startup canceled by user") {
		t.Fatalf("expected startup cancel message, got %q", stdout.String())
	}
	if len(client.updatedDesired) != 0 {
		t.Fatalf("expected no update service call, got %#v", client.updatedDesired)
	}
}

func TestRunAWSZeroCapacityTimeoutFails(t *testing.T) {
	originalTimeout := serviceTaskStartupTimeout
	originalPoll := serviceTaskStartupPollInterval
	serviceTaskStartupTimeout = 5 * time.Millisecond
	serviceTaskStartupPollInterval = time.Millisecond
	t.Cleanup(func() {
		serviceTaskStartupTimeout = originalTimeout
		serviceTaskStartupPollInterval = originalPoll
	})

	stderr := &strings.Builder{}
	selector := &recordingSelector{selections: map[string]Option{
		"Select ECS cluster": {Label: "dev", Value: "cluster"},
		"Select ECS service": {Label: "api (0/0)", Value: "service"},
		"Service service has no active or pending tasks. Start one task now?": {Label: "Start task", Value: "yes"},
	}}
	client := &fakeECSClient{
		clustersPages: [][]string{{"cluster"}},
		servicesPages: [][]string{{"service"}},
		serviceDetails: [][]ServiceDetail{
			{{ARN: "service", PendingCount: 0, RunningCount: 0}},
			{{ARN: "service", PendingCount: 0, RunningCount: 0}},
		},
	}
	cli := newTestCLI(Dependencies{Runner: &fakeRunner{}, Selector: selector, ECS: &fakeECSFactory{client: client}, Stdout: &strings.Builder{}, Stderr: stderr})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code == 0 {
		t.Fatalf("expected non-zero on startup timeout")
	}
	if !strings.Contains(stderr.String(), "Timed out waiting for service") {
		t.Fatalf("expected timeout message, got %q", stderr.String())
	}
}

func TestRunAWSZeroCapacityStartupSkipsTaskSelection(t *testing.T) {
	startedAt := time.Now().Add(-5 * time.Minute)
	runner := &fakeRunner{}
	stdout := &strings.Builder{}
	selector := &recordingSelector{selections: map[string]Option{
		"Select ECS cluster": {Label: "dev", Value: "cluster"},
		"Select ECS service": {Label: "api (0/0)", Value: "service"},
		"Service service has no active or pending tasks. Start one task now?": {Label: "Start task", Value: "yes"},
		"Select container": {Label: "api (container-123)", Value: "api"},
		"Connect to cluster/service/api/task-123?": {Label: "Connect", Value: "yes"},
	}}
	client := &fakeECSClient{
		clustersPages: [][]string{{"cluster"}},
		servicesPages: [][]string{{"service"}},
		tasksPages:    [][]string{{"arn:task/task-123"}},
		taskDetails: []TaskDetail{{
			ARN:        "arn:task/task-123",
			StartedAt:  &startedAt,
			LastStatus: "RUNNING",
			Containers: []ContainerDetail{{Name: "api", ID: "container-123", LastStatus: "RUNNING"}},
		}},
		serviceDetails: [][]ServiceDetail{
			{{ARN: "service", PendingCount: 0, RunningCount: 0, DesiredCount: 0, EnableExecuteCommand: true}},
			{{ARN: "service", PendingCount: 1, RunningCount: 0, DesiredCount: 1, EnableExecuteCommand: true}},
			{{ARN: "service", PendingCount: 0, RunningCount: 1, DesiredCount: 1, EnableExecuteCommand: true}},
		},
	}
	cli := newTestCLI(Dependencies{Runner: runner, Selector: selector, ECS: &fakeECSFactory{client: client}, Stdout: stdout, Stderr: &strings.Builder{}})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected success, got %d", code)
	}
	if _, ok := findCall(selector.calls, "Select task (first option is default)"); ok {
		t.Fatalf("did not expect task selection when startup preselect is available")
	}
	if runner.runName != "aws" {
		t.Fatalf("expected aws command, got %q", runner.runName)
	}
	if !strings.Contains(stdout.String(), "Starting new task… please wait") {
		t.Fatalf("expected startup wait feedback, got %q", stdout.String())
	}
}

func TestRunAWSZeroCapacityStartedTaskConnectFailureReturnsToContainerSelection(t *testing.T) {
	startedAt := time.Now().Add(-5 * time.Minute)
	stdout := &strings.Builder{}
	selector := &recordingSelector{selections: map[string]Option{
		"Select ECS cluster": {Label: "dev", Value: "cluster"},
		"Select ECS service": {Label: "api (0/0)", Value: "service"},
		"Service service has no active or pending tasks. Start one task now?": {Label: "Start task", Value: "yes"},
		"Select container": {Label: "api (container-123)", Value: "api"},
		"Connect to cluster/service/api/task-123?": {Label: "Connect", Value: "yes"},
	}}
	runner := &fakeRunner{runResults: []runnerResult{{exitCode: 11, err: errors.New("connect failed")}, {exitCode: 0, err: nil}}}
	client := &fakeECSClient{
		clustersPages: [][]string{{"cluster"}},
		servicesPages: [][]string{{"service"}},
		tasksPages:    [][]string{{"arn:task/task-123"}},
		taskDetails: []TaskDetail{{
			ARN:        "arn:task/task-123",
			StartedAt:  &startedAt,
			LastStatus: "RUNNING",
			Containers: []ContainerDetail{{Name: "api", ID: "container-123", LastStatus: "RUNNING"}},
		}},
		serviceDetails: [][]ServiceDetail{
			{{ARN: "service", PendingCount: 0, RunningCount: 0, DesiredCount: 0, EnableExecuteCommand: true}},
			{{ARN: "service", PendingCount: 1, RunningCount: 0, DesiredCount: 1, EnableExecuteCommand: true}},
			{{ARN: "service", PendingCount: 0, RunningCount: 1, DesiredCount: 1, EnableExecuteCommand: true}},
		},
	}

	cli := newTestCLI(Dependencies{Runner: runner, Selector: selector, ECS: &fakeECSFactory{client: client}, Stdout: stdout, Stderr: &strings.Builder{}})
	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected retry path to succeed, got %d", code)
	}
	if runner.runCalls != 2 {
		t.Fatalf("expected two connect attempts, got %d", runner.runCalls)
	}
	if countCalls(selector.calls, "Select container") != 2 {
		t.Fatalf("expected container selection to repeat after failed connect")
	}
	if countCalls(selector.calls, "Select task (first option is default)") != 0 {
		t.Fatalf("did not expect task picker in started-task retry branch")
	}
}

func TestRunAWSZeroCapacityStartupFailureFails(t *testing.T) {
	originalTimeout := serviceTaskStartupTimeout
	originalPoll := serviceTaskStartupPollInterval
	serviceTaskStartupTimeout = 50 * time.Millisecond
	serviceTaskStartupPollInterval = time.Millisecond
	t.Cleanup(func() {
		serviceTaskStartupTimeout = originalTimeout
		serviceTaskStartupPollInterval = originalPoll
	})

	stderr := &strings.Builder{}
	selector := &recordingSelector{selections: map[string]Option{
		"Select ECS cluster": {Label: "dev", Value: "cluster"},
		"Select ECS service": {Label: "api (0/0)", Value: "service"},
		"Service service has no active or pending tasks. Start one task now?": {Label: "Start task", Value: "yes"},
	}}
	client := &fakeECSClient{
		clustersPages: [][]string{{"cluster"}},
		servicesPages: [][]string{{"service"}},
		serviceDetails: [][]ServiceDetail{
			{{ARN: "service", PendingCount: 0, RunningCount: 0, DesiredCount: 0}},
			{{ARN: "service", PendingCount: 1, RunningCount: 0, DesiredCount: 1}},
			{{ARN: "service", PendingCount: 0, RunningCount: 0, DesiredCount: 1}},
		},
	}
	cli := newTestCLI(Dependencies{Runner: &fakeRunner{}, Selector: selector, ECS: &fakeECSFactory{client: client}, Stdout: &strings.Builder{}, Stderr: stderr})

	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code == 0 {
		t.Fatalf("expected non-zero on startup failure")
	}
	if !strings.Contains(stderr.String(), "failed to start a runnable task") {
		t.Fatalf("expected startup failure message, got %q", stderr.String())
	}
}

func TestRunAWSWithScriptedUIAdapterHappyPath(t *testing.T) {
	runner := &fakeRunner{}
	ui := &scriptedUIAdapter{
		selectByLabel: map[string]Option{
			"Select ECS cluster":                    {Label: "dev", Value: "arn:aws:ecs:us-east-1:123456789012:cluster/dev"},
			"Select ECS service":                    {Label: "api", Value: "arn:aws:ecs:us-east-1:123456789012:service/dev/api"},
			"Select container":                      {Label: "api", Value: "api"},
			"Select task (first option is default)": {Label: "task-123", Value: "arn:aws:ecs:us-east-1:123456789012:task/dev/task-123"},
		},
		confirmByLabel: map[string]bool{
			"Connect to dev/api/api/task-123?": true,
		},
	}
	client := &fakeECSClient{
		clustersPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:cluster/dev"}},
		servicesPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:service/dev/api"}},
		tasksPages:    [][]string{{"arn:aws:ecs:us-east-1:123456789012:task/dev/task-123"}},
		taskDetails: []TaskDetail{{
			ARN:        "arn:aws:ecs:us-east-1:123456789012:task/dev/task-123",
			LastStatus: "RUNNING",
			Containers: []ContainerDetail{{Name: "api", LastStatus: "RUNNING"}},
		}},
		serviceDetail: []ServiceDetail{{
			ARN:                  "arn:aws:ecs:us-east-1:123456789012:service/dev/api",
			EnableExecuteCommand: true,
			RunningCount:         1,
		}},
	}

	cli := newTestCLI(Dependencies{Runner: runner, UI: ui, ECS: &fakeECSFactory{client: client}, Stdout: &strings.Builder{}, Stderr: &strings.Builder{}})
	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected success, got %d", code)
	}
	if runner.runName != "aws" {
		t.Fatalf("expected aws command, got %q", runner.runName)
	}
	if countScriptedCalls(ui.calls, "select:") == 0 {
		t.Fatalf("expected select calls through UI adapter")
	}
	if countScriptedCalls(ui.calls, "confirm:") == 0 {
		t.Fatalf("expected confirm calls through UI adapter")
	}
}

func TestRunAWSHeaderUsesCanonicalValuesNotTableRows(t *testing.T) {
	runner := &fakeRunner{}
	ui := &scriptedUIAdapter{
		selectByLabel: map[string]Option{
			"Select ECS cluster":                    {Label: "dev | 2 | 0 | 1", Value: "arn:aws:ecs:us-east-1:123456789012:cluster/dev"},
			"Select ECS service":                    {Label: "api | RUNNING | 1 | 0 | 1 | 5m ago", Value: "arn:aws:ecs:us-east-1:123456789012:service/dev/api"},
			"Select container":                      {Label: "api | frontend | RUNNING", Value: "api"},
			"Select task (first option is default)": {Label: "task-123 | RUNNING | 256 | 512 | 1m ago", Value: "arn:aws:ecs:us-east-1:123456789012:task/dev/task-123"},
		},
		confirmByLabel: map[string]bool{
			"Connect to dev/api/api/task-123?": true,
		},
	}
	client := &fakeECSClient{
		clustersPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:cluster/dev"}},
		servicesPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:service/dev/api"}},
		tasksPages:    [][]string{{"arn:aws:ecs:us-east-1:123456789012:task/dev/task-123"}},
		taskDetails: []TaskDetail{{
			ARN:        "arn:aws:ecs:us-east-1:123456789012:task/dev/task-123",
			LastStatus: "RUNNING",
			Containers: []ContainerDetail{{Name: "api", LastStatus: "RUNNING"}},
		}},
		serviceDetail: []ServiceDetail{{
			ARN:                  "arn:aws:ecs:us-east-1:123456789012:service/dev/api",
			EnableExecuteCommand: true,
			RunningCount:         1,
		}},
	}

	cli := newTestCLI(Dependencies{Runner: runner, UI: ui, ECS: &fakeECSFactory{client: client}, Stdout: &strings.Builder{}, Stderr: &strings.Builder{}})
	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected success, got %d", code)
	}

	clusterLine := ""
	for _, call := range ui.calls {
		if strings.Contains(call, "message:Cluster:") && strings.Contains(call, "dev") {
			clusterLine = call
		}
	}
	if clusterLine == "" {
		t.Fatalf("expected cluster header line in calls: %#v", ui.calls)
	}
	if strings.Contains(clusterLine, "|") {
		t.Fatalf("expected canonical header values without table separators, got %q", clusterLine)
	}
}

func TestRunAWSPromptUsesCanonicalTupleWithoutColumns(t *testing.T) {
	runner := &fakeRunner{}
	ui := &scriptedUIAdapter{
		selectByLabel: map[string]Option{
			"Select ECS cluster":                    {Label: "dev | 2 | 0 | 1", Value: "arn:aws:ecs:us-east-1:123456789012:cluster/dev"},
			"Select ECS service":                    {Label: "api | RUNNING | 1 | 0 | 1 | 5m ago", Value: "arn:aws:ecs:us-east-1:123456789012:service/dev/api"},
			"Select container":                      {Label: "api | frontend | RUNNING", Value: "api"},
			"Select task (first option is default)": {Label: "task-123 | RUNNING | 256 | 512 | 1m ago", Value: "arn:aws:ecs:us-east-1:123456789012:task/dev/task-123"},
		},
		confirmByLabel: map[string]bool{
			"Connect to dev/api/api/task-123?": true,
		},
	}
	client := &fakeECSClient{
		clustersPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:cluster/dev"}},
		servicesPages: [][]string{{"arn:aws:ecs:us-east-1:123456789012:service/dev/api"}},
		tasksPages:    [][]string{{"arn:aws:ecs:us-east-1:123456789012:task/dev/task-123"}},
		taskDetails: []TaskDetail{{
			ARN:        "arn:aws:ecs:us-east-1:123456789012:task/dev/task-123",
			LastStatus: "RUNNING",
			Containers: []ContainerDetail{{Name: "api", LastStatus: "RUNNING"}},
		}},
		serviceDetail: []ServiceDetail{{
			ARN:                  "arn:aws:ecs:us-east-1:123456789012:service/dev/api",
			EnableExecuteCommand: true,
			RunningCount:         1,
		}},
	}

	cli := newTestCLI(Dependencies{Runner: runner, UI: ui, ECS: &fakeECSFactory{client: client}, Stdout: &strings.Builder{}, Stderr: &strings.Builder{}})
	code := cli.Run(context.Background(), Config{Mode: ModeAWS, Profile: "dev"})
	if code != 0 {
		t.Fatalf("expected success, got %d", code)
	}

	confirmCall := ""
	for _, call := range ui.calls {
		if strings.HasPrefix(call, "confirm:") {
			confirmCall = call
			break
		}
	}
	if confirmCall == "" {
		t.Fatalf("expected confirm call, got %#v", ui.calls)
	}
	if confirmCall != "confirm:Connect to dev/api/api/task-123?" {
		t.Fatalf("unexpected confirm prompt: %q", confirmCall)
	}
	if strings.Contains(confirmCall, "|") {
		t.Fatalf("confirm prompt must not include table separators: %q", confirmCall)
	}
}

type fakeSelectorSequence struct {
	selections []Option
	err        error
	index      int
}

func (f *fakeSelectorSequence) Select(_ string, options []Option, _ int) (Option, error) {
	if f.err != nil {
		return Option{}, f.err
	}
	if len(f.selections) == 0 {
		return options[0], nil
	}
	if f.index >= len(f.selections) {
		return options[0], nil
	}
	next := f.selections[f.index]
	f.index++
	return next, nil
}

func equalOptions(got []Option, want []Option) bool {
	if len(got) != len(want) {
		return false
	}
	for idx := range got {
		if got[idx] != want[idx] {
			return false
		}
	}

	return true
}

func findCall(calls []selectionCall, label string) (selectionCall, bool) {
	for _, call := range calls {
		if call.label == label {
			return call, true
		}
	}

	return selectionCall{}, false
}

func countCalls(calls []selectionCall, label string) int {
	count := 0
	for _, call := range calls {
		if call.label == label {
			count++
		}
	}

	return count
}

func countScriptedCalls(calls []string, prefix string) int {
	count := 0
	for _, call := range calls {
		if strings.HasPrefix(call, prefix) {
			count++
		}
	}

	return count
}
