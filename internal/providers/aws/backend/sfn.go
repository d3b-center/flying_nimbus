package aws

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
)

// StateMachine is a simplified view of an AWS Step Functions state machine.
type StateMachine struct {
	Name string
	Arn  string
	Type string
}

// SfnExecution is a simplified view of a single state machine execution.
type SfnExecution struct {
	Name      string
	Status    string
	StartDate time.Time
	StopDate  *time.Time
}

type sfnAPI interface {
	ListStateMachines(
		ctx context.Context,
		params *sfn.ListStateMachinesInput,
		optFns ...func(*sfn.Options),
	) (*sfn.ListStateMachinesOutput, error)

	ListExecutions(
		ctx context.Context,
		params *sfn.ListExecutionsInput,
		optFns ...func(*sfn.Options),
	) (*sfn.ListExecutionsOutput, error)
}

// SfnService provides read-only access to AWS Step Functions metadata.
type SfnService struct {
	api sfnAPI
}

// InitSfnService creates a new SfnService from an AWS config.
func InitSfnService(cfg aws.Config) *SfnService {
	slog.Debug("Initializing Step Functions Service")
	return &SfnService{api: sfn.NewFromConfig(cfg)}
}

// ListStateMachines returns all state machines visible to the caller,
// collecting all pages.
func (s *SfnService) ListStateMachines(ctx context.Context) ([]StateMachine, error) {
	var machines []StateMachine
	var nextToken *string

	for {
		out, err := s.api.ListStateMachines(ctx, &sfn.ListStateMachinesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list state machines: %w", err)
		}

		for _, m := range out.StateMachines {
			if m.Name == nil || m.StateMachineArn == nil {
				continue
			}
			machines = append(machines, StateMachine{
				Name: *m.Name,
				Arn:  *m.StateMachineArn,
				Type: string(m.Type),
			})
		}

		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	slog.Debug(fmt.Sprintf("sfn: found %d state machines", len(machines)))
	return machines, nil
}

// ListExecutions returns the most recent executions for a state machine ARN.
// Results are limited to maxResults to avoid flooding the chat viewport.
func (s *SfnService) ListExecutions(ctx context.Context, stateMachineArn string, maxResults int32) ([]SfnExecution, error) {
	out, err := s.api.ListExecutions(ctx, &sfn.ListExecutionsInput{
		StateMachineArn: aws.String(stateMachineArn),
		MaxResults:      maxResults,
	})
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}

	execs := make([]SfnExecution, 0, len(out.Executions))
	for _, e := range out.Executions {
		if e.Name == nil {
			continue
		}
		ex := SfnExecution{
			Name:   *e.Name,
			Status: string(e.Status),
		}
		if e.StartDate != nil {
			ex.StartDate = *e.StartDate
		}
		ex.StopDate = e.StopDate
		execs = append(execs, ex)
	}
	return execs, nil
}

// FindStateMachineByName searches for a state machine by exact name match,
// then case-insensitive match, then partial match.
func (s *SfnService) FindStateMachineByName(ctx context.Context, name string) (StateMachine, error) {
	machines, err := s.ListStateMachines(ctx)
	if err != nil {
		return StateMachine{}, err
	}
	return findStateMachineInList(machines, name)
}

func findStateMachineInList(machines []StateMachine, search string) (StateMachine, error) {
	searchLower := strings.ToLower(search)
	for _, m := range machines {
		if m.Name == search {
			return m, nil
		}
	}
	for _, m := range machines {
		if strings.EqualFold(m.Name, search) {
			return m, nil
		}
	}
	for _, m := range machines {
		if strings.Contains(strings.ToLower(m.Name), searchLower) {
			return m, nil
		}
	}
	return StateMachine{}, fmt.Errorf("no state machine found matching %q", search)
}
