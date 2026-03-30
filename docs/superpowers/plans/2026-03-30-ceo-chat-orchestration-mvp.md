# CEO Chat Orchestration MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a CEO chat profile that can orchestrate work items through a narrow orchestration CLI, with task-first behavior and thread escalation only when needed.

**Architecture:** Introduce a new `orchestrateapp` application service as the stable business-action layer for CLI calls. Wire a neutral `orchestrate task ...` Cobra command into `ai-flow`, reuse existing work item / planning / thread services, and add CEO-specific profile/skill config plus append-only journal metadata and preferred-profile overrides.

**Tech Stack:** Go, Cobra CLI, existing `internal/application/*` services, SQLite store, ACP profile registry, embedded builtin skills, TOML runtime config.

---

## File Map

### Existing files to modify

- `cmd/ai-flow/root.go`
  Add `orchestrate` command registration and dependency injection hooks.
- `cmd/ai-flow/main_test.go`
  Extend root-command tests for new CLI wiring and flag forwarding.
- `internal/platform/appcmd/config_loader.go`
  Reuse config/bootstrap loading helpers for a non-server command path.
- `internal/platform/config/defaults.toml`
  Add a default `ceo` profile referencing the new prompt template and skill.
- `internal/application/planning/service.go`
  Reuse DAG generation/materialization and add guardrails for overwrite conflicts.
- `internal/application/flow/resolver.go`
  Respect `Action.Config["preferred_profile_id"]` before role/capability fallback.
- `internal/core/action.go`
  Document the preferred-profile override contract in the action config comments if needed.
- `internal/skills/builtin/embed.go`
  Ensure the new builtin CEO skill is embedded and extracted.

### New files to create

- `cmd/ai-flow/orchestrate_cmd.go`
  Cobra command tree for `orchestrate task ...`.
- `internal/platform/appcmd/orchestrate.go`
  CLI runtime bootstrap and JSON output helpers for orchestration actions.
- `internal/platform/appcmd/orchestrate_test.go`
  Argument parsing and JSON output tests for orchestration command handlers.
- `internal/application/orchestrateapp/service.go`
  Application service implementing create/decompose/assign/follow-up/reassign/escalate.
- `internal/application/orchestrateapp/contracts.go`
  Store/service interfaces the orchestration service depends on.
- `internal/application/orchestrateapp/errors.go`
  Stable orchestration-layer error codes for CLI consumption.
- `internal/application/orchestrateapp/service_test.go`
  Unit tests for idempotent create, decompose conflicts, profile override propagation, follow-up summaries, reassign journal writes, and thread escalation.
- `configs/prompts/ceo_orchestrator.tmpl`
  Prompt template for the CEO chat profile.
- `internal/skills/builtin/ceo-manage/SKILL.md`
  Builtin skill teaching task-first orchestration rules.
- `internal/skills/builtin/ceo-manage/agents/openai.yaml`
  UI metadata for the builtin CEO skill.

### Existing tests likely to extend

- `internal/application/planning/service_test.go`
  Add overwrite-conflict or materialization propagation coverage if shared helpers change.
- `internal/skills/builtin_test.go`
  Verify builtin extraction includes `ceo-manage`.

---

### Task 1: Add The Orchestration CLI Skeleton

**Files:**
- Create: `cmd/ai-flow/orchestrate_cmd.go`
- Create: `internal/platform/appcmd/orchestrate.go`
- Create: `internal/platform/appcmd/orchestrate_test.go`
- Modify: `cmd/ai-flow/root.go`
- Modify: `cmd/ai-flow/main_test.go`

- [ ] **Step 1: Write the failing root-command test**

```go
func TestOrchestrateCommandForwardsTaskCreateFlags(t *testing.T) {
	t.Parallel()

	var gotArgs []string
	cmd := newRootCmd(commandDeps{
		out:             &bytes.Buffer{},
		err:             &bytes.Buffer{},
		version:         versionString,
		runServer:       func([]string) error { return nil },
		runExecutor:     func([]string) error { return nil },
		runQualityGate:  func([]string) error { return nil },
		runMCPServe:     func([]string) error { return nil },
		runOrchestrate: func(args []string) error {
			gotArgs = append([]string(nil), args...)
			return nil
		},
	})
	cmd.SetArgs([]string{
		"orchestrate", "task", "create",
		"--title", "CEO bootstrap",
		"--project-id", "12",
		"--json",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := []string{"task", "create", "--title", "CEO bootstrap", "--project-id", "12", "--json"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("orchestrate args = %#v, want %#v", gotArgs, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/ai-flow -run TestOrchestrateCommandForwardsTaskCreateFlags -count=1`

Expected: FAIL because `commandDeps` and `newRootCmd` do not know about `runOrchestrate`.

- [ ] **Step 3: Add the Cobra command skeleton and dependency hook**

```go
type commandDeps struct {
	out            io.Writer
	err            io.Writer
	version        string
	runServer      func([]string) error
	runExecutor    func([]string) error
	runQualityGate func([]string) error
	runMCPServe    func([]string) error
	runOrchestrate func([]string) error
}

func newOrchestrateCmd(deps commandDeps) *cobra.Command {
	root := &cobra.Command{
		Use:   "orchestrate",
		Short: "Run orchestration control actions",
	}
	taskCmd := &cobra.Command{Use: "task"}
	taskCmd.AddCommand(newOrchestrateTaskCreateCmd(deps))
	root.AddCommand(taskCmd)
	return root
}
```

- [ ] **Step 4: Add appcmd runner with JSON/stdout plumbing**

```go
func RunOrchestrate(args []string) error {
	opts, err := parseOrchestrateArgs(args)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(map[string]any{
		"ok":      false,
		"action":  opts.Action,
		"summary": "not implemented",
	})
}
```

- [ ] **Step 5: Run the focused command tests**

Run: `go test ./cmd/ai-flow ./internal/platform/appcmd -run 'TestOrchestrate|TestRootCommandShowsHelpWhenNoArgs' -count=1`

Expected: PASS for new wiring tests, with handler tests still reporting placeholder behavior where appropriate.

- [ ] **Step 6: Commit**

```bash
git add cmd/ai-flow/root.go cmd/ai-flow/orchestrate_cmd.go cmd/ai-flow/main_test.go internal/platform/appcmd/orchestrate.go internal/platform/appcmd/orchestrate_test.go
git commit -m "feat(cli): add orchestration command scaffold"
```

---

### Task 2: Create The Orchestration Application Service For Create / Follow-Up / Reassign

**Files:**
- Create: `internal/application/orchestrateapp/contracts.go`
- Create: `internal/application/orchestrateapp/errors.go`
- Create: `internal/application/orchestrateapp/service.go`
- Create: `internal/application/orchestrateapp/service_test.go`
- Modify: `internal/application/workitemapp/contracts.go`
- Modify: `internal/application/workitemapp/service.go`

- [ ] **Step 1: Write the failing create-idempotency and journal tests**

```go
func TestServiceCreateTaskReturnsExistingOpenWorkItemForSameDedupeKey(t *testing.T) {
	svc := newTestService(t)

	first, err := svc.CreateTask(context.Background(), CreateTaskInput{
		Title:     "CEO bootstrap",
		DedupeKey: "chat:42:goal:bootstrap",
	})
	if err != nil {
		t.Fatalf("CreateTask(first): %v", err)
	}

	second, err := svc.CreateTask(context.Background(), CreateTaskInput{
		Title:     "CEO bootstrap",
		DedupeKey: "chat:42:goal:bootstrap",
	})
	if err != nil {
		t.Fatalf("CreateTask(second): %v", err)
	}

	if second.WorkItem.ID != first.WorkItem.ID || second.Created {
		t.Fatalf("expected idempotent hit, got %+v", second)
	}
}

func TestServiceReassignAppendsCEOJournal(t *testing.T) {
	svc := newTestService(t)
	workItemID := seedWorkItem(t, svc.store, map[string]any{
		"ceo": map[string]any{"assigned_profile": "planner"},
	})

	result, err := svc.ReassignTask(context.Background(), ReassignTaskInput{
		WorkItemID:  workItemID,
		NewProfile:  "worker",
		Reason:      "planner stalled",
		ActorProfile: "ceo",
	})
	if err != nil {
		t.Fatalf("ReassignTask: %v", err)
	}
	if len(result.JournalEntries) != 1 {
		t.Fatalf("expected 1 journal entry, got %+v", result.JournalEntries)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/orchestrateapp -run 'TestService(CreateTaskReturnsExistingOpenWorkItemForSameDedupeKey|ReassignAppendsCEOJournal)' -count=1`

Expected: FAIL because the package and service do not exist yet.

- [ ] **Step 3: Define the service contracts and error surface**

```go
type Service struct {
	store           Store
	workItems       WorkItemService
	threads         ThreadService
	planner         PlannerService
	registry        core.AgentRegistry
}

type CreateTaskInput struct {
	Title               string
	Body                string
	ProjectID           *int64
	DedupeKey           string
	SourceChatSessionID string
	SourceGoalRef       string
}

type CreateTaskResult struct {
	WorkItem *core.WorkItem
	Created  bool
}
```

- [ ] **Step 4: Implement idempotent create, follow-up summary, and append-only journal writes**

```go
func appendCEOJournal(metadata map[string]any, entry map[string]any) map[string]any {
	out := cloneMetadata(metadata)
	journal, _ := out["ceo_journal"].([]any)
	out["ceo_journal"] = append(journal, entry)
	return out
}

func (s *Service) CreateTask(ctx context.Context, input CreateTaskInput) (*CreateTaskResult, error) {
	if existing := s.findMatchingOpenWorkItem(ctx, input.DedupeKey, input.SourceGoalRef); existing != nil {
		return &CreateTaskResult{WorkItem: existing, Created: false}, nil
	}
	workItem, err := s.workItems.CreateWorkItem(ctx, workitemapp.CreateWorkItemInput{
		ProjectID: input.ProjectID,
		Title:     input.Title,
		Body:      input.Body,
		Metadata: map[string]any{
			"ceo": map[string]any{
				"source_chat_session_id": input.SourceChatSessionID,
				"source_goal_ref":        input.SourceGoalRef,
				"dedupe_key":             input.DedupeKey,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return &CreateTaskResult{WorkItem: workItem, Created: true}, nil
}
```

- [ ] **Step 5: Run the orchestration service tests**

Run: `go test ./internal/application/orchestrateapp -count=1`

Expected: PASS for create/follow-up/reassign cases, with placeholder failures remaining for decompose/assign/escalate tests not implemented yet.

- [ ] **Step 6: Commit**

```bash
git add internal/application/orchestrateapp internal/application/workitemapp/contracts.go internal/application/workitemapp/service.go
git commit -m "feat(orchestrate): add task create follow-up and reassign service"
```

---

### Task 3: Implement Decompose Guardrails And Preferred-Profile Overrides

**Files:**
- Modify: `internal/application/orchestrateapp/service.go`
- Modify: `internal/application/orchestrateapp/service_test.go`
- Modify: `internal/application/planning/service.go`
- Modify: `internal/application/planning/service_test.go`
- Modify: `internal/application/flow/resolver.go`
- Modify: `internal/application/flow/engine_test.go`

- [ ] **Step 1: Write the failing decompose-conflict and preferred-profile tests**

```go
func TestServiceDecomposeRejectsOverwriteWhenActiveActionsExist(t *testing.T) {
	svc := newTestService(t)
	workItemID := seedWorkItemWithAction(t, svc.store, core.ActionRunning)

	_, err := svc.DecomposeTask(context.Background(), DecomposeTaskInput{
		WorkItemID:         workItemID,
		Objective:          "replan",
		OverwriteExisting:  true,
	})
	if CodeOf(err) != CodeDecomposeConflict {
		t.Fatalf("expected decompose conflict, got %v", err)
	}
}

func TestResolverPrefersConfiguredProfileOverride(t *testing.T) {
	reg := NewProfileRegistry([]*core.AgentProfile{
		{ID: "worker-a", Role: core.RoleWorker, Capabilities: []string{"backend"}},
		{ID: "worker-b", Role: core.RoleWorker, Capabilities: []string{"backend"}},
	})
	action := &core.Action{
		AgentRole: "worker",
		RequiredCapabilities: []string{"backend"},
		Config: map[string]any{"preferred_profile_id": "worker-b"},
	}

	got, err := reg.Resolve(context.Background(), action)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "worker-b" {
		t.Fatalf("preferred profile = %q, want worker-b", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/application/orchestrateapp ./internal/application/flow -run 'Test(ServiceDecomposeRejectsOverwriteWhenActiveActionsExist|ResolverPrefersConfiguredProfileOverride)' -count=1`

Expected: FAIL because overwrite conflicts and preferred-profile override are not implemented.

- [ ] **Step 3: Implement guarded decompose semantics**

```go
func (s *Service) DecomposeTask(ctx context.Context, input DecomposeTaskInput) (*DecomposeTaskResult, error) {
	actions, err := s.store.ListActionsByWorkItem(ctx, input.WorkItemID)
	if err != nil {
		return nil, err
	}
	if input.OverwriteExisting && hasStartedActions(actions) {
		return nil, newError(CodeDecomposeConflict, "cannot overwrite active or completed actions", nil)
	}
	// generate dag, optionally archive/delete only pending actions, then materialize
}
```

- [ ] **Step 4: Propagate and respect `preferred_profile_id`**

```go
func preferredProfileID(action *core.Action) string {
	if action == nil || action.Config == nil {
		return ""
	}
	raw, _ := action.Config["preferred_profile_id"].(string)
	return strings.TrimSpace(raw)
}

func (r *ProfileRegistry) Resolve(_ context.Context, action *core.Action) (string, error) {
	if preferred := preferredProfileID(action); preferred != "" {
		for _, p := range r.profiles {
			if p.ID == preferred {
				return p.ID, nil
			}
		}
	}
	// existing role/capability fallback
}
```

- [ ] **Step 5: Run the decompose and resolver test slices**

Run: `go test ./internal/application/orchestrateapp ./internal/application/planning ./internal/application/flow -run 'Test(ServiceDecompose|ResolverPrefersConfiguredProfileOverride)' -count=1`

Expected: PASS for overwrite conflict detection and preferred-profile resolution.

- [ ] **Step 6: Commit**

```bash
git add internal/application/orchestrateapp/service.go internal/application/orchestrateapp/service_test.go internal/application/planning/service.go internal/application/planning/service_test.go internal/application/flow/resolver.go internal/application/flow/engine_test.go
git commit -m "feat(orchestrate): add decompose guardrails and profile override"
```

---

### Task 4: Implement Thread Escalation And WorkItem Linking

**Files:**
- Modify: `internal/application/orchestrateapp/contracts.go`
- Modify: `internal/application/orchestrateapp/service.go`
- Modify: `internal/application/orchestrateapp/service_test.go`
- Modify: `internal/application/threadapp/service.go`
- Modify: `internal/application/threadapp/service_test.go`

- [ ] **Step 1: Write the failing escalation-idempotency test**

```go
func TestServiceEscalateThreadReturnsExistingActiveThreadLink(t *testing.T) {
	svc := newTestService(t)
	workItemID, threadID := seedLinkedThread(t, svc.store)

	result, err := svc.EscalateThread(context.Background(), EscalateThreadInput{
		WorkItemID:   workItemID,
		Reason:       "needs coordination",
		ThreadTitle:  "CEO escalation",
	})
	if err != nil {
		t.Fatalf("EscalateThread: %v", err)
	}
	if result.Thread.ID != threadID || result.Created {
		t.Fatalf("expected reuse of existing thread, got %+v", result)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/orchestrateapp -run TestServiceEscalateThreadReturnsExistingActiveThreadLink -count=1`

Expected: FAIL because escalation lookup/reuse logic does not exist.

- [ ] **Step 3: Implement escalation lookup, thread creation, and link creation**

```go
func (s *Service) EscalateThread(ctx context.Context, input EscalateThreadInput) (*EscalateThreadResult, error) {
	if existing := s.findActiveEscalationThread(ctx, input.WorkItemID); existing != nil && !input.ForceNew {
		return &EscalateThreadResult{Thread: existing, Created: false}, nil
	}
	thread, err := s.threads.CreateThread(ctx, threadapp.CreateThreadInput{
		Title:   input.ThreadTitle,
		OwnerID: input.ActorProfile,
		Metadata: map[string]any{
			"source_work_item_id": input.WorkItemID,
			"source_type":         "ceo_escalation",
			"escalation_reason":   input.Reason,
		},
	})
	if err != nil {
		return nil, err
	}
	_, err = s.threads.LinkThreadWorkItem(ctx, threadapp.LinkThreadWorkItemInput{
		ThreadID:     thread.Thread.ID,
		WorkItemID:   input.WorkItemID,
		RelationType: "drives",
		IsPrimary:    true,
	})
	if err != nil {
		return nil, err
	}
	return &EscalateThreadResult{Thread: thread.Thread, Created: true}, nil
}
```

- [ ] **Step 4: Add human-invite semantics guard in tests**

```go
func TestServiceEscalateThreadTreatsInviteHumansAsMeetingParticipantsOnly(t *testing.T) {
	// assert thread participants contain users, but no work-item assignee metadata is mutated for humans
}
```

- [ ] **Step 5: Run thread/escalation test slices**

Run: `go test ./internal/application/orchestrateapp ./internal/application/threadapp -run 'TestServiceEscalateThread|TestServiceCreateWorkItemFromThread' -count=1`

Expected: PASS for escalation reuse, new-thread creation, and link persistence.

- [ ] **Step 6: Commit**

```bash
git add internal/application/orchestrateapp/contracts.go internal/application/orchestrateapp/service.go internal/application/orchestrateapp/service_test.go internal/application/threadapp/service.go internal/application/threadapp/service_test.go
git commit -m "feat(orchestrate): add thread escalation flow"
```

---

### Task 5: Add The CEO Profile, Prompt Template, And Builtin Skill

**Files:**
- Create: `configs/prompts/ceo_orchestrator.tmpl`
- Create: `internal/skills/builtin/ceo-manage/SKILL.md`
- Create: `internal/skills/builtin/ceo-manage/agents/openai.yaml`
- Modify: `internal/platform/config/defaults.toml`
- Modify: `internal/skills/builtin_test.go`

- [ ] **Step 1: Write the failing config and builtin extraction tests**

```go
func TestDefaultConfigIncludesCEOProfile(t *testing.T) {
	cfg := loadDefaultConfigForTest(t)
	profile := findProfile(t, cfg.Runtime.Agents.Profiles, "ceo")
	if profile.PromptTemplate != "ceo_orchestrator" {
		t.Fatalf("prompt template = %q, want ceo_orchestrator", profile.PromptTemplate)
	}
	if !slices.Contains(profile.Skills, "ceo-manage") {
		t.Fatalf("expected ceo-manage skill, got %+v", profile.Skills)
	}
}

func TestEnsureBuiltinSkillsExtractsCEOManage(t *testing.T) {
	skillsRoot := t.TempDir()
	if err := EnsureBuiltinSkills(skillsRoot); err != nil {
		t.Fatalf("EnsureBuiltinSkills: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillsRoot, "ceo-manage", "SKILL.md")); err != nil {
		t.Fatalf("ceo-manage skill missing: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/platform/config ./internal/skills -run 'Test(DefaultConfigIncludesCEOProfile|EnsureBuiltinSkillsExtractsCEOManage)' -count=1`

Expected: FAIL because the CEO profile and builtin skill do not exist.

- [ ] **Step 3: Add the default CEO profile and prompt template**

```toml
[[runtime.agents.profiles]]
id = "ceo"
name = "CEO Orchestrator"
driver = "claude-acp"
llm_config_id = "system"
role = "lead"
capabilities = ["planning", "coordination", "review"]
skills = ["ceo-manage", "plan-actions", "sys-action-manage"]
prompt_template = "ceo_orchestrator"
  [runtime.agents.profiles.session]
  reuse = true
  max_turns = 40
  idle_ttl = "20m"
```

- [ ] **Step 4: Add the builtin CEO skill body and metadata**

```md
---
name: ceo-manage
description: Task-first orchestration for the CEO chat profile. Use when the agent must turn a user goal into work items, decompose work, assign profiles, follow up, reassign, and escalate to threads only when complexity requires coordination.
---

# CEO Manage

1. Default to task-first orchestration.
2. Use `orchestrate task create` before escalating.
3. Use `orchestrate task follow-up` before `orchestrate task reassign`.
4. Escalate to thread only for coordination blockers, conflicting dependencies, or repeated stalls.
```

- [ ] **Step 5: Run the profile/skill tests**

Run: `go test ./internal/platform/config ./internal/skills -count=1`

Expected: PASS, with the CEO profile discoverable and the builtin skill extracted.

- [ ] **Step 6: Commit**

```bash
git add configs/prompts/ceo_orchestrator.tmpl internal/skills/builtin/ceo-manage internal/platform/config/defaults.toml internal/skills/builtin_test.go
git commit -m "feat(config): add CEO orchestration profile and skill"
```

---

### Task 6: Wire The CLI To The Orchestration Service And Add End-To-End Smoke Coverage

**Files:**
- Modify: `internal/platform/appcmd/orchestrate.go`
- Modify: `internal/platform/appcmd/orchestrate_test.go`
- Modify: `internal/application/orchestrateapp/service.go`
- Modify: `cmd/ai-flow/main_test.go`
- Create: `internal/application/orchestrateapp/cli_smoke_test.go`

- [ ] **Step 1: Write a failing CLI smoke test for `task create -> follow-up`**

```go
func TestRunOrchestrateTaskCreateThenFollowUp(t *testing.T) {
	runtime := newOrchestrateRuntimeForTest(t)

	createOut := runCLI(t, runtime, "task", "create", "--title", "CEO smoke", "--dedupe-key", "chat:smoke")
	createResp := decodeJSON(t, createOut)
	workItemID := int64(createResp["work_item_id"].(float64))

	followOut := runCLI(t, runtime, "task", "follow-up", "--work-item-id", strconv.FormatInt(workItemID, 10))
	followResp := decodeJSON(t, followOut)

	if followResp["work_item_id"] != float64(workItemID) {
		t.Fatalf("unexpected follow-up response: %+v", followResp)
	}
}
```

- [ ] **Step 2: Run the smoke test to verify it fails**

Run: `go test ./internal/platform/appcmd ./internal/application/orchestrateapp -run TestRunOrchestrateTaskCreateThenFollowUp -count=1`

Expected: FAIL because the appcmd runner is not yet instantiating the orchestration service.

- [ ] **Step 3: Instantiate the orchestration service in appcmd and connect subcommands**

```go
func RunOrchestrate(args []string) error {
	cfg, dataDir, _, err := LoadConfig()
	if err != nil {
		return err
	}
	store, registry, runtimeManager, cleanup, _ := bootstrap.Build(...)
	defer cleanup()

	svc := orchestrateapp.New(orchestrateapp.Config{
		Store:    store,
		Registry: registry,
		Planner:  planning.NewService(...),
	})
	return runOrchestrateAction(os.Stdout, svc, args)
}
```

- [ ] **Step 4: Run the CLI and application smoke slices**

Run: `go test ./cmd/ai-flow ./internal/platform/appcmd ./internal/application/orchestrateapp -count=1`

Expected: PASS for command wiring, JSON output, and create/follow-up smoke coverage.

- [ ] **Step 5: Run the broader targeted regression suite**

Run: `pwsh -NoProfile -File .\scripts\test\backend-unit.ps1`

Expected: PASS for backend unit coverage within the default timeout budget; if a subset is needed during iteration, at minimum keep `go test ./cmd/ai-flow ./internal/application/orchestrateapp ./internal/application/flow ./internal/application/planning ./internal/application/threadapp ./internal/platform/appcmd ./internal/platform/config ./internal/skills -count=1` green.

- [ ] **Step 6: Commit**

```bash
git add cmd/ai-flow/main_test.go internal/platform/appcmd/orchestrate.go internal/platform/appcmd/orchestrate_test.go internal/application/orchestrateapp/service.go internal/application/orchestrateapp/cli_smoke_test.go
git commit -m "feat(orchestrate): wire CEO orchestration CLI end to end"
```

---

## Final Verification Checklist

- [ ] `ai-flow orchestrate task create --title "CEO bootstrap" --dedupe-key "chat:42:goal:bootstrap" --json` returns stable JSON
- [ ] repeated `task create` with same dedupe key returns the same open work item with `"created": false`
- [ ] `task decompose --overwrite-existing` rejects active/completed actions with a conflict error
- [ ] `task assign-profile --profile worker` stores `WorkItem.metadata.ceo.assigned_profile` and propagates `preferred_profile_id` to pending execution actions
- [ ] `task follow-up` returns a useful summary for CEO chat replies
- [ ] `task reassign` appends to `ceo_journal` instead of overwriting prior entries
- [ ] `task escalate-thread` reuses an active linked thread unless `force-new` is supplied
- [ ] default config exposes a `ceo` profile with `ceo-manage` skill and `ceo_orchestrator` prompt template

---

## Notes For The Implementer

- Keep the orchestration control surface narrow. Do not add proposal/initiative management commands in this MVP.
- Reuse existing store/services instead of wrapping HTTP with `curl`.
- Prefer metadata append semantics over schema expansion unless a test demonstrates the current metadata approach is insufficient.
- Preserve current `Thread` behavior: escalation creates or reuses collaboration space, but does not become the default path for simple tasks.
