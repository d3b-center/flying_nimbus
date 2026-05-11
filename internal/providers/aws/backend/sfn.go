package aws

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	sfntypes "github.com/aws/aws-sdk-go-v2/service/sfn/types"
)

// StateMachine is a simplified view of an AWS Step Functions state machine.
type StateMachine struct {
	Name              string
	Arn               string
	Type              string
	// LastExecutionTime is the start time of the most recent execution.
	// Nil means no executions have been recorded (or could not be fetched).
	LastExecutionTime *time.Time
}

// SfnExecution is a simplified view of a single state machine execution.
type SfnExecution struct {
	Name      string
	Status    string
	StartDate time.Time
	StopDate  *time.Time
}

// SfnLogEvent is a filtered, simplified representation of a single execution
// history event suitable for display in the chat viewport.
type SfnLogEvent struct {
	Timestamp time.Time
	Type      string // human-readable label
	StateName string // non-empty for state enter/exit events
	Error     string // non-empty for failure events
	Cause     string // non-empty for failure events
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

	GetExecutionHistory(
		ctx context.Context,
		params *sfn.GetExecutionHistoryInput,
		optFns ...func(*sfn.Options),
	) (*sfn.GetExecutionHistoryOutput, error)
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

// ListStateMachinesWithActivity fetches all state machines and enriches each
// entry with the start time of its most recent execution, then sorts the list
// by that timestamp descending (most recently active first).
// Machines that have never run appear at the bottom.
func (s *SfnService) ListStateMachinesWithActivity(ctx context.Context) ([]StateMachine, error) {
	machines, err := s.ListStateMachines(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch the most recent execution for each machine.
	// ListExecutions returns results newest-first, so MaxResults=1 gives us
	// what we need without iterating.
	for i, m := range machines {
		out, err := s.api.ListExecutions(ctx, &sfn.ListExecutionsInput{
			StateMachineArn: aws.String(m.Arn),
			MaxResults:      1,
		})
		if err != nil {
			// Non-fatal: a permission error on one machine should not abort the list.
			slog.Debug("sfn: could not get last execution", "arn", m.Arn, "err", err)
			continue
		}
		if len(out.Executions) > 0 && out.Executions[0].StartDate != nil {
			t := *out.Executions[0].StartDate
			machines[i].LastExecutionTime = &t
		}
	}

	// Sort: machines with a recent execution first, newest-to-oldest.
	// Machines with no executions sink to the bottom.
	sort.Slice(machines, func(i, j int) bool {
		ti := machines[i].LastExecutionTime
		tj := machines[j].LastExecutionTime
		if ti == nil && tj == nil {
			return machines[i].Name < machines[j].Name // stable: alphabetical
		}
		if ti == nil {
			return false
		}
		if tj == nil {
			return true
		}
		return ti.After(*tj)
	})

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

// GetExecutionLogs fetches the execution history for a named execution and
// returns it as a filtered list of SfnLogEvent values.
//
// The execution ARN is constructed directly from the state machine ARN and the
// execution name — no ListExecutions call is needed.
//
// Only meaningful events are returned: execution start/end, state transitions,
// failures, and successes. Low-level scheduler/worker events are skipped.
func (s *SfnService) GetExecutionLogs(ctx context.Context, stateMachineArn, executionName string) ([]SfnLogEvent, error) {
	execArn, err := buildExecutionArn(stateMachineArn, executionName)
	if err != nil {
		return nil, err
	}

	var events []SfnLogEvent
	var nextToken *string

	for {
		out, err := s.api.GetExecutionHistory(ctx, &sfn.GetExecutionHistoryInput{
			ExecutionArn:        aws.String(execArn),
			IncludeExecutionData: aws.Bool(false), // skip large I/O blobs
			NextToken:           nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("get execution history: %w", err)
		}

		for _, e := range out.Events {
			if !isMeaningfulEvent(e.Type) {
				continue
			}
			ev := SfnLogEvent{Type: string(e.Type)}
			if e.Timestamp != nil {
				ev.Timestamp = *e.Timestamp
			}
			// Extract state name from entry/exit events.
			if e.StateEnteredEventDetails != nil && e.StateEnteredEventDetails.Name != nil {
				ev.StateName = *e.StateEnteredEventDetails.Name
			}
			if e.StateExitedEventDetails != nil && e.StateExitedEventDetails.Name != nil {
				ev.StateName = *e.StateExitedEventDetails.Name
			}
			// Extract error/cause from various failure event types.
			if e.ExecutionFailedEventDetails != nil {
				ev.Error = aws.ToString(e.ExecutionFailedEventDetails.Error)
				ev.Cause = aws.ToString(e.ExecutionFailedEventDetails.Cause)
			}
			if e.LambdaFunctionFailedEventDetails != nil {
				ev.Error = aws.ToString(e.LambdaFunctionFailedEventDetails.Error)
				ev.Cause = aws.ToString(e.LambdaFunctionFailedEventDetails.Cause)
			}
			if e.TaskFailedEventDetails != nil {
				ev.Error = aws.ToString(e.TaskFailedEventDetails.Error)
				ev.Cause = aws.ToString(e.TaskFailedEventDetails.Cause)
			}
			events = append(events, ev)
		}

		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return events, nil
}

// buildExecutionArn constructs an execution ARN from a state machine ARN and
// execution name without making any API calls.
//
//	stateMachine ARN: arn:aws:states:<region>:<account>:stateMachine:<name>
//	execution ARN:    arn:aws:states:<region>:<account>:execution:<name>:<execName>
func buildExecutionArn(stateMachineArn, executionName string) (string, error) {
	parts := strings.SplitN(stateMachineArn, ":", 8)
	if len(parts) != 7 || parts[5] != "stateMachine" {
		return "", fmt.Errorf("unexpected state machine ARN format: %q", stateMachineArn)
	}
	// Replace "stateMachine" segment and append execution name.
	return strings.Join([]string{
		parts[0], parts[1], parts[2], parts[3], parts[4],
		"execution", parts[6], executionName,
	}, ":"), nil
}

// isMeaningfulEvent reports whether an event type is worth showing to the user.
// Low-level scheduler, worker-start, and worker-heartbeat events are skipped.
func isMeaningfulEvent(t sfntypes.HistoryEventType) bool {
	s := string(t)
	// Always show execution-level transitions.
	if strings.HasPrefix(s, "Execution") {
		return true
	}
	// Show all state entry and exit transitions.
	if strings.HasSuffix(s, "Entered") || strings.HasSuffix(s, "Exited") {
		return true
	}
	// Show all failures, timeouts, and aborts.
	if strings.HasSuffix(s, "Failed") ||
		strings.HasSuffix(s, "TimedOut") ||
		strings.HasSuffix(s, "Aborted") {
		return true
	}
	// Show Lambda / task successes.
	if strings.HasSuffix(s, "Succeeded") {
		return true
	}
	return false
}
