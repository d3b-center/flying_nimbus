package aws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/mwaa"
)

// ── Public types ──────────────────────────────────────────────────────────────

type MWAAEnvironment struct{ Name string }

type MWAADag struct {
	DagId    string
	IsPaused bool
}

type MWAADagRun struct {
	RunId         string
	State         string
	ExecutionDate string
	StartDate     string
	EndDate       string
}

type MWAATaskInstance struct {
	TaskId    string
	State     string
	StartDate string
	EndDate   string
	TryNumber int
}

// ── Interface ─────────────────────────────────────────────────────────────────

type mwaaAPI interface {
	ListEnvironments(
		ctx context.Context,
		params *mwaa.ListEnvironmentsInput,
		optFns ...func(*mwaa.Options),
	) (*mwaa.ListEnvironmentsOutput, error)

	CreateCliToken(
		ctx context.Context,
		params *mwaa.CreateCliTokenInput,
		optFns ...func(*mwaa.Options),
	) (*mwaa.CreateCliTokenOutput, error)
}

// ── Service ───────────────────────────────────────────────────────────────────

// MWAAService provides access to Amazon Managed Workflows for Apache Airflow.
//
// Environment listing uses the AWS SDK. All DAG/run/log operations use the
// MWAA CLI API endpoint (/aws_mwaa/cli) with a CreateCliToken bearer token —
// this is the documented programmatic access method for MWAA and works for
// both Airflow 2.x and 3.x.
type MWAAService struct {
	api  mwaaAPI
	http *http.Client
	cwl  cwlAPI
}

type cwlAPI interface {
	GetLogEvents(
		ctx context.Context,
		params *cloudwatchlogs.GetLogEventsInput,
		optFns ...func(*cloudwatchlogs.Options),
	) (*cloudwatchlogs.GetLogEventsOutput, error)
}

func InitMWAAService(cfg aws.Config) *MWAAService {
	slog.Debug("Initializing MWAA Service")
	return &MWAAService{
		api:  mwaa.NewFromConfig(cfg),
		http: &http.Client{Timeout: 30 * time.Second},
		cwl:  cloudwatchlogs.NewFromConfig(cfg),
	}
}

// ListEnvironments returns all MWAA environment names.
func (s *MWAAService) ListEnvironments(ctx context.Context) ([]MWAAEnvironment, error) {
	var envs []MWAAEnvironment
	var nextToken *string
	for {
		out, err := s.api.ListEnvironments(ctx, &mwaa.ListEnvironmentsInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list MWAA environments: %w", err)
		}
		for _, name := range out.Environments {
			envs = append(envs, MWAAEnvironment{Name: name})
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	slog.Debug(fmt.Sprintf("mwaa: found %d environments", len(envs)))
	return envs, nil
}

// ── CLI API helpers ───────────────────────────────────────────────────────────

// cliEndpoint returns the CLI API URL and a fresh CLI token for the given
// environment. CreateCliToken produces a bearer token accepted by the
// /aws_mwaa/cli proxy endpoint for both Airflow 2.x and 3.x.
func (s *MWAAService) cliEndpoint(ctx context.Context, envName string) (url, token string, err error) {
	out, err := s.api.CreateCliToken(ctx, &mwaa.CreateCliTokenInput{
		Name: aws.String(envName),
	})
	if err != nil {
		return "", "", fmt.Errorf("create CLI token for %q: %w", envName, err)
	}
	hostname := aws.ToString(out.WebServerHostname)
	if !strings.HasPrefix(hostname, "https://") {
		hostname = "https://" + hostname
	}
	return hostname + "/aws_mwaa/cli", aws.ToString(out.CliToken), nil
}

// cliRun posts an Airflow CLI command and returns the decoded stdout.
// The /aws_mwaa/cli endpoint wraps every CLI invocation in a JSON envelope
// with base64-encoded stdout, stderr, and a returncode.
func (s *MWAAService) cliRun(ctx context.Context, cliURL, token, command string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cliURL,
		strings.NewReader(command))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("MWAA CLI POST: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("MWAA CLI returned %d: %s", resp.StatusCode, string(body))
	}

	var envelope struct {
		Stdout     string `json:"stdout"`
		Stderr     string `json:"stderr"`
		ReturnCode int    `json:"returncode"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", fmt.Errorf("parse CLI envelope: %w (body: %.200s)", err, string(body))
	}

	decoded, err := base64.StdEncoding.DecodeString(envelope.Stdout)
	if err != nil {
		return "", fmt.Errorf("decode CLI stdout: %w", err)
	}
	return string(decoded), nil
}

// ── DAG / run / task operations ───────────────────────────────────────────────

// ListDags returns all DAGs in the given MWAA environment.
func (s *MWAAService) ListDags(ctx context.Context, envName string) ([]MWAADag, error) {
	url, tok, err := s.cliEndpoint(ctx, envName)
	if err != nil {
		return nil, err
	}
	output, err := s.cliRun(ctx, url, tok, "dags list --output json")
	if err != nil {
		return nil, fmt.Errorf("list DAGs: %w", err)
	}

	// Airflow CLI JSON: [{dag_id, paused/is_paused, ...}]
	// is_paused is returned as a Python-style string ("True"/"False"),
	// not a JSON boolean, so we parse it as string and convert.
	var raw []struct {
		DagID    string `json:"dag_id"`
		Paused   string `json:"paused"`
		IsPaused string `json:"is_paused"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &raw); err != nil {
		return nil, fmt.Errorf("parse dags: %w (output: %.300s)", err, output)
	}
	dags := make([]MWAADag, len(raw))
	for i, d := range raw {
		paused := strings.EqualFold(d.Paused, "true") || strings.EqualFold(d.IsPaused, "true")
		dags[i] = MWAADag{DagId: d.DagID, IsPaused: paused}
	}
	return dags, nil
}

// cliTryCommands executes the first command in the list that does not return
// a 400 (argument parsing) error. This lets us support both Airflow 2.x
// (flag-based) and Airflow 3.x (positional / renamed flags) without knowing
// the version in advance.
func (s *MWAAService) cliTryCommands(ctx context.Context, cliURL, token string, cmds []string) (string, error) {
	var lastErr error
	for _, cmd := range cmds {
		out, err := s.cliRun(ctx, cliURL, token, cmd)
		if err == nil {
			return out, nil
		}
		lastErr = err
		// Only retry on argument-parsing errors; propagate auth / network errors.
		if !strings.Contains(err.Error(), "400") {
			return "", err
		}
	}
	return "", lastErr
}

// ListDagRuns returns the most recent runs for the given DAG.
func (s *MWAAService) ListDagRuns(ctx context.Context, envName, dagId string) ([]MWAADagRun, error) {
	url, tok, err := s.cliEndpoint(ctx, envName)
	if err != nil {
		return nil, err
	}
	// Airflow 3 made dag_id positional; Airflow 2 used -d / --dag-id.
	output, err := s.cliTryCommands(ctx, url, tok, []string{
		fmt.Sprintf("dags list-runs %s --output json", dagId),
		fmt.Sprintf("dags list-runs --dag-id %s --output json", dagId),
		fmt.Sprintf("dags list-runs -d %s --output json", dagId),
	})
	if err != nil {
		return nil, fmt.Errorf("list DAG runs: %w", err)
	}

	var raw []struct {
		RunID         string `json:"run_id"`
		State         string `json:"state"`
		ExecutionDate string `json:"execution_date"` // Airflow 2.x
		LogicalDate   string `json:"logical_date"`   // Airflow 3.x (renamed)
		StartDate     string `json:"start_date"`
		EndDate       string `json:"end_date"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &raw); err != nil {
		return nil, fmt.Errorf("parse dag runs: %w (output: %.300s)", err, output)
	}
	runs := make([]MWAADagRun, len(raw))
	for i, r := range raw {
		// Prefer logical_date (Airflow 3) but fall back to execution_date (Airflow 2).
		execDate := r.LogicalDate
		if execDate == "" {
			execDate = r.ExecutionDate
		}
		runs[i] = MWAADagRun{
			RunId:         r.RunID,
			State:         r.State,
			ExecutionDate: execDate,
			StartDate:     r.StartDate,
			EndDate:       r.EndDate,
		}
	}
	return runs, nil
}

// listTaskIds returns the task IDs defined in a DAG (not run-specific).
// In Airflow 3, tasks list takes only the dag_id without an execution date.
func (s *MWAAService) listTaskIds(ctx context.Context, cliURL, tok, dagId string) ([]string, error) {
	output, err := s.cliTryCommands(ctx, cliURL, tok, []string{
		fmt.Sprintf("tasks list %s --output json", dagId),
		fmt.Sprintf("tasks list --dag-id %s --output json", dagId),
		fmt.Sprintf("tasks list -d %s --output json", dagId),
		// Airflow 3 might output task IDs as a plain list without --output json.
		fmt.Sprintf("tasks list %s", dagId),
	})
	if err != nil {
		return nil, err
	}

	// Try parsing as [{task_id: ...}] (JSON array of objects).
	var objList []struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &objList); err == nil && len(objList) > 0 {
		ids := make([]string, len(objList))
		for i, t := range objList {
			ids[i] = t.TaskID
		}
		return ids, nil
	}

	// Try parsing as ["task_id1", "task_id2"] (JSON array of strings).
	var strList []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &strList); err == nil {
		return strList, nil
	}

	// Fall back: parse plain-text output (one task_id per line, skip header/separators).
	var ids []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "=") || strings.HasPrefix(line, "-") ||
			strings.HasPrefix(line, "task_id") {
			continue
		}
		// Strip table borders and pick the first field.
		if strings.Contains(line, "|") {
			parts := strings.SplitN(line, "|", 2)
			line = strings.TrimSpace(parts[0])
		}
		if line != "" {
			ids = append(ids, line)
		}
	}
	return ids, nil
}

// ListTaskInstances returns task names defined in the DAG, with state
// where available. In Airflow 3 the CLI no longer exposes per-run task
// state; we retrieve the task definitions and show the run's overall state.
func (s *MWAAService) ListTaskInstances(ctx context.Context, envName, dagId, runId string) ([]MWAATaskInstance, error) {
	cliURL, tok, err := s.cliEndpoint(ctx, envName)
	if err != nil {
		return nil, err
	}
	ids, err := s.listTaskIds(ctx, cliURL, tok, dagId)
	if err != nil {
		return nil, fmt.Errorf("list task instances: %w", err)
	}
	tasks := make([]MWAATaskInstance, len(ids))
	for i, id := range ids {
		tasks[i] = MWAATaskInstance{TaskId: id}
	}
	return tasks, nil
}

// GetRunLogs fetches logs or state for every task in a DAG run.
// It tries `tasks logs` in multiple forms; if all fail it falls back to
// `tasks state` and shows the CloudWatch Logs path where MWAA writes task logs.
func (s *MWAAService) GetRunLogs(ctx context.Context, envName, dagId, runId string) (string, error) {
	execDate, err := s.execDateForRun(ctx, envName, dagId, runId)
	if err != nil {
		// For manually triggered runs the run_id contains the timestamp.
		execDate = runId
	}
	execDateZ := strings.Replace(execDate, "+00:00", "Z", 1)

	cliURL, tok, err := s.cliEndpoint(ctx, envName)
	if err != nil {
		return "", err
	}

	taskIds, err := s.listTaskIds(ctx, cliURL, tok, dagId)
	if err != nil || len(taskIds) == 0 {
		return fmt.Sprintf("Could not retrieve task list for DAG %q: %v", dagId, err), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Logs for %s / %s / %s:\n", envName, dagId, runId))

	for _, taskId := range taskIds {
		sb.WriteString(fmt.Sprintf("\n  ── task: %s ──\n", taskId))

		// 1. Try every known form of `tasks logs`.
		logs, logErr := s.cliTryCommands(ctx, cliURL, tok, []string{
			fmt.Sprintf("tasks logs %s %s %s", dagId, runId, taskId),
			fmt.Sprintf("tasks logs --dag-id %s --run-id %s --task-id %s", dagId, runId, taskId),
			fmt.Sprintf("tasks logs --dag-id %s --task-id %s --logical-date %s", dagId, taskId, execDateZ),
			fmt.Sprintf("tasks logs %s %s %s", dagId, taskId, execDateZ),
			fmt.Sprintf("tasks logs %s %s %s", dagId, taskId, execDate),
			fmt.Sprintf("tasks logs --dag-id %s --task-id %s --execution-date %s", dagId, taskId, execDate),
			fmt.Sprintf("tasks logs -d %s -t %s -e %s", dagId, taskId, execDate),
		})
		if logErr == nil {
			for _, line := range strings.Split(strings.TrimRight(logs, "\n"), "\n") {
				sb.WriteString("  " + line + "\n")
			}
			continue
		}

		// 2. Fall back to `tasks state` to at least show success/failure status.
		state, stateErr := s.cliTryCommands(ctx, cliURL, tok, []string{
			fmt.Sprintf("tasks state %s %s %s", dagId, taskId, runId),
			fmt.Sprintf("tasks state %s %s %s", dagId, taskId, execDateZ),
			fmt.Sprintf("tasks state %s %s %s", dagId, taskId, execDate),
			fmt.Sprintf("tasks state -d %s -t %s -e %s", dagId, taskId, execDate),
		})
		if stateErr == nil {
			sb.WriteString(fmt.Sprintf("  State: %s\n", strings.TrimSpace(state)))
		}

		// 3. Fetch task logs directly from CloudWatch Logs.
		logGroup := fmt.Sprintf("airflow-%s-Task", envName)
		logStream := fmt.Sprintf("dag_id=%s/run_id=%s/task_id=%s/attempt=1.log", dagId, runId, taskId)
		cwLogs, cwErr := s.fetchCWLogs(ctx, logGroup, logStream, 200)
		if cwErr == nil && strings.TrimSpace(cwLogs) != "" {
			for _, line := range strings.Split(strings.TrimRight(cwLogs, "\n"), "\n") {
				sb.WriteString("  " + line + "\n")
			}
		} else {
			// Show the path so the user can look manually.
			sb.WriteString(fmt.Sprintf(
				"  CloudWatch group:  %s\n  CloudWatch stream:\n  %s\n",
				logGroup, logStream))
			if cwErr != nil {
				sb.WriteString(fmt.Sprintf("  (CW fetch error: %v)\n", cwErr))
			}
		}
	}

	return strings.TrimRight(sb.String(), "\n"), nil
}

// fetchCWLogs retrieves the most recent log events from a CloudWatch Logs
// stream. MWAA writes task logs as JSON objects with an "event" field
// containing the actual log line; this helper extracts that field when present.
func (s *MWAAService) fetchCWLogs(ctx context.Context, logGroup, logStream string, limit int32) (string, error) {
	out, err := s.cwl.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(logStream),
		StartFromHead: aws.Bool(false), // newest events first
		Limit:         &limit,
	})
	if err != nil {
		return "", fmt.Errorf("CloudWatch GetLogEvents: %w", err)
	}

	var sb strings.Builder
	for _, e := range out.Events {
		if e.Message == nil {
			continue
		}
		msg := *e.Message
		// MWAA emits JSON: {"logger":"...","event":"actual text","level":"info"}
		var entry struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal([]byte(msg), &entry); err == nil && entry.Event != "" {
			sb.WriteString(entry.Event + "\n")
		} else {
			sb.WriteString(msg + "\n")
		}
	}
	return sb.String(), nil
}

// execDateForRun looks up the execution date for a given run ID.
func (s *MWAAService) execDateForRun(ctx context.Context, envName, dagId, runId string) (string, error) {
	runs, err := s.ListDagRuns(ctx, envName, dagId)
	if err != nil {
		return "", err
	}
	for _, r := range runs {
		if r.RunId == runId {
			return r.ExecutionDate, nil
		}
	}
	return "", fmt.Errorf("run %q not found in DAG %q", runId, dagId)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
