package views

import (
	"context"
	"fmt"
	"strings"

	"flying_nimbus/internal/app"
	aws "flying_nimbus/internal/providers/aws/backend"
	c "flying_nimbus/internal/providers/aws/views/components"

	tea "github.com/charmbracelet/bubbletea"
)

// ── Command registry ──────────────────────────────────────────────────────────

// allHashCommands is the canonical list used for autocomplete and help.
// Add a new entry here when implementing a new # command.
var allHashCommands = []string{
	"#shell",
	"#tunnel",
	"#ls",
	"#logs",
	"#help",
}

// ── Resource-reference expansion ─────────────────────────────────────────────

// expandResourceRefs scans text for embedded #ls resource references and
// replaces each with a human-readable description the DevOps Agent can act on.
//
//	"why is #ls ecs prod service api task abc123 slow?"
//	→  "why is ECS task "abc123" in service "api" on cluster "prod" slow?"
func expandResourceRefs(text string) string {
	if !strings.Contains(strings.ToLower(text), "#ls ") {
		return text
	}
	tokens := strings.Fields(text)
	result := make([]string, 0, len(tokens))
	i := 0
	for i < len(tokens) {
		if strings.EqualFold(tokens[i], "#ls") && i+1 < len(tokens) {
			rest := tokens[i+1:]
			lowerRest := make([]string, len(rest))
			for j, t := range rest {
				lowerRest[j] = strings.ToLower(t)
			}
			if expansion, consumed := expandRefTokens(lowerRest, rest); consumed > 0 {
				result = append(result, expansion)
				i += 1 + consumed
				continue
			}
		}
		result = append(result, tokens[i])
		i++
	}
	return strings.Join(result, " ")
}

// expandRefTokens parses the token slice that follows "#ls" and returns the
// human-readable expansion plus the number of tokens consumed.
func expandRefTokens(lower, orig []string) (string, int) {
	if len(lower) == 0 {
		return "", 0
	}
	switch lower[0] {
	case "ec2":
		if len(lower) >= 2 {
			return fmt.Sprintf("EC2 instance %q", orig[1]), 2
		}
	case "rds":
		if len(lower) >= 2 {
			return fmt.Sprintf("RDS instance %q", orig[1]), 2
		}
	case "sfn":
		if len(lower) < 2 {
			return "", 0
		}
		for i := 2; i < len(lower)-1; i++ {
			if lower[i] == "job" {
				return fmt.Sprintf("Step Function execution %q on %q",
					orig[i+1], strings.Join(orig[1:i], " ")), i + 2
			}
		}
		return fmt.Sprintf("Step Function %q", orig[1]), 2
	case "emr":
		if len(lower) >= 2 {
			return fmt.Sprintf("EMR cluster %q", orig[1]), 2
		}
	case "ecs":
		if len(lower) < 2 {
			return "", 0
		}
		for i := 2; i < len(lower)-1; i++ {
			if lower[i] == "service" {
				service := orig[i+1]
				for j := i + 2; j < len(lower)-1; j++ {
					if lower[j] == "task" {
						return fmt.Sprintf("ECS task %q in service %q on cluster %q",
							orig[j+1], service, orig[1]), j + 2
					}
				}
				return fmt.Sprintf("ECS service %q in cluster %q", service, orig[1]), i + 2
			}
		}
		return fmt.Sprintf("ECS cluster %q", orig[1]), 2
	}
	return "", 0
}

// lsPartFrom returns the portion of the input from the last "#ls " onwards
// (lowercased). Used to extract the #ls context from mid-sentence inputs.
func lsPartFrom(lower string) string {
	if idx := strings.LastIndex(lower, "#ls "); idx >= 0 {
		return lower[idx:]
	}
	return ""
}

// lsSubCommands are the valid resource types accepted by #ls.
var lsSubCommands = []string{
	"ec2",
	"rds",
	"emr",
	"sfn",
	"ecs",
	"mwaa",
}

// logsSubCommands are the valid resource types accepted by #logs.
// Extend this slice when support for additional log sources is added.
var logsSubCommands = []string{
	"sfn",
	"ecs",
	"mwaa",
}

// computeSuggestions returns commands (or sub-resources / keywords / resource
// names) that match the current input prefix.
//
// It is a method so it can read the model's resource-name caches.
func (m *DevOpsAgentViewModel) computeSuggestions(input string) []string {
	if input == "" {
		return nil
	}
	lower := strings.ToLower(input)

	// #shell — suggest running EC2 instance names from cache.
	if strings.HasPrefix(lower, "#shell ") {
		return resourceNameSuggestions(lower, "#shell ", m.cachedEC2Names)
	}

	// #ls suggestions — work whether #ls is at the start (command mode) or
	// embedded mid-sentence (resource-reference mode).  We find the last
	// occurrence so the user can type prose before the reference.
	if lsPart := lsPartFrom(lower); lsPart != "" {
		prefix := input[:len(input)-len(lsPart)] // original-cased text before #ls
		lsSuggs := m.computeLsSuggestions(lsPart)
		if len(lsSuggs) > 0 {
			full := make([]string, len(lsSuggs))
			for i, s := range lsSuggs {
				full[i] = prefix + s
			}
			return full
		}
	}

	// #logs mode (only at the start of the input).
	if strings.HasPrefix(lower, "#logs ") {
		if strings.HasPrefix(lower, "#logs sfn ") {
			return logsJobSuggestion(lower)
		}
		if strings.HasPrefix(lower, "#logs ecs ") {
			return ecsLogsSuggestions(lower, m.cachedECSClusters, m.cachedECSServices, m.cachedECSTasks)
		}
		if strings.HasPrefix(lower, "#logs mwaa ") {
			return mwaaLogsSuggestions(lower, m.cachedMWAAEnvNames, m.cachedMWAADags, m.cachedMWAARuns)
		}
		typed := lower[len("#logs "):]
		var matches []string
		for _, sub := range logsSubCommands {
			if strings.HasPrefix(sub, typed) && sub != typed {
				matches = append(matches, "#logs "+sub)
			}
		}
		return matches
	}

	// Regular # command mode — spaces signal argument entry, stop suggesting.
	if strings.Contains(input, " ") {
		return nil
	}
	var matches []string
	for _, cmd := range allHashCommands {
		if strings.HasPrefix(cmd, lower) && cmd != lower {
			matches = append(matches, cmd)
		}
	}
	return matches
}

// resourceNameSuggestions returns full command strings filtered by the
// partially-typed resource name.  prefix is e.g. "#ls sfn " and names is the
// cached list (may be nil if not yet fetched).
func resourceNameSuggestions(lower, prefix string, names []string) []string {
	typed := lower[len(prefix):]
	// Stop suggesting once a space is present — user is typing a keyword.
	if strings.Contains(typed, " ") {
		return nil
	}
	var matches []string
	for _, name := range names {
		nameLower := strings.ToLower(name)
		if strings.HasPrefix(nameLower, typed) && nameLower != typed {
			matches = append(matches, prefix+name)
		}
	}
	return matches
}

// computeLsSuggestions returns suggestions for a string that starts with "#ls ".
// It is called both for standalone commands and for mid-sentence references.
func (m *DevOpsAgentViewModel) computeLsSuggestions(lower string) []string {
	if !strings.HasPrefix(lower, "#ls ") {
		return nil
	}
	if strings.HasPrefix(lower, "#ls sfn ") {
		if s := lsJobsSuggestion(lower, "#ls sfn "); s != nil {
			return s
		}
		return resourceNameSuggestions(lower, "#ls sfn ", m.cachedSFNNames)
	}
	if strings.HasPrefix(lower, "#ls ec2 ") {
		return resourceNameSuggestions(lower, "#ls ec2 ", m.cachedEC2Names)
	}
	if strings.HasPrefix(lower, "#ls rds ") {
		return resourceNameSuggestions(lower, "#ls rds ", m.cachedRDSNames)
	}
	if strings.HasPrefix(lower, "#ls emr ") {
		return resourceNameSuggestions(lower, "#ls emr ", m.cachedEMRNames)
	}
	if strings.HasPrefix(lower, "#ls ecs ") {
		return ecsLsSuggestions(lower, m.cachedECSClusters, m.cachedECSServices, m.cachedECSTasks)
	}
	if strings.HasPrefix(lower, "#ls mwaa ") {
		return mwaaLsSuggestions(lower, m.cachedMWAAEnvNames, m.cachedMWAADags, m.cachedMWAARuns)
	}
	// Suggest sub-resource types.
	typed := lower[len("#ls "):]
	// Stop after a space in the typed part — user is no longer on the type word.
	if strings.Contains(typed, " ") {
		return nil
	}
	var matches []string
	for _, sub := range lsSubCommands {
		if strings.HasPrefix(sub, typed) && sub != typed {
			matches = append(matches, "#ls "+sub)
		}
	}
	return matches
}

// lsJobsSuggestion returns a ["<prefix><name> job"] suggestion when the user
// appears to be at the point of typing the "job" keyword after a resource
// name. prefix is the fixed command prefix, e.g. "#ls sfn ".
func lsJobsSuggestion(lower, prefix string) []string {
	rest := lower[len(prefix):]

	// Already typed "job" at the end — suppress.
	if strings.Contains(rest, " job ") || strings.HasSuffix(rest, " job") {
		return nil
	}

	lastSpace := strings.LastIndex(rest, " ")
	if lastSpace < 0 {
		return nil // still typing the first word of the resource name
	}

	partial := rest[lastSpace+1:]
	if !strings.HasPrefix("job", partial) || partial == "job" {
		return nil
	}

	base := lower[:len(prefix)+lastSpace+1]
	return []string{base + "job"}
}

// logsJobSuggestion returns a ["#logs sfn <name> job"] suggestion when the
// user appears to be at the point of typing the "job" keyword.
// lower must already be lowercased and start with "#logs sfn ".
//
// Detection: the user has typed at least one space after the SFN name portion
// (i.e. the SFN name is at least one word long and they've moved to the next
// word), and the partial word they're typing so far is a prefix of "job".
func logsJobSuggestion(lower string) []string {
	rest := lower[len("#logs sfn "):]

	// Already past the "job" keyword — suppress.
	if strings.Contains(rest, " job ") || strings.HasSuffix(rest, " job") {
		return nil
	}

	// Find the last space: everything before it is the SFN name, everything
	// after it is the partial keyword being typed.
	lastSpace := strings.LastIndex(rest, " ")
	if lastSpace < 0 {
		// No space yet — still typing the first word of the SFN name.
		return nil
	}

	partial := rest[lastSpace+1:]

	// Partial must be a non-complete prefix of "job".
	if !strings.HasPrefix("job", partial) || partial == "job" {
		return nil
	}

	// Build the full suggestion: base + "job".
	base := lower[:len("#logs sfn ")+lastSpace+1]
	return []string{base + "job"}
}

// ecsLsSuggestions returns suggestions for #ls ecs <...> depending on how
// far the user has typed:
//   - cluster not yet complete → suggest cluster names
//   - cluster done + space → suggest "service" keyword
//   - cluster + service keyword + partial service name → suggest service names
// ecsLsSuggestions returns suggestions for #ls ecs <...> handling the full
// three-level chain: cluster → "service" keyword → service name → "task"
// keyword → task ID.
func ecsLsSuggestions(lower string, clusters []string, services, tasks map[string][]string) []string {
	rest := lower[len("#ls ecs "):]

	svcIdx := strings.Index(rest, " service ")
	if svcIdx >= 0 {
		cluster := rest[:svcIdx]
		afterSvc := rest[svcIdx+len(" service "):]

		taskIdx := strings.Index(afterSvc, " task ")
		if taskIdx >= 0 {
			// Past "task " → suggest running task IDs from cache.
			svcName := afterSvc[:taskIdx]
			cacheKey := cluster + "/" + svcName
			prefix := lower[:len("#ls ecs ")+svcIdx+len(" service ")+taskIdx+len(" task ")]
			return resourceNameSuggestions(lower, prefix, tasks[cacheKey])
		}

		// Past "service " → suggest "task" keyword or service names.
		svcPrefix := lower[:len("#ls ecs ")+svcIdx+len(" service ")]
		if s := keywordAfterName(lower, svcPrefix, "task"); s != nil {
			return s
		}
		return resourceNameSuggestions(lower, svcPrefix, services[cluster])
	}

	// "service" not yet complete — suggest keyword or cluster names.
	if s := keywordAfterName(lower, "#ls ecs ", "service"); s != nil {
		return s
	}
	return resourceNameSuggestions(lower, "#ls ecs ", clusters)
}

// ecsLogsSuggestions returns suggestions for #logs ecs <...> handling the
// three-level chain: cluster → "service" → service name → "task" → task ID.
func ecsLogsSuggestions(lower string, clusters []string, services, tasks map[string][]string) []string {
	rest := lower[len("#logs ecs "):]

	svcIdx := strings.Index(rest, " service ")
	if svcIdx < 0 {
		// Not yet past "service" — suggest service keyword or cluster names.
		if s := keywordAfterName(lower, "#logs ecs ", "service"); s != nil {
			return s
		}
		return resourceNameSuggestions(lower, "#logs ecs ", clusters)
	}

	cluster := rest[:svcIdx]
	afterSvc := rest[svcIdx+len(" service "):]

	taskIdx := strings.Index(afterSvc, " task ")
	if taskIdx < 0 {
		// Not yet past "task" — suggest task keyword or service names.
		svcKey := cluster
		svcNames := services[svcKey]
		prefix := lower[:len("#logs ecs ")+svcIdx+len(" service ")]
		if s := keywordAfterName(lower, prefix, "task"); s != nil {
			return s
		}
		return resourceNameSuggestions(lower, prefix, svcNames)
	}

	// Past "task" — suggest task IDs from cache.
	svcName := afterSvc[:taskIdx]
	cacheKey := cluster + "/" + svcName
	taskIDs := tasks[cacheKey]
	prefix := lower[:len("#logs ecs ")+svcIdx+len(" service ")+taskIdx+len(" task ")]
	return resourceNameSuggestions(lower, prefix, taskIDs)
}

// mwaaLsSuggestions handles #ls mwaa <env> [job [<dag> [run [<run>]]]] suggestions.
// The chain mirrors the SFN/ECS pattern: env → "job" keyword → dag name → "run" keyword → run ID.
func mwaaLsSuggestions(lower string, envNames []string, dags, runs map[string][]string) []string {
	rest := lower[len("#ls mwaa "):]

	// Level 3: past "job <dag> run " → suggest run IDs.
	if runIdx := strings.Index(rest, " run "); runIdx >= 0 {
		jobIdx := strings.Index(rest, " job ")
		if jobIdx >= 0 && jobIdx < runIdx {
			envName := rest[:jobIdx]
			dagName := rest[jobIdx+len(" job "):runIdx]
			cacheKey := envName + "/" + dagName
			prefix := lower[:len("#ls mwaa ")+runIdx+len(" run ")]
			return resourceNameSuggestions(lower, prefix, runs[cacheKey])
		}
	}

	// Level 2: past "job " → suggest dag names or "run" keyword after dag.
	if jobIdx := strings.Index(rest, " job "); jobIdx >= 0 {
		envName := rest[:jobIdx]
		dagPrefix := lower[:len("#ls mwaa ")+jobIdx+len(" job ")]
		// Suggest "run" keyword once dag name is complete.
		if s := keywordAfterName(lower, dagPrefix, "run"); s != nil {
			return s
		}
		return resourceNameSuggestions(lower, dagPrefix, dags[envName])
	}

	// Level 1: past env name → suggest "job" keyword.
	if s := keywordAfterName(lower, "#ls mwaa ", "job"); s != nil {
		return s
	}
	_ = rest // avoid unused warning
	return resourceNameSuggestions(lower, "#ls mwaa ", envNames)
}

// mwaaLogsSuggestions handles #logs mwaa <env> job <dag> run <run> suggestions.
func mwaaLogsSuggestions(lower string, envNames []string, dags, runs map[string][]string) []string {
	rest := lower[len("#logs mwaa "):]

	// Level 3: past "job <dag> run " → suggest run IDs.
	if runIdx := strings.Index(rest, " run "); runIdx >= 0 {
		jobIdx := strings.Index(rest, " job ")
		if jobIdx >= 0 && jobIdx < runIdx {
			envName := rest[:jobIdx]
			dagName := rest[jobIdx+len(" job "):runIdx]
			cacheKey := envName + "/" + dagName
			prefix := lower[:len("#logs mwaa ")+runIdx+len(" run ")]
			return resourceNameSuggestions(lower, prefix, runs[cacheKey])
		}
	}

	// Level 2: past "job " → suggest dag names or "run" keyword.
	if jobIdx := strings.Index(rest, " job "); jobIdx >= 0 {
		envName := rest[:jobIdx]
		dagPrefix := lower[:len("#logs mwaa ")+jobIdx+len(" job ")]
		if s := keywordAfterName(lower, dagPrefix, "run"); s != nil {
			return s
		}
		return resourceNameSuggestions(lower, dagPrefix, dags[envName])
	}

	// Level 1: suggest "job" keyword or env names.
	if s := keywordAfterName(lower, "#logs mwaa ", "job"); s != nil {
		return s
	}
	return resourceNameSuggestions(lower, "#logs mwaa ", envNames)
}

// keywordAfterName returns a ["<base><keyword>"] suggestion when the user has
// typed at least one word after the fixed prefix and the last partial word is
// a non-complete prefix of keyword.
func keywordAfterName(lower, prefix, keyword string) []string {
	rest := lower[len(prefix):]
	lastSpace := strings.LastIndex(rest, " ")
	if lastSpace < 0 {
		return nil // still on first word (the resource name)
	}
	partial := rest[lastSpace+1:]
	if !strings.HasPrefix(keyword, partial) || partial == keyword {
		return nil
	}
	base := lower[:len(lower)-len(partial)]
	return []string{base + keyword}
}

// refreshSuggestions keeps both the separator-bar suggestions slice and the
// textinput's built-in ghost-text/Tab suggestions in sync with the current
// input value. Call this after every keystroke that may change the input.
func (m *DevOpsAgentViewModel) refreshSuggestions() {
	val := m.input.Value()
	m.suggestions = m.computeSuggestions(val)

	// For the textinput's built-in ghost-text we need to pass the full
	// candidate set — computeSuggestions already filters, so use it directly.
	// When there are no suggestions yet (e.g. still typing a top-level command)
	// fall back to allHashCommands so Tab completes command names.
	if len(m.suggestions) > 0 {
		m.input.SetSuggestions(m.suggestions)
	} else {
		m.input.SetSuggestions(allHashCommands)
	}
}

// ── Background resource-name fetch commands ───────────────────────────────────

// fetchResourceNamesCmd returns a tea.Cmd that fetches resource names for the
// given kind ("ec2", "rds", "sfn", "emr") and sends back a resourceNamesMsg.
func fetchResourceNamesCmd(a *app.App, kind string) tea.Cmd {
	return func() tea.Msg {
		if a == nil || a.AWS == nil {
			return resourceNamesMsg{kind: kind, names: []string{}}
		}
		ctx := a.Context
		var names []string

		switch kind {
		case "ec2":
			instances, err := a.AWS.Ec2.ListInstances(ctx)
			if err != nil {
				return resourceNamesMsg{kind: kind, names: []string{}}
			}
			for _, i := range instances {
				if i.Name != "" {
					names = append(names, i.Name)
				} else {
					names = append(names, i.InstanceID)
				}
			}
		case "rds":
			instances, err := a.AWS.Rds.ListDBInstances(ctx)
			if err != nil {
				return resourceNamesMsg{kind: kind, names: []string{}}
			}
			for _, i := range instances {
				names = append(names, i.Id)
			}
		case "sfn":
			machines, err := a.AWS.Sfn.ListStateMachines(ctx)
			if err != nil {
				return resourceNamesMsg{kind: kind, names: []string{}}
			}
			for _, m := range machines {
				names = append(names, m.Name)
			}
		case "emr":
			clusters, err := a.AWS.Emr.ListClusters(ctx)
			if err != nil {
				return resourceNamesMsg{kind: kind, names: []string{}}
			}
			for _, c := range clusters {
				names = append(names, c.Name)
			}
		case "ecs-clusters":
			clusters, err := a.AWS.Ecs.ListClusters(ctx)
			if err != nil {
				return resourceNamesMsg{kind: kind, names: []string{}}
			}
			for _, c := range clusters {
				names = append(names, c.Name)
			}
		case "mwaa-envs":
			envs, err := a.AWS.MWAA.ListEnvironments(ctx)
			if err != nil {
				return resourceNamesMsg{kind: kind, names: []string{}}
			}
			for _, e := range envs {
				names = append(names, e.Name)
			}
		}

		if names == nil {
			names = []string{}
		}
		return resourceNamesMsg{kind: kind, names: names}
	}
}

// fetchECSServicesCmd fetches service names for a cluster.
func fetchECSServicesCmd(a *app.App, cluster string) tea.Cmd {
	return func() tea.Msg {
		if a == nil || a.AWS == nil || a.AWS.Ecs == nil {
			return resourceNamesMsg{kind: "ecs-services", key: cluster, names: []string{}}
		}
		services, err := a.AWS.Ecs.ListServices(a.Context, cluster)
		if err != nil {
			return resourceNamesMsg{kind: "ecs-services", key: cluster, names: []string{}}
		}
		names := make([]string, 0, len(services))
		for _, s := range services {
			names = append(names, s.Name)
		}
		return resourceNamesMsg{kind: "ecs-services", key: cluster, names: names}
	}
}

// fetchECSTasksCmd fetches running task IDs for a cluster/service pair.
func fetchECSTasksCmd(a *app.App, cluster, service, cacheKey string) tea.Cmd {
	return func() tea.Msg {
		if a == nil || a.AWS == nil || a.AWS.Ecs == nil {
			return resourceNamesMsg{kind: "ecs-tasks", key: cacheKey, names: []string{}}
		}
		tasks, err := a.AWS.Ecs.ListRunningTasks(a.Context, cluster, service)
		if err != nil {
			return resourceNamesMsg{kind: "ecs-tasks", key: cacheKey, names: []string{}}
		}
		ids := make([]string, 0, len(tasks))
		for _, t := range tasks {
			ids = append(ids, t.TaskID)
		}
		return resourceNamesMsg{kind: "ecs-tasks", key: cacheKey, names: ids}
	}
}

// fetchMWAADagsCmd fetches DAG IDs for a given MWAA environment.
func fetchMWAADagsCmd(a *app.App, envName string) tea.Cmd {
	return func() tea.Msg {
		if a == nil || a.AWS == nil || a.AWS.MWAA == nil {
			return resourceNamesMsg{kind: "mwaa-dags", key: envName, names: []string{}}
		}
		dags, err := a.AWS.MWAA.ListDags(a.Context, envName)
		if err != nil {
			return resourceNamesMsg{kind: "mwaa-dags", key: envName, names: []string{}}
		}
		ids := make([]string, 0, len(dags))
		for _, d := range dags {
			ids = append(ids, d.DagId)
		}
		return resourceNamesMsg{kind: "mwaa-dags", key: envName, names: ids}
	}
}

// fetchMWAARunsCmd fetches recent run IDs for a specific DAG.
func fetchMWAARunsCmd(a *app.App, envName, dagId, cacheKey string) tea.Cmd {
	return func() tea.Msg {
		if a == nil || a.AWS == nil || a.AWS.MWAA == nil {
			return resourceNamesMsg{kind: "mwaa-runs", key: cacheKey, names: []string{}}
		}
		runs, err := a.AWS.MWAA.ListDagRuns(a.Context, envName, dagId)
		if err != nil {
			return resourceNamesMsg{kind: "mwaa-runs", key: cacheKey, names: []string{}}
		}
		ids := make([]string, 0, len(runs))
		for _, r := range runs {
			ids = append(ids, r.RunId)
		}
		return resourceNamesMsg{kind: "mwaa-runs", key: cacheKey, names: ids}
	}
}

// ── Dispatcher ────────────────────────────────────────────────────────────────

// handleHashCommand parses a raw # input and dispatches to the matching
// command method. Adding a new command only requires adding a case here and
// a corresponding handle*Cmd method below.
func (m DevOpsAgentViewModel) handleHashCommand(input string) (DevOpsAgentViewModel, tea.Cmd) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return m, nil
	}
	name := strings.ToLower(strings.TrimPrefix(parts[0], "#"))
	args := parts[1:]

	m.appendMsg("user", input)

	switch name {
	case "shell":
		return m.handleShellCmd(args)
	case "tunnel":
		return m.handleTunnelCmd(args)
	case "ls":
		return m.handleLsCmd(args)
	case "logs":
		return m.handleLogsCmd(args)
	case "help":
		return m.handleHelpCmd()
	default:
		m.appendMsg("system", fmt.Sprintf("Unknown command %q. Type #help for available commands.", name))
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil
	}
}

// ── Individual command handlers ───────────────────────────────────────────────

func (m DevOpsAgentViewModel) handleShellCmd(args []string) (DevOpsAgentViewModel, tea.Cmd) {
	if len(args) == 0 {
		m.appendMsg("system", "Usage: #shell <instance-id-or-name>")
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil
	}
	m.appendMsg("system", fmt.Sprintf("Resolving instance %q...", args[0]))
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	return m, tea.Batch(
		m.spinner.Tick,
		resolveInstanceCmd(m.app.Context, m.app.AWS.Ec2, args[0], ssmKindShell),
	)
}

func (m DevOpsAgentViewModel) handleTunnelCmd(args []string) (DevOpsAgentViewModel, tea.Cmd) {
	if len(args) == 0 {
		m.appendMsg("system", "Usage: #tunnel <ec2-or-rds-name>")
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil
	}
	m.appendMsg("system", fmt.Sprintf("Resolving tunnel target %q (trying EC2, then RDS)...", args[0]))
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	return m, tea.Batch(
		m.spinner.Tick,
		resolveTunnelTargetCmd(m.app.Context, m.app.AWS.Ec2, m.app.AWS.Rds, args[0]),
	)
}

func (m DevOpsAgentViewModel) handleListEC2Cmd() (DevOpsAgentViewModel, tea.Cmd) {
	m.appendMsg("system", "Fetching EC2 instances...")
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.Ec2
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		instances, err := svc.ListInstances(ctx)
		if err != nil {
			return listResultMsg{err: err}
		}
		return listResultMsg{content: formatEC2List(instances)}
	})
}

func (m DevOpsAgentViewModel) handleListRDSCmd() (DevOpsAgentViewModel, tea.Cmd) {
	m.appendMsg("system", "Fetching RDS instances...")
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.Rds
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		instances, err := svc.ListDBInstances(ctx)
		if err != nil {
			return listResultMsg{err: err}
		}
		return listResultMsg{content: formatRDSList(instances)}
	})
}

// sfnMachineEntry pairs a state machine with its recent executions.
type sfnMachineEntry struct {
	machine  aws.StateMachine
	execs    []aws.SfnExecution
	execsErr error
}

func (m DevOpsAgentViewModel) handleListSFNCmd() (DevOpsAgentViewModel, tea.Cmd) {
	m.appendMsg("system", "Fetching Step Functions and recent executions...")
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.Sfn
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		machines, err := svc.ListStateMachinesWithActivity(ctx)
		if err != nil {
			return listResultMsg{err: err}
		}
		entries := make([]sfnMachineEntry, len(machines))
		for i, machine := range machines {
			execs, execsErr := svc.ListExecutions(ctx, machine.Arn, 5)
			entries[i] = sfnMachineEntry{machine: machine, execs: execs, execsErr: execsErr}
		}
		return listResultMsg{content: formatSFNWithExecutions(entries)}
	})
}

func (m DevOpsAgentViewModel) handleListSFNRunsCmd(args []string) (DevOpsAgentViewModel, tea.Cmd) {
	if len(args) == 0 {
		m.appendMsg("system", "Usage: #listsfnruns <state-machine-name>")
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil
	}
	name := strings.Join(args, " ")
	m.appendMsg("system", fmt.Sprintf("Fetching executions for %q...", name))
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.Sfn
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		machine, err := svc.FindStateMachineByName(ctx, name)
		if err != nil {
			return listResultMsg{err: err}
		}
		execs, err := svc.ListExecutions(ctx, machine.Arn, 20)
		if err != nil {
			return listResultMsg{err: err}
		}
		return listResultMsg{content: formatSFNExecutions(machine.Name, execs)}
	})
}

// emrClusterWithSteps pairs a cluster with its recent steps.
type emrClusterWithSteps struct {
	cluster  aws.EmrCluster
	steps    []aws.EmrStep
	stepsErr error
}

func (m DevOpsAgentViewModel) handleListEMRCmd() (DevOpsAgentViewModel, tea.Cmd) {
	m.appendMsg("system", "Fetching EMR clusters and recent steps...")
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.Emr
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		clusters, err := svc.ListClustersWithActivity(ctx)
		if err != nil {
			return listResultMsg{err: err}
		}
		entries := make([]emrClusterWithSteps, len(clusters))
		for i, c := range clusters {
			steps, stepsErr := svc.GetRecentSteps(ctx, c.Id, 5)
			entries[i] = emrClusterWithSteps{cluster: c, steps: steps, stepsErr: stepsErr}
		}
		return listResultMsg{content: formatEMRWithSteps(entries)}
	})
}

func (m DevOpsAgentViewModel) handleListEMRJobsCmd(args []string) (DevOpsAgentViewModel, tea.Cmd) {
	if len(args) == 0 {
		m.appendMsg("system", "Usage: #listemrjobs <cluster-name>")
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil
	}
	name := strings.Join(args, " ")
	m.appendMsg("system", fmt.Sprintf("Fetching steps for cluster %q...", name))
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.Emr
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		cluster, err := svc.FindClusterByName(ctx, name)
		if err != nil {
			return listResultMsg{err: err}
		}
		steps, err := svc.ListSteps(ctx, cluster.Id)
		if err != nil {
			return listResultMsg{err: err}
		}
		return listResultMsg{content: formatEMRSteps(cluster.Name, steps)}
	})
}

// handleLsCmd is a short alias for the #list* family of commands.
//
//	#ls ec2                     → list EC2 instances
//	#ls rds                     → list RDS instances
//	#ls emr                     → list EMR clusters
//	#ls emr <name>              → list steps for a specific cluster
//	#ls sfn                     → list Step Functions state machines
//	#ls sfn <name>              → list executions for a specific state machine
//	#ls sfn <name> job <exec>   → show execution timeline (same as #logs sfn)
func (m DevOpsAgentViewModel) handleLsCmd(args []string) (DevOpsAgentViewModel, tea.Cmd) {
	if len(args) == 0 {
		m.appendMsg("system",
			"Usage: #ls <resource>\n"+
				"  ec2                  — EC2 instances\n"+
				"  rds                  — RDS instances\n"+
				"  emr                  — EMR clusters\n"+
				"  emr <name>           — steps for an EMR cluster\n"+
				"  sfn                  — Step Functions state machines\n"+
				"  sfn <name>           — executions for a state machine\n"+
				"  sfn <name> job <id>  — execution timeline",
		)
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil
	}

	switch strings.ToLower(args[0]) {
	case "ec2":
		return m.handleListEC2Cmd()
	case "rds":
		return m.handleListRDSCmd()

	case "sfn":
		if len(args) == 1 {
			// #ls sfn — list all state machines.
			return m.handleListSFNCmd()
		}
		// Check for "job" keyword to drill into a specific execution timeline.
		for i := 2; i < len(args); i++ {
			if strings.EqualFold(args[i], "job") && i < len(args)-1 {
				// #ls sfn <name> job <exec-id> — delegate to the logs command.
				// handleLogsCmd expects ["sfn", ...<name>..., "job", ...<exec-id>...]
				return m.handleLogsCmd(append([]string{"sfn"}, args[1:]...))
			}
		}
		// #ls sfn <name> — list executions for that state machine.
		return m.handleListSFNRunsCmd(args[1:])

	case "emr":
		if len(args) == 1 {
			// #ls emr — list all clusters.
			return m.handleListEMRCmd()
		}
		// #ls emr <name> — list steps for that cluster.
		return m.handleListEMRJobsCmd(args[1:])

	case "ecs":
		if len(args) == 1 {
			// #ls ecs — list all clusters.
			return m.handleListECSCmd()
		}
		// #ls ecs <cluster> service <service> — list running tasks.
		if cluster, service, ok := parseLsECSArgs(args[1:]); ok {
			return m.handleListECSTasksCmd(cluster, service)
		}
		// #ls ecs <cluster> — list services in that cluster.
		return m.handleListECSServicesCmd(strings.Join(args[1:], " "))

	case "mwaa":
		if len(args) == 1 {
			return m.handleListMWAACmd()
		}
		// #ls mwaa <env> job <dag> run <run> → list task instances
		if env, dag, run, ok := parseMWAARunArgs(args[1:]); ok {
			return m.handleListMWAATaskInstancesCmd(env, dag, run)
		}
		// #ls mwaa <env> job <dag> → list DAG runs
		if env, dag, ok := parseMWAAJobArgs(args[1:]); ok {
			return m.handleListMWAARunsCmd(env, dag)
		}
		// #ls mwaa <env> → list DAGs
		return m.handleListMWAADagsCmd(strings.Join(args[1:], " "))

	default:
		m.appendMsg("system", fmt.Sprintf(
			"Unknown resource %q. Try: ec2, rds, emr, emr <name>, sfn, sfn <name>, sfn <name> job <id>",
			args[0],
		))
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil
	}
}

// ecsClusterEntry pairs a cluster with its fetched service list.
type ecsClusterEntry struct {
	cluster  aws.EcsCluster
	services []aws.EcsServiceSummary
	svcErr   error
}

func (m DevOpsAgentViewModel) handleListECSCmd() (DevOpsAgentViewModel, tea.Cmd) {
	m.appendMsg("system", "Fetching ECS clusters and services...")
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.Ecs
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		clusters, err := svc.ListClusters(ctx)
		if err != nil {
			return listResultMsg{err: err}
		}
		entries := make([]ecsClusterEntry, len(clusters))
		for i, c := range clusters {
			svcs, svcErr := svc.ListServices(ctx, c.Name)
			entries[i] = ecsClusterEntry{cluster: c, services: svcs, svcErr: svcErr}
		}
		return listResultMsg{content: formatECSClustersWithServices(entries)}
	})
}

func (m DevOpsAgentViewModel) handleListECSServicesCmd(clusterName string) (DevOpsAgentViewModel, tea.Cmd) {
	m.appendMsg("system", fmt.Sprintf("Fetching services for cluster %q...", clusterName))
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.Ecs
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		services, err := svc.ListServices(ctx, clusterName)
		if err != nil {
			return listResultMsg{err: err}
		}
		return listResultMsg{content: formatECSServiceList(clusterName, services)}
	})
}

func (m DevOpsAgentViewModel) handleListECSTasksCmd(clusterName, serviceName string) (DevOpsAgentViewModel, tea.Cmd) {
	m.appendMsg("system", fmt.Sprintf(
		"Fetching running tasks for service %q in cluster %q...", serviceName, clusterName,
	))
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.Ecs
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		tasks, err := svc.ListRunningTasks(ctx, clusterName, serviceName)
		if err != nil {
			return listResultMsg{err: err}
		}
		return listResultMsg{content: formatECSTaskList(clusterName, serviceName, tasks)}
	})
}

// ── MWAA handlers ─────────────────────────────────────────────────────────────

func (m DevOpsAgentViewModel) handleListMWAACmd() (DevOpsAgentViewModel, tea.Cmd) {
	m.appendMsg("system", "Fetching MWAA environments...")
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.MWAA
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		envs, err := svc.ListEnvironments(ctx)
		if err != nil {
			return listResultMsg{err: err}
		}
		return listResultMsg{content: formatMWAAEnvironmentList(envs)}
	})
}

func (m DevOpsAgentViewModel) handleListMWAADagsCmd(envName string) (DevOpsAgentViewModel, tea.Cmd) {
	m.appendMsg("system", fmt.Sprintf("Fetching DAGs for environment %q...", envName))
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.MWAA
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		dags, err := svc.ListDags(ctx, envName)
		if err != nil {
			return listResultMsg{err: err}
		}
		return listResultMsg{content: formatMWAADagList(envName, dags)}
	})
}

func (m DevOpsAgentViewModel) handleListMWAARunsCmd(envName, dagId string) (DevOpsAgentViewModel, tea.Cmd) {
	m.appendMsg("system", fmt.Sprintf("Fetching runs for DAG %q in %q...", dagId, envName))
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.MWAA
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		runs, err := svc.ListDagRuns(ctx, envName, dagId)
		if err != nil {
			return listResultMsg{err: err}
		}
		return listResultMsg{content: formatMWAARunList(envName, dagId, runs)}
	})
}

func (m DevOpsAgentViewModel) handleListMWAATaskInstancesCmd(envName, dagId, runId string) (DevOpsAgentViewModel, tea.Cmd) {
	m.appendMsg("system", fmt.Sprintf("Fetching task instances for run %q...", runId))
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.MWAA
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		tasks, err := svc.ListTaskInstances(ctx, envName, dagId, runId)
		if err != nil {
			return listResultMsg{err: err}
		}
		return listResultMsg{content: formatMWAATaskInstanceList(envName, dagId, runId, tasks)}
	})
}

func (m DevOpsAgentViewModel) handleMWAALogsCmd(envName, dagId, runId string) (DevOpsAgentViewModel, tea.Cmd) {
	m.appendMsg("system", fmt.Sprintf("Fetching logs for run %q (DAG %q, env %q)...", runId, dagId, envName))
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.MWAA
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		logs, err := svc.GetRunLogs(ctx, envName, dagId, runId)
		if err != nil {
			return listResultMsg{err: err}
		}
		return listResultMsg{content: logs}
	})
}

func (m DevOpsAgentViewModel) handleHelpCmd() (DevOpsAgentViewModel, tea.Cmd) {
	m.appendMsg("system", devOpsHelpText())
	m.syncViewport()
	m.viewport.GotoBottom()
	return m, nil
}

// handleLogsCmd handles:
//   #logs sfn <state_machine_name> job <execution_name>
//   #logs ecs <cluster> service <service_name> task <task_id>
//   #logs mwaa <env> job <dag> run <run_id>
func (m DevOpsAgentViewModel) handleLogsCmd(args []string) (DevOpsAgentViewModel, tea.Cmd) {
	if len(args) > 0 && strings.EqualFold(args[0], "mwaa") {
		env, dag, run, ok := parseMWAARunArgs(args[1:])
		if !ok {
			m.appendMsg("system",
				"Usage: #logs mwaa <environment> job <dag_id> run <run_id>")
			m.syncViewport()
			m.viewport.GotoBottom()
			return m, nil
		}
		return m.handleMWAALogsCmd(env, dag, run)
	}

	if len(args) > 0 && strings.EqualFold(args[0], "ecs") {
		cluster, service, taskID, ok := parseLogsECSArgs(args[1:])
		if !ok {
			m.appendMsg("system",
				"Usage: #logs ecs <cluster> service <service_name> task <task_id>")
			m.syncViewport()
			m.viewport.GotoBottom()
			return m, nil
		}
		m.appendMsg("system", fmt.Sprintf(
			"Fetching container logs for task %q (cluster %q, service %q)...",
			taskID, cluster, service,
		))
		m.isSending = true
		m.syncViewport()
		m.viewport.GotoBottom()
		ctx := m.app.Context
		svc := m.app.AWS.Ecs
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			logs, err := svc.GetContainerLogs(ctx, cluster, taskID, 200)
			if err != nil {
				return listResultMsg{err: err}
			}
			return listResultMsg{content: formatECSContainerLogs(cluster, service, taskID, logs)}
		})
	}

	// Default: SFN execution timeline.
	sfnName, jobName, ok := parseSFNLogsArgs(args)
	if !ok {
		m.appendMsg("system",
			"Usage:\n"+
				"  #logs sfn <state_machine_name> job <execution_name>\n"+
				"  #logs ecs <cluster> service <service_name> task <task_id>")
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil
	}
	m.appendMsg("system", fmt.Sprintf("Fetching logs for execution %q on %q...", jobName, sfnName))
	m.isSending = true
	m.syncViewport()
	m.viewport.GotoBottom()
	ctx := m.app.Context
	svc := m.app.AWS.Sfn
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		machine, err := svc.FindStateMachineByName(ctx, sfnName)
		if err != nil {
			return listResultMsg{err: err}
		}
		events, err := svc.GetExecutionLogs(ctx, machine.Arn, jobName)
		if err != nil {
			return listResultMsg{err: err}
		}
		return listResultMsg{content: formatSFNLogs(machine.Name, jobName, events)}
	})
}

// ── SSM resolution commands ───────────────────────────────────────────────────

// resolveInstanceCmd looks up an EC2 instance by ID or name tag for #shell.
func resolveInstanceCmd(ctx context.Context, ec2Svc *aws.Ec2Service, search string, kind ssmKind) tea.Cmd {
	return func() tea.Msg {
		instances, err := ec2Svc.ListInstances(ctx)
		if err != nil {
			return ssmInstanceResolvedMsg{err: fmt.Errorf("list instances: %w", err), kind: kind}
		}
		inst, err := findDevOpsInstance(instances, search)
		return ssmInstanceResolvedMsg{instance: inst, kind: kind, err: err}
	}
}

// resolveTunnelTargetCmd is used by #tunnel: tries EC2 first, falls back to
// RDS and locates the bastion in the same VPC.
func resolveTunnelTargetCmd(ctx context.Context, ec2Svc *aws.Ec2Service, rdsSvc *aws.RdsService, search string) tea.Cmd {
	return func() tea.Msg {
		if ec2Instances, err := ec2Svc.ListInstances(ctx); err == nil {
			if inst, err := findDevOpsInstance(ec2Instances, search); err == nil {
				return ssmInstanceResolvedMsg{instance: inst, kind: ssmKindTunnel}
			}
		}
		rdsInstances, err := rdsSvc.ListDBInstances(ctx)
		if err != nil {
			return ssmRDSTunnelResolvedMsg{err: fmt.Errorf("no EC2 or RDS instance matching %q", search)}
		}
		rds, err := findRDSBySearch(rdsInstances, search)
		if err != nil {
			return ssmRDSTunnelResolvedMsg{err: fmt.Errorf("no EC2 or RDS instance matching %q", search)}
		}
		bastion, err := ec2Svc.FindBastionHost(ctx, rds.VpcID)
		if err != nil {
			return ssmRDSTunnelResolvedMsg{
				err: fmt.Errorf("RDS instance %q found but no bastion in VPC %s: %w", rds.Id, rds.VpcID, err),
			}
		}
		return ssmRDSTunnelResolvedMsg{rds: rds, bastion: bastion}
	}
}

// ── SSM form callbacks ────────────────────────────────────────────────────────

// portForwardInputs returns the form fields for EC2 SSM port forwarding.
func portForwardInputs() []c.InputField {
	return []c.InputField{
		{Label: LocalPortLabel, Placeholder: "8080", CharLimit: 5, Validator: aws.ValidatePort},
		{Label: RemotePortLabel, Placeholder: "8080", CharLimit: 5, Validator: aws.ValidatePort},
	}
}

// portForwardOnSubmit handles the EC2 direct port-forward form submission.
func (m DevOpsAgentViewModel) portForwardOnSubmit(values c.InputFormResult) tea.Cmd {
	inst := m.pendingTunnelInstance
	localPort := mustAtoi(values[LocalPortLabel])
	remotePort := mustAtoi(values[RemotePortLabel])
	config := aws.PortForwardConfig{LocalPort: localPort, RemotePort: remotePort}
	cmd := m.app.AWS.Ssm.BuildPortForwardCmd(inst.InstanceID, config)
	desc := fmt.Sprintf("tunnel %s (%s) localhost:%d → :%d", inst.Name, inst.InstanceID, localPort, remotePort)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return ssmExitedMsg{description: desc, err: err}
	})
}

// rdsPortForwardOnSubmit handles the RDS-via-bastion port-forward form submission.
func (m DevOpsAgentViewModel) rdsPortForwardOnSubmit(values c.InputFormResult) tea.Cmd {
	localPort := mustAtoi(values[LocalPortLabel])
	config := aws.PortForwardConfig{
		LocalPort:  localPort,
		RemotePort: int(m.pendingRDSInstance.Port),
		RemoteHost: m.pendingRDSInstance.Endpoint,
	}
	cmd := m.app.AWS.Ssm.BuildRemotePortForwardCmd(m.pendingBastion.InstanceID, config)
	desc := fmt.Sprintf(
		"RDS tunnel %s → localhost:%d (via bastion %s)",
		m.pendingRDSInstance.Id, localPort, m.pendingBastion.Name,
	)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return ssmExitedMsg{description: desc, err: err}
	})
}

// ── Instance lookup helpers ───────────────────────────────────────────────────

func findDevOpsInstance(instances []aws.Ec2Instance, search string) (aws.Ec2Instance, error) {
	lower := strings.ToLower(search)
	for _, i := range instances {
		if i.InstanceID == search {
			return i, nil
		}
	}
	for _, i := range instances {
		if strings.EqualFold(i.Name, search) {
			return i, nil
		}
	}
	for _, i := range instances {
		if strings.Contains(strings.ToLower(i.Name), lower) {
			return i, nil
		}
	}
	return aws.Ec2Instance{}, fmt.Errorf("no instance found matching %q", search)
}

func findRDSBySearch(instances []aws.RDSInstance, search string) (aws.RDSInstance, error) {
	lower := strings.ToLower(search)
	for _, i := range instances {
		if i.Id == search {
			return i, nil
		}
	}
	for _, i := range instances {
		if strings.EqualFold(i.Id, search) {
			return i, nil
		}
	}
	for _, i := range instances {
		if strings.Contains(strings.ToLower(i.Id), lower) {
			return i, nil
		}
	}
	return aws.RDSInstance{}, fmt.Errorf("no RDS instance matching %q", search)
}

// ── Formatters ────────────────────────────────────────────────────────────────

func formatEC2List(instances []aws.Ec2Instance) string {
	if len(instances) == 0 {
		return "No EC2 instances found."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("EC2 Instances (%d):\n", len(instances)))
	for _, i := range instances {
		name := i.Name
		if name == "" {
			name = "(unnamed)"
		}
		sb.WriteString(fmt.Sprintf("  • %-36s  %-21s  %s\n", name, i.InstanceID, i.State))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatRDSList(instances []aws.RDSInstance) string {
	if len(instances) == 0 {
		return "No RDS instances found."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("RDS Instances (%d):\n", len(instances)))
	for _, i := range instances {
		engine := i.DbEngine + " " + i.DbVersion
		endpoint := i.Endpoint
		if i.Port > 0 {
			endpoint = fmt.Sprintf("%s:%d", i.Endpoint, i.Port)
		}
		sb.WriteString(fmt.Sprintf("  • %-36s  %-22s  %-10s  %s\n", i.Id, engine, i.Status, endpoint))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatSFNList(machines []aws.StateMachine) string {
	if len(machines) == 0 {
		return "No Step Functions state machines found."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Step Functions (%d, sorted by last execution):\n", len(machines)))
	for _, m := range machines {
		lastRun := "never"
		if m.LastExecutionTime != nil {
			lastRun = m.LastExecutionTime.Local().Format("2006-01-02 15:04:05")
		}
		sb.WriteString(fmt.Sprintf("  • %-44s  %-10s  %s\n", m.Name, m.Type, lastRun))
	}
	sb.WriteString("\nTip: use #ls sfn <name> to list executions.")
	return strings.TrimRight(sb.String(), "\n")
}

// formatSFNWithExecutions renders a state-machine → executions tree.
func formatSFNWithExecutions(entries []sfnMachineEntry) string {
	if len(entries) == 0 {
		return "No Step Functions state machines found."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Step Functions (%d, sorted by last execution):\n", len(entries)))
	for _, e := range entries {
		lastRun := "never"
		if e.machine.LastExecutionTime != nil {
			lastRun = e.machine.LastExecutionTime.Local().Format("2006-01-02 15:04:05")
		}
		sb.WriteString(fmt.Sprintf("\n  %s  [%s]  last: %s\n", e.machine.Name, e.machine.Type, lastRun))
		if e.execsErr != nil {
			sb.WriteString(fmt.Sprintf("    (error fetching executions: %v)\n", e.execsErr))
			continue
		}
		if len(e.execs) == 0 {
			sb.WriteString("    (no executions)\n")
			continue
		}
		for _, ex := range e.execs {
			started := ex.StartDate.Local().Format("2006-01-02 15:04:05")
			sb.WriteString(fmt.Sprintf("    • %-40s  %-12s  %s\n", ex.Name, ex.Status, started))
		}
	}
	sb.WriteString("\nTip: use #ls sfn <name> <exec-id> or #logs sfn <name> job <exec-id> for details.")
	return strings.TrimRight(sb.String(), "\n")
}

func formatSFNExecutions(machineName string, execs []aws.SfnExecution) string {
	if len(execs) == 0 {
		return fmt.Sprintf("No executions found for %q.", machineName)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Executions for %s (%d):\n", machineName, len(execs)))
	for _, e := range execs {
		started := e.StartDate.Format("2006-01-02 15:04:05")
		sb.WriteString(fmt.Sprintf("  • %-40s  %-12s  %s\n", e.Name, e.Status, started))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatEMRList(clusters []aws.EmrCluster) string {
	if len(clusters) == 0 {
		return "No EMR clusters found."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("EMR Clusters (%d, sorted by last job):\n", len(clusters)))
	for _, c := range clusters {
		lastJob := "never"
		if c.LastJobTime != nil {
			lastJob = c.LastJobTime.Local().Format("2006-01-02 15:04:05")
		}
		sb.WriteString(fmt.Sprintf("  • %-40s  %-14s  %s  %s\n", c.Name, c.State, c.Id, lastJob))
	}
	sb.WriteString("\nTip: use #ls emr <cluster-name> to list steps.")
	return strings.TrimRight(sb.String(), "\n")
}

// formatEMRWithSteps renders a cluster → recent steps tree.
func formatEMRWithSteps(entries []emrClusterWithSteps) string {
	if len(entries) == 0 {
		return "No EMR clusters found."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("EMR Clusters (%d, sorted by last job):\n", len(entries)))
	for _, e := range entries {
		lastJob := "never"
		if e.cluster.LastJobTime != nil {
			lastJob = e.cluster.LastJobTime.Local().Format("2006-01-02 15:04:05")
		}
		sb.WriteString(fmt.Sprintf("\n  %s  [%s]  last: %s\n", e.cluster.Name, e.cluster.State, lastJob))
		if e.stepsErr != nil {
			sb.WriteString(fmt.Sprintf("    (error fetching steps: %v)\n", e.stepsErr))
			continue
		}
		if len(e.steps) == 0 {
			sb.WriteString("    (no steps)\n")
			continue
		}
		for _, s := range e.steps {
			sb.WriteString(fmt.Sprintf("    • %-40s  %s\n", s.Name, s.State))
		}
	}
	sb.WriteString("\nTip: use #ls emr <cluster-name> to list all steps.")
	return strings.TrimRight(sb.String(), "\n")
}

func formatEMRSteps(clusterName string, steps []aws.EmrStep) string {
	if len(steps) == 0 {
		return fmt.Sprintf("No steps found for cluster %q.", clusterName)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Steps for %s (%d):\n", clusterName, len(steps)))
	for _, s := range steps {
		sb.WriteString(fmt.Sprintf("  • %-40s  %-12s  %s\n", s.Name, s.State, s.Id))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ── ECS formatters ────────────────────────────────────────────────────────────

func formatECSClusterList(clusters []aws.EcsCluster) string {
	if len(clusters) == 0 {
		return "No ECS clusters found."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ECS Clusters (%d):\n", len(clusters)))
	for _, c := range clusters {
		sb.WriteString(fmt.Sprintf("  • %s\n", c.Name))
	}
	sb.WriteString("\nTip: use #ls ecs <cluster-name> to list services.")
	return strings.TrimRight(sb.String(), "\n")
}

// formatECSClustersWithServices renders a cluster→service tree used by
// #ls ecs (no arguments). Each cluster lists its services indented beneath it.
func formatECSClustersWithServices(entries []ecsClusterEntry) string {
	if len(entries) == 0 {
		return "No ECS clusters found."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ECS (%d cluster(s)):\n", len(entries)))
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("\n  %s\n", e.cluster.Name))
		if e.svcErr != nil {
			sb.WriteString(fmt.Sprintf("    (error fetching services: %v)\n", e.svcErr))
			continue
		}
		if len(e.services) == 0 {
			sb.WriteString("    (no services)\n")
			continue
		}
		for _, s := range e.services {
			sb.WriteString(fmt.Sprintf("    • %s\n", s.Name))
		}
	}
	sb.WriteString("\nTip: use #ls ecs <cluster> service <service> to list running tasks.")
	return strings.TrimRight(sb.String(), "\n")
}

func formatECSServiceList(clusterName string, services []aws.EcsServiceSummary) string {
	if len(services) == 0 {
		return fmt.Sprintf("No services found in cluster %q.", clusterName)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Services in %s (%d):\n", clusterName, len(services)))
	for _, s := range services {
		sb.WriteString(fmt.Sprintf("  • %s\n", s.Name))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatECSTaskList(clusterName, serviceName string, tasks []aws.EcsTaskSummary) string {
	if len(tasks) == 0 {
		return fmt.Sprintf("No running tasks found for service %q in cluster %q.", serviceName, clusterName)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Running tasks — %s / %s (%d):\n", clusterName, serviceName, len(tasks)))
	for _, t := range tasks {
		sb.WriteString(fmt.Sprintf("  • %s\n", t.TaskID))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatECSContainerLogs(clusterName, serviceName, taskID string, containers []aws.ContainerLogs) string {
	if len(containers) == 0 {
		return fmt.Sprintf(
			"No container logs found for task %q (cluster %q, service %q).\n"+
				"Make sure the task uses the awslogs log driver.",
			taskID, clusterName, serviceName)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Logs for task %s / %s / %s:\n", clusterName, serviceName, taskID))
	for _, c := range containers {
		sb.WriteString(fmt.Sprintf("\n  ── %s", c.ContainerName))
		if c.LogGroup != "" {
			sb.WriteString(fmt.Sprintf("  (%s)", c.LogGroup))
		}
		sb.WriteString(" ──\n")
		if len(c.Events) == 0 {
			sb.WriteString("  (no log events)\n")
			continue
		}
		for _, e := range c.Events {
			ts := ""
			if !e.Timestamp.IsZero() {
				ts = e.Timestamp.Local().Format("2006-01-02 15:04:05") + "  "
			}
			sb.WriteString(fmt.Sprintf("  %s%s\n", ts, strings.TrimRight(e.Message, "\n\r")))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ── ECS argument parsers ──────────────────────────────────────────────────────

// parseLsECSArgs parses "<cluster> service <service_name>" from args.
func parseLsECSArgs(args []string) (cluster, service string, ok bool) {
	for i, a := range args {
		if strings.EqualFold(a, "service") && i > 0 && i < len(args)-1 {
			return strings.Join(args[:i], " "), strings.Join(args[i+1:], " "), true
		}
	}
	return "", "", false
}

// parseLogsECSArgs parses "<cluster> service <service_name> task <task_id>" from args.
func parseLogsECSArgs(args []string) (cluster, service, taskID string, ok bool) {
	// Find "service" keyword.
	svcIdx := -1
	for i, a := range args {
		if strings.EqualFold(a, "service") {
			svcIdx = i
			break
		}
	}
	if svcIdx < 0 || svcIdx >= len(args)-1 {
		return "", "", "", false
	}
	cluster = strings.Join(args[:svcIdx], " ")
	rest := args[svcIdx+1:]

	// Find "task" keyword.
	taskIdx := -1
	for i, a := range rest {
		if strings.EqualFold(a, "task") {
			taskIdx = i
			break
		}
	}
	if taskIdx < 0 || taskIdx >= len(rest)-1 {
		return "", "", "", false
	}
	service = strings.Join(rest[:taskIdx], " ")
	taskID = strings.Join(rest[taskIdx+1:], " ")

	if cluster == "" || service == "" || taskID == "" {
		return "", "", "", false
	}
	return cluster, service, taskID, true
}

// ── MWAA argument parsers ─────────────────────────────────────────────────────

// parseMWAAJobArgs parses "<env> job <dag_id>" from args.
// "job" is the keyword separating the environment name from the DAG id.
func parseMWAAJobArgs(args []string) (envName, dagId string, ok bool) {
	for i, a := range args {
		if strings.EqualFold(a, "job") && i > 0 && i < len(args)-1 {
			return strings.Join(args[:i], " "), strings.Join(args[i+1:], " "), true
		}
	}
	return "", "", false
}

// parseMWAARunArgs parses "<env> job <dag_id> run <run_id>" from args.
func parseMWAARunArgs(args []string) (envName, dagId, runId string, ok bool) {
	// Find "run" keyword.
	runIdx := -1
	for i, a := range args {
		if strings.EqualFold(a, "run") {
			runIdx = i
			break
		}
	}
	if runIdx < 0 || runIdx >= len(args)-1 {
		return "", "", "", false
	}
	// Find "job" keyword before "run".
	jobIdx := -1
	for i, a := range args[:runIdx] {
		if strings.EqualFold(a, "job") {
			jobIdx = i
			break
		}
	}
	if jobIdx < 0 || jobIdx >= runIdx-1 {
		return "", "", "", false
	}
	envName = strings.Join(args[:jobIdx], " ")
	dagId = strings.Join(args[jobIdx+1:runIdx], " ")
	runId = strings.Join(args[runIdx+1:], " ")
	if envName == "" || dagId == "" || runId == "" {
		return "", "", "", false
	}
	return envName, dagId, runId, true
}

// ── MWAA formatters ───────────────────────────────────────────────────────────

func formatMWAAEnvironmentList(envs []aws.MWAAEnvironment) string {
	if len(envs) == 0 {
		return "No MWAA environments found."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("MWAA Environments (%d):\n", len(envs)))
	for _, e := range envs {
		sb.WriteString(fmt.Sprintf("  • %s\n", e.Name))
	}
	sb.WriteString("\nTip: use #ls mwaa <env> to list DAGs.")
	return strings.TrimRight(sb.String(), "\n")
}

func formatMWAADagList(envName string, dags []aws.MWAADag) string {
	if len(dags) == 0 {
		return fmt.Sprintf("No DAGs found in environment %q.", envName)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("DAGs in %s (%d):\n", envName, len(dags)))
	for _, d := range dags {
		paused := ""
		if d.IsPaused {
			paused = "  [paused]"
		}
		sb.WriteString(fmt.Sprintf("  • %s%s\n", d.DagId, paused))
	}
	sb.WriteString("\nTip: use #ls mwaa <env> job <dag> to list runs.")
	return strings.TrimRight(sb.String(), "\n")
}

func formatMWAARunList(envName, dagId string, runs []aws.MWAADagRun) string {
	if len(runs) == 0 {
		return fmt.Sprintf("No runs found for DAG %q in environment %q.", dagId, envName)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Runs for %s / %s (%d, newest first):\n", envName, dagId, len(runs)))
	for _, r := range runs {
		ts := r.ExecutionDate
		if len(ts) > 19 {
			ts = ts[:19]
		}
		sb.WriteString(fmt.Sprintf("  • %-50s  %-12s  %s\n", r.RunId, r.State, ts))
	}
	sb.WriteString("\nTip: use #ls mwaa <env> job <dag> run <run> to list task instances.")
	return strings.TrimRight(sb.String(), "\n")
}

func formatMWAATaskInstanceList(envName, dagId, runId string, tasks []aws.MWAATaskInstance) string {
	if len(tasks) == 0 {
		return fmt.Sprintf("No task instances found for run %q.", runId)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Task instances for %s / %s / %s (%d):\n", envName, dagId, runId, len(tasks)))
	for _, t := range tasks {
		sb.WriteString(fmt.Sprintf("  • %-40s  %-12s  try %d\n", t.TaskId, t.State, t.TryNumber))
	}
	sb.WriteString("\nTip: use #logs mwaa <env> job <dag> run <run> to see task logs.")
	return strings.TrimRight(sb.String(), "\n")
}

// ── #logs parser & formatter ──────────────────────────────────────────────────

// parseSFNLogsArgs extracts the state machine name and execution name from
// the args slice for:  #logs sfn <sfn_name> job <job_name>
// The keywords "sfn" and "job" are case-insensitive separators; names may
// contain spaces.
func parseSFNLogsArgs(args []string) (sfnName, jobName string, ok bool) {
	if len(args) < 4 {
		return "", "", false
	}
	if !strings.EqualFold(args[0], "sfn") {
		return "", "", false
	}
	// Find "job" keyword after the initial "sfn".
	jobIdx := -1
	for i := 2; i < len(args); i++ {
		if strings.EqualFold(args[i], "job") {
			jobIdx = i
			break
		}
	}
	if jobIdx < 0 || jobIdx >= len(args)-1 {
		return "", "", false
	}
	sfnName = strings.Join(args[1:jobIdx], " ")
	jobName = strings.Join(args[jobIdx+1:], " ")
	if sfnName == "" || jobName == "" {
		return "", "", false
	}
	return sfnName, jobName, true
}

func formatSFNLogs(machineName, executionName string, events []aws.SfnLogEvent) string {
	if len(events) == 0 {
		return fmt.Sprintf("No log events found for execution %q on %q.\n"+
			"The execution may not exist, or logging may not be enabled.",
			executionName, machineName)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Logs for %s / %s (%d events):\n", machineName, executionName, len(events)))

	for _, e := range events {
		ts := e.Timestamp.Local().Format("2006-01-02 15:04:05")
		label := eventLabel(e.Type)

		if e.StateName != "" {
			sb.WriteString(fmt.Sprintf("  %s  %s  %s\n", ts, label, e.StateName))
		} else {
			sb.WriteString(fmt.Sprintf("  %s  %s\n", ts, label))
		}
		if e.Error != "" {
			sb.WriteString(fmt.Sprintf("             Error: %s\n", e.Error))
		}
		if e.Cause != "" {
			// Wrap long cause strings.
			cause := e.Cause
			if len(cause) > 120 {
				cause = cause[:120] + "…"
			}
			sb.WriteString(fmt.Sprintf("             Cause: %s\n", cause))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// eventLabel converts a raw HistoryEventType string into a short display label.
func eventLabel(eventType string) string {
	switch eventType {
	case "ExecutionStarted":
		return "▶ started"
	case "ExecutionSucceeded":
		return "✓ succeeded"
	case "ExecutionFailed":
		return "✗ failed"
	case "ExecutionAborted":
		return "⊘ aborted"
	case "ExecutionTimedOut":
		return "⧗ timed out"
	case "LambdaFunctionSucceeded", "TaskSucceeded", "ActivitySucceeded":
		return "  ✓ task succeeded"
	case "LambdaFunctionFailed", "TaskFailed", "ActivityFailed",
		"LambdaFunctionScheduleFailed", "LambdaFunctionStartFailed":
		return "  ✗ task failed"
	case "LambdaFunctionTimedOut", "TaskTimedOut", "ActivityTimedOut":
		return "  ⧗ task timed out"
	}
	// State entry / exit
	if strings.HasSuffix(eventType, "StateEntered") || eventType == "StateEntered" {
		return "  → entered"
	}
	if strings.HasSuffix(eventType, "StateExited") || eventType == "StateExited" {
		return "  ← exited"
	}
	if strings.HasSuffix(eventType, "Entered") {
		return "  → entered"
	}
	if strings.HasSuffix(eventType, "Exited") {
		return "  ← exited"
	}
	return "  " + eventType
}

// ── Help text ─────────────────────────────────────────────────────────────────

func devOpsHelpText() string {
	return "Available commands:\n" +
		"\n" +
		"  Connectivity\n" +
		"  #shell <instance>              — open an SSM shell session\n" +
		"  #tunnel <ec2-or-rds-name>      — port forwarding; EC2 direct, RDS via bastion\n" +
		"\n" +
		"  Listings  (#ls <resource>)\n" +
		"  #ls ec2                                     — EC2 instances\n" +
		"  #ls rds                                     — RDS instances\n" +
		"  #ls emr                                     — EMR clusters\n" +
		"  #ls emr <name>                              — steps for an EMR cluster\n" +
		"  #ls sfn                                     — Step Functions state machines\n" +
		"  #ls sfn <name>                              — executions for a state machine\n" +
		"  #ls sfn <name> job <exec-id>                — execution timeline\n" +
		"  #ls ecs                                     — ECS clusters\n" +
		"  #ls ecs <cluster>                           — services in a cluster\n" +
		"  #ls ecs <cluster> service <service>         — running tasks for a service\n" +
		"  #ls mwaa                                    — MWAA environments\n" +
		"  #ls mwaa <env>                              — DAGs in an environment\n" +
		"  #ls mwaa <env> job <dag>                    — runs for a DAG\n" +
		"  #ls mwaa <env> job <dag> run <run>          — task instances for a run\n" +
		"\n" +
		"  Logs\n" +
		"  #logs sfn <sfn> job <exec>                      — SFN execution timeline\n" +
		"  #logs ecs <cluster> service <svc> task <id>     — ECS container logs\n" +
		"  #logs mwaa <env> job <dag> run <run>            — MWAA DAG run task logs\n" +
		"\n" +
		"  #help                          — show this message\n" +
		"\n" +
		"Name matching: exact ID → exact name (case-insensitive) → partial name.\n" +
		"Scroll: ↑/↓ line  •  PgUp/PgDn or ctrl+u/ctrl+d half page"
}

// ── Shared utilities ──────────────────────────────────────────────────────────

func mustAtoi(s string) int {
	v := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		v = v*10 + int(ch-'0')
	}
	return v
}
