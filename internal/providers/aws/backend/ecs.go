package aws

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// ── Public types ──────────────────────────────────────────────────────────────

// EcsCluster is a simplified view of an ECS cluster.
type EcsCluster struct {
	Name string
	Arn  string
}

// EcsServiceSummary is a simplified view of an ECS service.
type EcsServiceSummary struct {
	Name string
	Arn  string
}

// EcsTaskSummary is a simplified view of a running ECS task.
type EcsTaskSummary struct {
	TaskID string // short ID (last segment of ARN)
	Arn    string
}

// CWLEvent is a single CloudWatch Logs event.
type CWLEvent struct {
	Timestamp time.Time
	Message   string
}

// ContainerLogs holds the log events for one container inside an ECS task.
type ContainerLogs struct {
	ContainerName string
	LogGroup      string
	LogStream     string
	Events        []CWLEvent
}

// ── Interfaces ────────────────────────────────────────────────────────────────

type ecsListAPI interface {
	ListClusters(
		ctx context.Context,
		params *ecs.ListClustersInput,
		optFns ...func(*ecs.Options),
	) (*ecs.ListClustersOutput, error)

	ListServices(
		ctx context.Context,
		params *ecs.ListServicesInput,
		optFns ...func(*ecs.Options),
	) (*ecs.ListServicesOutput, error)

	ListTasks(
		ctx context.Context,
		params *ecs.ListTasksInput,
		optFns ...func(*ecs.Options),
	) (*ecs.ListTasksOutput, error)

	DescribeTasks(
		ctx context.Context,
		params *ecs.DescribeTasksInput,
		optFns ...func(*ecs.Options),
	) (*ecs.DescribeTasksOutput, error)

	DescribeTaskDefinition(
		ctx context.Context,
		params *ecs.DescribeTaskDefinitionInput,
		optFns ...func(*ecs.Options),
	) (*ecs.DescribeTaskDefinitionOutput, error)
}

type ecsCWLAPI interface {
	GetLogEvents(
		ctx context.Context,
		params *cloudwatchlogs.GetLogEventsInput,
		optFns ...func(*cloudwatchlogs.Options),
	) (*cloudwatchlogs.GetLogEventsOutput, error)
}

// ── Service ───────────────────────────────────────────────────────────────────

// EcsService provides read and log-fetch access to Amazon ECS.
type EcsService struct {
	ecs ecsListAPI
	cwl ecsCWLAPI
}

// InitEcsService creates an EcsService from an AWS config.
func InitEcsService(cfg aws.Config) *EcsService {
	slog.Debug("Initializing ECS Service")
	return &EcsService{
		ecs: ecs.NewFromConfig(cfg),
		cwl: cloudwatchlogs.NewFromConfig(cfg),
	}
}

// ListClusters returns all ECS cluster names visible to the caller.
func (s *EcsService) ListClusters(ctx context.Context) ([]EcsCluster, error) {
	var clusters []EcsCluster
	var nextToken *string

	for {
		out, err := s.ecs.ListClusters(ctx, &ecs.ListClustersInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list ECS clusters: %w", err)
		}
		for _, arn := range out.ClusterArns {
			clusters = append(clusters, EcsCluster{
				Name: lastArnSegment(arn),
				Arn:  arn,
			})
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	slog.Debug(fmt.Sprintf("ecs: found %d clusters", len(clusters)))
	return clusters, nil
}

// ListServices returns all service names in the given cluster.
func (s *EcsService) ListServices(ctx context.Context, clusterName string) ([]EcsServiceSummary, error) {
	var services []EcsServiceSummary
	var nextToken *string

	for {
		out, err := s.ecs.ListServices(ctx, &ecs.ListServicesInput{
			Cluster:   aws.String(clusterName),
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list ECS services: %w", err)
		}
		for _, arn := range out.ServiceArns {
			services = append(services, EcsServiceSummary{
				Name: lastArnSegment(arn),
				Arn:  arn,
			})
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return services, nil
}

// ListRunningTasks returns running task summaries for the given cluster and
// service. Returns at most 100 tasks (one page).
func (s *EcsService) ListRunningTasks(ctx context.Context, clusterName, serviceName string) ([]EcsTaskSummary, error) {
	out, err := s.ecs.ListTasks(ctx, &ecs.ListTasksInput{
		Cluster:       aws.String(clusterName),
		ServiceName:   aws.String(serviceName),
		DesiredStatus: ecstypes.DesiredStatusRunning,
	})
	if err != nil {
		return nil, fmt.Errorf("list ECS tasks: %w", err)
	}

	tasks := make([]EcsTaskSummary, 0, len(out.TaskArns))
	for _, arn := range out.TaskArns {
		tasks = append(tasks, EcsTaskSummary{
			TaskID: lastArnSegment(arn),
			Arn:    arn,
		})
	}
	return tasks, nil
}

// GetContainerLogs fetches CloudWatch Logs for every awslogs-configured
// container in the given task. taskID is the short task identifier (last ARN
// segment); the cluster name is required to describe the task.
func (s *EcsService) GetContainerLogs(ctx context.Context, clusterName, taskID string, tailLines int32) ([]ContainerLogs, error) {
	// Describe the task to get the task definition ARN.
	tasksOut, err := s.ecs.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(clusterName),
		Tasks:   []string{taskID},
	})
	if err != nil {
		return nil, fmt.Errorf("describe ECS task: %w", err)
	}
	if len(tasksOut.Tasks) == 0 {
		return nil, fmt.Errorf("task %q not found in cluster %q", taskID, clusterName)
	}

	task := tasksOut.Tasks[0]
	if task.TaskDefinitionArn == nil {
		return nil, fmt.Errorf("task has no task definition ARN")
	}

	// Describe the task definition to get container log configurations.
	defOut, err := s.ecs.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: task.TaskDefinitionArn,
	})
	if err != nil {
		return nil, fmt.Errorf("describe task definition: %w", err)
	}
	if defOut.TaskDefinition == nil {
		return nil, fmt.Errorf("task definition is nil")
	}

	// Pull logs for each awslogs-configured container.
	var results []ContainerLogs
	for _, cd := range defOut.TaskDefinition.ContainerDefinitions {
		logs, err := s.containerLogs(ctx, cd, taskID, tailLines)
		if err != nil {
			slog.Debug("ecs: skip container", "name", aws.ToString(cd.Name), "err", err)
			continue
		}
		results = append(results, logs)
	}
	return results, nil
}

func (s *EcsService) containerLogs(
	ctx context.Context,
	cd ecstypes.ContainerDefinition,
	taskID string,
	tailLines int32,
) (ContainerLogs, error) {
	if cd.Name == nil {
		return ContainerLogs{}, fmt.Errorf("container has no name")
	}
	if cd.LogConfiguration == nil || cd.LogConfiguration.LogDriver != ecstypes.LogDriverAwslogs {
		return ContainerLogs{}, fmt.Errorf("container %q does not use awslogs driver", *cd.Name)
	}

	opts := cd.LogConfiguration.Options
	logGroup := opts["awslogs-group"]
	streamPrefix := opts["awslogs-stream-prefix"]
	if logGroup == "" {
		return ContainerLogs{}, fmt.Errorf("container %q has no awslogs-group", *cd.Name)
	}

	// Standard stream name: <prefix>/<container-name>/<task-id>
	logStream := fmt.Sprintf("%s/%s/%s", streamPrefix, *cd.Name, taskID)

	out, err := s.cwl.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(logStream),
		StartFromHead: aws.Bool(false),
		Limit:         &tailLines,
	})
	if err != nil {
		return ContainerLogs{}, fmt.Errorf("get log events for %q: %w", logStream, err)
	}

	events := make([]CWLEvent, 0, len(out.Events))
	for _, e := range out.Events {
		ev := CWLEvent{Message: aws.ToString(e.Message)}
		if e.Timestamp != nil {
			ev.Timestamp = time.UnixMilli(*e.Timestamp)
		}
		events = append(events, ev)
	}

	return ContainerLogs{
		ContainerName: *cd.Name,
		LogGroup:      logGroup,
		LogStream:     logStream,
		Events:        events,
	}, nil
}

// lastArnSegment returns the resource name from an ARN by splitting on "/" and
// taking the last segment. Works for cluster, service, and task ARNs.
func lastArnSegment(arn string) string {
	parts := strings.Split(arn, "/")
	if len(parts) == 0 {
		return arn
	}
	return parts[len(parts)-1]
}
