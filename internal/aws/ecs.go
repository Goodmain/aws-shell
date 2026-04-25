package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/example/aws-shell/internal/app"
)

type SDKFactory struct{}

func NewSDKFactory() *SDKFactory {
	return &SDKFactory{}
}

func (f *SDKFactory) New(ctx context.Context) (app.ECSClient, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	client := ecs.NewFromConfig(cfg)
	return &SDKClient{client: client}, nil
}

type SDKClient struct {
	client *ecs.Client
}

func (c *SDKClient) ListClusters(ctx context.Context, nextToken *string) ([]string, *string, error) {
	input := &ecs.ListClustersInput{}
	if nextToken != nil {
		input.NextToken = nextToken
	}

	out, err := c.client.ListClusters(ctx, input)
	if err != nil {
		return nil, nil, err
	}

	clusters := make([]string, 0, len(out.ClusterArns))
	for _, arn := range out.ClusterArns {
		clusters = append(clusters, arn)
	}

	return clusters, out.NextToken, nil
}

func (c *SDKClient) ListServices(ctx context.Context, clusterArn string, nextToken *string) ([]string, *string, error) {
	input := &ecs.ListServicesInput{
		Cluster: &clusterArn,
	}
	if nextToken != nil {
		input.NextToken = nextToken
	}

	out, err := c.client.ListServices(ctx, input)
	if err != nil {
		return nil, nil, err
	}

	services := make([]string, 0, len(out.ServiceArns))
	for _, arn := range out.ServiceArns {
		services = append(services, arn)
	}

	return services, out.NextToken, nil
}

func (c *SDKClient) ListTasks(ctx context.Context, clusterArn string, serviceName string, nextToken *string) ([]string, *string, error) {
	input := &ecs.ListTasksInput{
		Cluster:       &clusterArn,
		ServiceName:   &serviceName,
		DesiredStatus: types.DesiredStatusRunning,
	}
	if nextToken != nil {
		input.NextToken = nextToken
	}

	out, err := c.client.ListTasks(ctx, input)
	if err != nil {
		return nil, nil, err
	}

	tasks := make([]string, 0, len(out.TaskArns))
	for _, arn := range out.TaskArns {
		tasks = append(tasks, arn)
	}

	return tasks, out.NextToken, nil
}

func (c *SDKClient) DescribeTasks(ctx context.Context, clusterArn string, taskArns []string) ([]app.TaskDetail, error) {
	if len(taskArns) == 0 {
		return []app.TaskDetail{}, nil
	}

	input := &ecs.DescribeTasksInput{
		Cluster: &clusterArn,
		Tasks:   taskArns,
	}

	out, err := c.client.DescribeTasks(ctx, input)
	if err != nil {
		return nil, err
	}

	details := make([]app.TaskDetail, 0, len(out.Tasks))
	for _, task := range out.Tasks {
		containers := make([]app.ContainerDetail, 0, len(task.Containers))
		for _, container := range task.Containers {
			containers = append(containers, app.ContainerDetail{
				Name:       value(container.Name),
				ID:         value(container.RuntimeId),
				Image:      value(container.Image),
				LastStatus: value(container.LastStatus),
				CPU:        value(container.Cpu),
				Memory:     value(container.Memory),
			})
		}

		details = append(details, app.TaskDetail{
			ARN:        value(task.TaskArn),
			LastStatus: value(task.LastStatus),
			CreatedAt:  task.CreatedAt,
			StartedAt:  task.StartedAt,
			CPU:        value(task.Cpu),
			Memory:     value(task.Memory),
			StopReason: value(task.StoppedReason),
			Containers: containers,
		})
	}

	return details, nil
}

func (c *SDKClient) DescribeServices(ctx context.Context, clusterArn string, serviceArns []string) ([]app.ServiceDetail, error) {
	if len(serviceArns) == 0 {
		return []app.ServiceDetail{}, nil
	}

	input := &ecs.DescribeServicesInput{
		Cluster:  &clusterArn,
		Services: serviceArns,
	}

	out, err := c.client.DescribeServices(ctx, input)
	if err != nil {
		return nil, err
	}

	details := make([]app.ServiceDetail, 0, len(out.Services))
	for _, service := range out.Services {
		details = append(details, app.ServiceDetail{
			ARN:                  value(service.ServiceArn),
			Status:               value(service.Status),
			EnableExecuteCommand: service.EnableExecuteCommand,
			CreatedAt:            service.CreatedAt,
			PendingCount:         service.PendingCount,
			RunningCount:         service.RunningCount,
			DesiredCount:         service.DesiredCount,
		})
	}

	return details, nil
}

func (c *SDKClient) UpdateServiceDesiredCount(ctx context.Context, clusterArn string, serviceArn string, desiredCount int32) error {
	_, err := c.client.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:      &clusterArn,
		Service:      &serviceArn,
		DesiredCount: &desiredCount,
	})

	return err
}

func value(v *string) string {
	if v == nil {
		return ""
	}

	return *v
}
