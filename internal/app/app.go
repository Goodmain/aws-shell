package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/example/aws-shell/internal/system"
)

type Config struct {
	Mode       string
	Profile    string
	Executable string
}

type Dependencies struct {
	Runner      Runner
	Selector    Selector
	UI          UIAdapter
	ECS         ECSFactory
	Preferences ProfilePreferenceStore
	Stdout      io.Writer
	Stderr      io.Writer
}

type CLI struct {
	runner   Runner
	selector Selector
	ui       UIAdapter
	ecs      ECSFactory
	prefs    ProfilePreferenceStore
	stdout   io.Writer
	stderr   io.Writer
}

var (
	serviceTaskStartupTimeout      = 2 * time.Minute
	serviceTaskStartupPollInterval = 2 * time.Second
)

const connectionFailureRetryBackPrompt = "Connection failed. Choose next action"

type wizardState struct {
	profile   string
	region    string
	cluster   string
	service   string
	container string
	task      string
	step      string
}

type wizardSelectConfig struct {
	allowBack    bool
	allowRefresh bool
}

type uiSuspender interface {
	Suspend(run func())
}

func NewCLI(deps Dependencies) *CLI {
	stdout := deps.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := deps.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	prefs := deps.Preferences
	if prefs == nil {
		prefs = noopProfilePreferenceStore{}
	}

	selector := deps.Selector
	ui := deps.UI
	if ui == nil {
		panic("ui adapter is required")
	}

	return &CLI{
		runner:   deps.Runner,
		selector: selector,
		ui:       ui,
		ecs:      deps.ECS,
		prefs:    prefs,
		stdout:   stdout,
		stderr:   stderr,
	}
}

func (c *CLI) Run(ctx context.Context, cfg Config) int {
	switch cfg.Mode {
	case ModeBootstrap:
		return c.runBootstrap(ctx, cfg)
	case ModeAWS:
		return c.runAWS(ctx, cfg)
	default:
		c.ui.Error(fmt.Sprintf("invalid mode %q (expected %q or %q)", cfg.Mode, ModeBootstrap, ModeAWS))
		return 1
	}
}

func (c *CLI) runBootstrap(ctx context.Context, cfg Config) int {
	if _, err := c.runner.LookPath("aws-vault"); err != nil {
		c.ui.Error("aws-vault is required but was not found in PATH. Install aws-vault and try again.")
		return 1
	}

	profilesOutput, err := c.runner.Output(ctx, "aws-vault", "list", "--profiles")
	if err != nil {
		c.ui.Error(fmt.Sprintf("failed to list aws-vault profiles: %v", err))
		return 1
	}

	profiles := ParseProfiles(profilesOutput)
	if len(profiles) == 0 {
		c.ui.Error("no aws-vault profiles found. Configure at least one profile and try again.")
		return 1
	}

	profileOptions := BuildProfileOptions(profiles)
	defaultProfileIndex := 0
	lastUsedProfile, err := c.prefs.LoadLastUsedProfile()
	if err != nil {
		c.ui.Error(fmt.Sprintf("warning: failed to load last-used profile preference: %v", err))
	} else if strings.TrimSpace(lastUsedProfile) != "" {
		defaultProfileIndex = DefaultOptionIndex(profileOptions, lastUsedProfile)
	}

	if c.selector == nil {
		c.ui.Error("profile selector is not configured")
		return 1
	}

	selected, err := c.selector.Select("Select AWS profile", profileOptions, defaultProfileIndex)
	if err != nil {
		if errors.Is(err, ErrSelectionCanceled) {
			c.ui.Error("profile selection canceled.")
			return 1
		}

		c.ui.Error(fmt.Sprintf("failed to select profile: %v", err))
		return 1
	}

	if err := c.prefs.SaveLastUsedProfile(selected.Value); err != nil {
		c.ui.Error(fmt.Sprintf("warning: failed to persist last-used profile preference: %v", err))
	}

	if strings.TrimSpace(cfg.Executable) == "" {
		c.ui.Error("cannot run credentialed mode: executable path is empty")
		return 1
	}

	fmt.Fprintf(c.stdout, "Selected profile %q. Launching aws-vault...\n", selected.Value)

	args := system.ComposeAWSVaultExecArgs(cfg.Executable, selected.Value)
	exitCode, execErr := c.runner.Run(ctx, "aws-vault", args...)
	if execErr != nil {
		c.ui.Error(fmt.Sprintf("aws-vault execution failed: %v", execErr))
		if exitCode > 0 {
			return exitCode
		}
		return 1
	}

	return exitCode
}

func (c *CLI) runAWS(ctx context.Context, cfg Config) int {
	if strings.TrimSpace(cfg.Profile) == "" {
		c.ui.Error(fmt.Sprintf("--profile is required in %q mode", ModeAWS))
		return 1
	}

	client, err := c.ecs.New(ctx)
	if err != nil {
		c.ui.Error(fmt.Sprintf("failed to initialize AWS configuration: %v", err))
		return 1
	}

	state := wizardState{profile: cfg.Profile, region: "-", step: "profile"}

	selectedClusterValue := ""

forCluster:
	for {
		clusters, err := ListAllClusters(ctx, client)
		if err != nil {
			c.ui.Error(fmt.Sprintf("failed to list ECS clusters: %v", err))
			return 1
		}

		if len(clusters) == 0 {
			c.ui.Message(fmt.Sprintf("No ECS clusters found for profile %q.", cfg.Profile))
			return 0
		}

		clusterStatsByARN := make(map[string]ClusterStats, len(clusters))
		for _, clusterARN := range clusters {
			stats, statsErr := DescribeClusterStats(ctx, client, clusterARN)
			if statsErr != nil {
				c.ui.Error(fmt.Sprintf("failed to summarize ECS cluster %q: %v", ResourceName(clusterARN), statsErr))
				return 1
			}
			clusterStatsByARN[clusterARN] = stats
		}

		clusterOptions := BuildClusterOptionsWithStats(clusters, clusterStatsByARN)
		var clusterDefaultIndex int
		if selectedClusterValue != "" {
			if idx, ok := defaultIndexForValue(clusterOptions, selectedClusterValue); ok {
				clusterDefaultIndex = idx
			} else {
				clusterDefaultIndex = 1
				c.ui.Message("Previously selected cluster is no longer available; defaulting to the first cluster.")
				selectedClusterValue = ""
			}
		} else {
			clusterDefaultIndex = 1
		}

		state.step = "cluster"
		selectedCluster, clusterAction, err := c.selectWizardOption(
			"Select ECS cluster",
			clusterOptions,
			clusterDefaultIndex,
			state,
			wizardSelectConfig{allowRefresh: true},
		)
		if err != nil {
			if errors.Is(err, ErrSelectionCanceled) {
				c.ui.Error("cluster selection canceled.")
				return 1
			}

			c.ui.Error(fmt.Sprintf("failed to select cluster: %v", err))
			return 1
		}

		switch clusterAction {
		case WizardActionQuit:
			return c.quitSession()
		case WizardActionRefresh:
			continue forCluster
		case WizardActionBack:
			c.ui.Message("Already at the first step. Nothing to go back to.")
			continue forCluster
		}

		selectedClusterValue = selectedCluster.Value

		selectedClusterModel := ClusterSelection{ARN: selectedCluster.Value, Label: selectedCluster.Label}
		state.service = ""
		state.container = ""
		state.task = ""
		state.region = displayOrDash(regionFromARN(selectedClusterModel.ARN))

		clusterName := ResourceName(selectedClusterModel.ARN)
		if clusterName == "" {
			clusterName = strings.TrimSpace(selectedClusterModel.ARN)
		}
		if clusterName == "" {
			clusterName = selectedClusterModel.Label
		}
		state.cluster = clusterName

		selectedServiceValue := ""

	forService:
		for {
			services, err := ListAllServices(ctx, client, selectedClusterModel.ARN)
			if err != nil {
				c.ui.Error(fmt.Sprintf("failed to list ECS services: %v", err))
				return 1
			}

			if len(services) == 0 {
				c.ui.Message(fmt.Sprintf("No ECS services found in cluster %q.", clusterName))
				return 0
			}

			serviceDetails, err := DescribeAllServices(ctx, client, selectedClusterModel.ARN, services)
			if err != nil {
				c.ui.Error(fmt.Sprintf("failed to describe ECS services: %v", err))
				return 1
			}
			serviceOptions := BuildServiceOptions(services, serviceDetails)
			var serviceDefaultIndex int

			if selectedServiceValue != "" {
				if idx, ok := defaultIndexForValue(serviceOptions, selectedServiceValue); ok {
					serviceDefaultIndex = idx
				} else {
					serviceDefaultIndex = 1
					c.ui.Message("Previously selected service is no longer available; defaulting to the first service.")
					selectedServiceValue = ""
				}
			} else {
				serviceDefaultIndex = 1
			}

			state.step = "service"
			selectedService, serviceAction, err := c.selectWizardOption(
				"Select ECS service",
				serviceOptions,
				serviceDefaultIndex,
				state,
				wizardSelectConfig{allowBack: true, allowRefresh: true},
			)
			if err != nil {
				if errors.Is(err, ErrSelectionCanceled) {
					c.ui.Error("service selection canceled.")
					return 1
				}

				c.ui.Error(fmt.Sprintf("failed to select service: %v", err))
				return 1
			}

			switch serviceAction {
			case WizardActionQuit:
				return c.quitSession()
			case WizardActionRefresh:
				continue forService
			case WizardActionBack:
				state.service = ""
				state.container = ""
				state.task = ""
				continue forCluster
			}

			selectedServiceValue = selectedService.Value

			selectedServiceName := ResourceName(selectedService.Value)
			if selectedServiceName == "" {
				selectedServiceName = selectedService.Label
			}
			selectedServiceModel := ServiceSelection{ARN: selectedService.Value, Label: selectedServiceName}
			selectedServiceDetail, hasServiceDetail := serviceDetails[selectedServiceModel.ARN]
			state.service = selectedServiceModel.Label
			state.container = ""
			state.task = ""

			var selectedTask Option
			startupTaskPreselected := false
			if hasServiceDetail && IsServiceZeroCapacity(selectedServiceDetail) {
				startPrompt := fmt.Sprintf("Service %s has no active or pending tasks. Start one task now?", selectedServiceModel.Label)
				startChoice, startErr := c.ui.Confirm(startPrompt, "Start task", "Cancel", true)
				if startErr != nil {
					if errors.Is(startErr, ErrSelectionCanceled) {
						c.ui.Message("Task startup canceled by user.")
						return 0
					}

					c.ui.Error(fmt.Sprintf("failed to confirm task startup: %v", startErr))
					return 1
				}
				if !startChoice {
					c.ui.Message("Task startup canceled by user.")
					return 0
				}

				stopStartupAnimation := c.startTaskStartupAnimation("Starting new task… please wait")
				startedTask, startErr := StartServiceTaskAndWait(ctx, client, selectedClusterModel, selectedServiceModel, selectedServiceDetail, serviceTaskStartupTimeout, serviceTaskStartupPollInterval)
				stopStartupAnimation()
				if startErr != nil {
					switch {
					case errors.Is(startErr, ErrTaskStartupTimedOut):
						c.ui.Error(fmt.Sprintf("Timed out waiting for service %q to start a task. Check ECS service events and capacity, then retry.", selectedServiceModel.Label))
					case errors.Is(startErr, ErrTaskStartupFailed):
						c.ui.Error(fmt.Sprintf("Service %q failed to start a runnable task. Check ECS service events and task definition, then retry.", selectedServiceModel.Label))
					default:
						c.ui.Error(fmt.Sprintf("failed to start task for service %q: %v", selectedServiceModel.Label, startErr))
					}
					return 1
				}

				selectedTask = Option{Label: startedTask.Label, Value: startedTask.ARN}
				startupTaskPreselected = true
				state.task = displayOrDash(ResourceName(selectedTask.Value))
			}

			activeTask := Option{}
			taskFromStartup := false
			if startupTaskPreselected && selectedTask.Value != "" {
				tasks, err := DiscoverRunnableTasks(ctx, client, selectedClusterModel, selectedServiceModel, ContainerSelection{})
				if err != nil {
					c.ui.Error(fmt.Sprintf("failed to discover running tasks: %v", err))
					return 1
				}
				if IsTaskStillRunnable(tasks, selectedTask.Value) {
					activeTask = selectedTask
					taskFromStartup = true
				}
			}

		forTask:
			for {
				if activeTask.Value == "" {
					tasks, err := DiscoverRunnableTasks(ctx, client, selectedClusterModel, selectedServiceModel, ContainerSelection{})
					if err != nil {
						c.ui.Error(fmt.Sprintf("failed to discover running tasks: %v", err))
						return 1
					}

					if len(tasks) == 0 {
						c.ui.Message(fmt.Sprintf("No running tasks available for service %q.", selectedServiceModel.Label))
						return 0
					}

					taskOptions := BuildTaskOptions(tasks)
					var taskDefaultIndex int
					if selectedTask.Value != "" {
						if idx, ok := defaultIndexForValue(taskOptions, selectedTask.Value); ok {
							taskDefaultIndex = idx
						} else {
							taskDefaultIndex = 1
							c.ui.Message("Previously selected task is no longer available; defaulting to the first task.")
							selectedTask = Option{}
						}
					} else {
						taskDefaultIndex = 1
					}

					state.step = "task"
					state.container = ""
					var taskAction string
					activeTask, taskAction, err = c.selectWizardOption(
						"Select task (first option is default)",
						taskOptions,
						taskDefaultIndex,
						state,
						wizardSelectConfig{allowBack: true, allowRefresh: true},
					)
					if err != nil {
						if errors.Is(err, ErrSelectionCanceled) {
							c.ui.Error("task selection canceled.")
							return 1
						}

						c.ui.Error(fmt.Sprintf("failed to select task: %v", err))
						return 1
					}

					switch taskAction {
					case WizardActionQuit:
						return c.quitSession()
					case WizardActionRefresh:
						continue forTask
					case WizardActionBack:
						activeTask = Option{}
						continue forService
					}

					taskFromStartup = false
					selectedTask = activeTask
				}

				taskID := ResourceName(activeTask.Value)
				if taskID == "" {
					taskID = activeTask.Label
				}
				state.task = taskID

				selectedContainerValue := ""
				containerDefaultIndex := 0

			forContainer:
				for {
					containers, err := DiscoverTaskContainers(ctx, client, selectedClusterModel, activeTask.Value)
					if err != nil {
						c.ui.Error(fmt.Sprintf("failed to discover task containers: %v", err))
						return 1
					}

					if len(containers) == 0 {
						c.ui.Message(fmt.Sprintf("No runnable containers found for task %q.", taskID))
						return 0
					}

					containerOptions := BuildContainerOptions(containers)
					if selectedContainerValue != "" {
						if idx, ok := defaultIndexForValue(containerOptions, selectedContainerValue); ok {
							containerDefaultIndex = idx
						} else {
							containerDefaultIndex = 0
							c.ui.Message("Previously selected container is no longer available; defaulting to the first container.")
							selectedContainerValue = ""
						}
					}

					state.step = "container"
					selectedContainer, containerAction, err := c.selectWizardOption(
						"Select container",
						containerOptions,
						containerDefaultIndex,
						state,
						wizardSelectConfig{allowBack: true, allowRefresh: true},
					)
					if err != nil {
						if errors.Is(err, ErrSelectionCanceled) {
							c.ui.Error("container selection canceled.")
							return 1
						}

						c.ui.Error(fmt.Sprintf("failed to select container: %v", err))
						return 1
					}

					switch containerAction {
					case WizardActionQuit:
						return c.quitSession()
					case WizardActionRefresh:
						continue forContainer
					case WizardActionBack:
						state.container = ""
						selectedContainerValue = ""
						activeTask = Option{}
						continue forTask
					}

					selectedContainerValue = selectedContainer.Value
					containerDefaultIndex, _ = defaultIndexForValue(containerOptions, selectedContainerValue)

					selectedContainerModel := ContainerSelection{Name: selectedContainer.Value, Label: selectedContainer.Label}
					state.container = selectedContainerModel.Name

					confirmPrompt := fmt.Sprintf("Connect to %s/%s/%s/%s?", clusterName, selectedServiceModel.Label, selectedContainerModel.Name, taskID)
					confirmation, err := c.ui.Confirm(confirmPrompt, "Connect", "Cancel", true)
					if err != nil {
						if errors.Is(err, ErrSelectionCanceled) {
							c.ui.Error("connection confirmation canceled.")
							return 1
						}

						c.ui.Error(fmt.Sprintf("failed to confirm task selection: %v", err))
						return 1
					}

					if !confirmation {
						c.ui.Message("Connection canceled.")
						continue forContainer
					}

					execEnabled, err := ServiceExecEnabled(ctx, client, selectedClusterModel.ARN, selectedServiceModel.ARN)
					if err != nil {
						c.ui.Error(fmt.Sprintf("failed to validate ECS Exec prerequisites: %v", err))
						return 1
					}
					if !execEnabled {
						c.ui.Error("ECS Exec is not enabled for the selected service. Enable ECS Exec and retry.")
						return 1
					}

					revalidatedTasks, err := DiscoverRunnableTasks(ctx, client, selectedClusterModel, selectedServiceModel, selectedContainerModel)
					if err != nil {
						c.ui.Error(fmt.Sprintf("failed to revalidate selected task: %v", err))
						return 1
					}
					if !IsTaskStillRunnable(revalidatedTasks, activeTask.Value) {
						c.ui.Error("Selected task/container is no longer runnable. Rerun and reselect a task.")
						return 1
					}

					args := system.ComposeAWSECSExecArgs(selectedClusterModel.ARN, activeTask.Value, selectedContainerModel.Name, "/bin/sh")
					exitCode, execErr := c.runWithUISuspension(ctx, "aws", args...)
					if execErr != nil {
						c.ui.Error(FormatConnectError(cfg.Profile, execErr))
						nextAction, actionErr := c.ui.RetryOrBack(connectionFailureRetryBackPrompt, true)
						if actionErr != nil {
							if errors.Is(actionErr, ErrSelectionCanceled) {
								c.ui.Error("connection retry selection canceled.")
								return 1
							}

							c.ui.Error(fmt.Sprintf("failed to capture retry/back action: %v", actionErr))
							return 1
						}
						if nextAction == RetryBackActionBack {
							activeTask = Option{}
							continue forTask
						}
						if taskFromStartup {
							continue forTask
						}
						activeTask = Option{}
						continue forTask
					}

					return exitCode
				}
			}
		}
	}
}

func (c *CLI) selectWizardOption(label string, options []Option, defaultIndex int, state wizardState, cfg wizardSelectConfig) (Option, string, error) {
	for {
		c.renderWizardHeader(state)

		selection, err := c.ui.Select(label, options, defaultIndex)
		if err != nil {
			return Option{}, "", err
		}

		switch selection.Value {
		case WizardActionBack, WizardActionRefresh, WizardActionQuit:
			if selection.Value == WizardActionBack && !cfg.allowBack {
				c.ui.Message("Already at the first step. Nothing to go back to.")
				continue
			}
			if selection.Value == WizardActionRefresh && !cfg.allowRefresh {
				continue
			}
			return Option{}, selection.Value, nil
		case WizardTableHeaderValue:
			continue
		default:
			return selection, "", nil
		}
	}
}

func (c *CLI) renderWizardHeader(state wizardState) {
	c.ui.Message("=== ECS Connect Wizard ===")
	c.ui.Message(fmt.Sprintf("Profile: %s  Region: %s", displayOrDash(state.profile), displayOrDash(state.region)))
	c.ui.Message(fmt.Sprintf("Cluster: %s  Service: %s", displayOrDash(state.cluster), displayOrDash(state.service)))
	c.ui.Message(fmt.Sprintf("Container: %s  Task: %s", displayOrDash(state.container), displayOrDash(state.task)))
	c.ui.Message(fmt.Sprintf("Step: %s    Keys: ↑/↓: move  Enter: select  b: back  r: refresh  q: quit", displayOrDash(state.step)))
}

func defaultIndexForValue(options []Option, value string) (int, bool) {
	for idx, option := range options {
		if option.Value == value {
			return idx, true
		}
	}

	return 0, false
}

func regionFromARN(arn string) string {
	parts := strings.Split(strings.TrimSpace(arn), ":")
	if len(parts) < 4 {
		return ""
	}

	return strings.TrimSpace(parts[3])
}

func (c *CLI) startTaskStartupAnimation(message string) func() {
	if c.stdout == nil {
		return func() {}
	}

	fmt.Fprint(c.stdout, message)

	done := make(chan struct{})
	stopped := make(chan struct{})

	var once sync.Once
	go func() {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		defer close(stopped)

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				fmt.Fprint(c.stdout, ".")
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(done)
			<-stopped
			fmt.Fprintln(c.stdout)
		})
	}
}

func displayOrDash(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "-"
	}

	return trimmed
}

func (c *CLI) runWithUISuspension(ctx context.Context, name string, args ...string) (int, error) {
	if suspender, ok := c.ui.(uiSuspender); ok {
		exitCode := 0
		var runErr error
		suspender.Suspend(func() {
			exitCode, runErr = c.runner.Run(ctx, name, args...)
		})
		return exitCode, runErr
	}

	return c.runner.Run(ctx, name, args...)
}

func (c *CLI) quitSession() int {
	_ = c.ui.Quit()
	return 0
}
