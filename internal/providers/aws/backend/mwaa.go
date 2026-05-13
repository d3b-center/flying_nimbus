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
}

func InitMWAAService(cfg aws.Config) *MWAAService {
	slog.Debug("Initializing MWAA Service")
	return &MWAAService{
		api:  mwaa.NewFromConfig(cfg),
		http: &http.Client{Timeout: 30 * time.Second},
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
		ExecutionDate string `json:"execution_date"`
		StartDate     string `json:"start_date"`
		EndDate       string `json:"end_date"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &raw); err != nil {
		return nil, fmt.Errorf("parse dag runs: %w (output: %.300s)", err, output)
	}
	runs := make([]MWAADagRun, len(raw))
	for i, r := range raw {
		runs[i] = MWAADagRun{
			RunId:         r.RunID,
			State:         r.State,
			ExecutionDate: r.ExecutionDate,
			StartDate:     r.StartDate,
			EndDate:       r.EndDate,
		}
	}
	return runs, nil
}

// ListTaskInstances returns the task instances for a specific run.
// It first looks up the execution date from the run ID, then lists tasks.
func (s *MWAAService) ListTaskInstances(ctx context.Context, envName, dagId, runId string) ([]MWAATaskInstance, error) {
	execDate, err := s.execDateForRun(ctx, envName, dagId, runId)
	if err != nil {
		return nil, err
	}

	cliURL, tok, err := s.cliEndpoint(ctx, envName)
	if err != nil {
		return nil, err
	}
	// Airflow 3: positional dag_id + --execution-date; Airflow 2: -d/-e flags.
	output, err := s.cliTryCommands(ctx, cliURL, tok, []string{
		fmt.Sprintf("tasks list %s %s --output json", dagId, execDate),
		fmt.Sprintf("tasks list --dag-id %s --execution-date %s --output json", dagId, execDate),
		fmt.Sprintf("tasks list -d %s -e %s --output json", dagId, execDate),
	})
	if err != nil {
		return nil, fmt.Errorf("list task instances: %w", err)
	}

	var raw []struct {
		TaskID string `json:"task_id"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &raw); err != nil {
		// Fall back: return a single placeholder if parsing fails.
		slog.Debug(fmt.Sprintf("mwaa: tasks list parse failed: %v (output: %.200s)", err, output))
		return []MWAATaskInstance{{TaskId: "(parse error)", State: output[:min(80, len(output))]}}, nil
	}
	tasks := make([]MWAATaskInstance, len(raw))
	for i, t := range raw {
		tasks[i] = MWAATaskInstance{TaskId: t.TaskID, State: t.State}
	}
	return tasks, nil
}

// GetRunLogs fetches the logs for every task in a DAG run.
func (s *MWAAService) GetRunLogs(ctx context.Context, envName, dagId, runId string) (string, error) {
	execDate, err := s.execDateForRun(ctx, envName, dagId, runId)
	if err != nil {
		return "", err
	}

	tasks, err := s.ListTaskInstances(ctx, envName, dagId, runId)
	if err != nil {
		return "", err
	}
	if len(tasks) == 0 {
		return fmt.Sprintf("No task instances found for run %q.", runId), nil
	}

	cliURL, tok, err := s.cliEndpoint(ctx, envName)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Logs for %s / %s / %s:\n", envName, dagId, runId))

	for _, task := range tasks {
		sb.WriteString(fmt.Sprintf("\n  ── task: %s  [%s] ──\n", task.TaskId, task.State))
		logs, err := s.cliTryCommands(ctx, cliURL, tok, []string{
			fmt.Sprintf("tasks logs %s %s %s", dagId, task.TaskId, execDate),
			fmt.Sprintf("tasks logs --dag-id %s --task-id %s --execution-date %s", dagId, task.TaskId, execDate),
			fmt.Sprintf("tasks logs -d %s -t %s -e %s", dagId, task.TaskId, execDate),
		})
		if err != nil {
			sb.WriteString(fmt.Sprintf("  (error fetching logs: %v)\n", err))
			continue
		}
		for _, line := range strings.Split(strings.TrimRight(logs, "\n"), "\n") {
			sb.WriteString("  " + line + "\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n"), nil
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
