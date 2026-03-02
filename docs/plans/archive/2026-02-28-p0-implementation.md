# P0 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a locally usable AI workflow orchestrator that chains Claude Code, Codex CLI, and OpenSpec into automated pipelines with TUI interface.

**Architecture:** Plugin-based engine with 9 interface slots. P0 implements one concrete per slot (process runtime, worktree workspace, SQLite store). Pipeline Engine is a checkpoint-based state machine that drives Agent plugins through Stage sequences. EventBus (Go channels) decouples all components.

**Tech Stack:** Go 1.22+, Bubble Tea (TUI), SQLite via modernc.org/sqlite, gopkg.in/yaml.v3, slog, os/exec for git/agent CLIs.

**P0 Scope:** Core types + EventBus + Config + SQLite Store + Git ops + Claude/Codex Agent drivers + Process Runtime + Pipeline Executor + Simplified Reactions + CLI commands + TUI.

**Out of Scope:** Scheduler (P1), GitHub integration (P2), Web Dashboard (P3), Notifier/custom templates (P4), factory/dynamic plugin loading (P1).

---

## Task 1: Project Foundation

**Files:**
- Create: `go.mod`
- Create: `cmd/ai-flow/main.go`
- Create: `internal/core/doc.go`

**Step 1: Initialize Go module**

Run: `cd D:/project/ai-workflow && go mod init github.com/user/ai-workflow`
Expected: `go.mod` created with Go 1.22+

**Step 2: Create directory skeleton**

```bash
mkdir -p cmd/ai-flow cmd/server
mkdir -p internal/{core,engine,plugins/{agent-claude,agent-codex,runtime-process,workspace-worktree,store-sqlite},git,config,eventbus,tui}
mkdir -p configs/prompts
```

**Step 3: Create minimal main.go**

```go
// cmd/ai-flow/main.go
package main

import (
    "fmt"
    "os"
)

func main() {
    if err := run(); err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }
}

func run() error {
    fmt.Println("ai-flow v0.1.0-dev")
    return nil
}
```

**Step 4: Verify build**

Run: `go build ./cmd/ai-flow/`
Expected: Binary compiles without errors

**Step 5: Commit**

```bash
git init && git add -A
git commit -m "chore: initialize Go project skeleton"
```

---

## Task 2: Core Domain Types

**Files:**
- Create: `internal/core/plugin.go`
- Create: `internal/core/stage.go`
- Create: `internal/core/pipeline.go`
- Create: `internal/core/project.go`
- Create: `internal/core/events.go`
- Test: `internal/core/pipeline_test.go`

**Step 1: Write test for Pipeline state transitions**

```go
// internal/core/pipeline_test.go
package core

import "testing"

func TestValidateTransition(t *testing.T) {
    valid := []struct{ from, to PipelineStatus }{
        {StatusCreated, StatusRunning},
        {StatusCreated, StatusAborted},
        {StatusRunning, StatusWaitingHuman},
        {StatusRunning, StatusPaused},
        {StatusRunning, StatusFailed},
        {StatusRunning, StatusDone},
        {StatusPaused, StatusRunning},
        {StatusPaused, StatusAborted},
        {StatusWaitingHuman, StatusRunning},
        {StatusWaitingHuman, StatusAborted},
        {StatusFailed, StatusRunning},
    }
    for _, tt := range valid {
        if err := ValidateTransition(tt.from, tt.to); err != nil {
            t.Errorf("expected valid: %s -> %s, got err: %v", tt.from, tt.to, err)
        }
    }

    invalid := []struct{ from, to PipelineStatus }{
        {StatusCreated, StatusDone},
        {StatusDone, StatusRunning},
        {StatusAborted, StatusRunning},
        {StatusFailed, StatusDone},
    }
    for _, tt := range invalid {
        if err := ValidateTransition(tt.from, tt.to); err == nil {
            t.Errorf("expected invalid: %s -> %s, got nil", tt.from, tt.to)
        }
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestValidateTransition -v`
Expected: FAIL — types not defined

**Step 3: Implement Plugin interfaces**

```go
// internal/core/plugin.go
package core

import "context"

type PluginSlot string

const (
    SlotAgent     PluginSlot = "agent"
    SlotRuntime   PluginSlot = "runtime"
    SlotWorkspace PluginSlot = "workspace"
    SlotSpec      PluginSlot = "spec"
    SlotTracker   PluginSlot = "tracker"
    SlotSCM       PluginSlot = "scm"
    SlotNotifier  PluginSlot = "notifier"
    SlotStore     PluginSlot = "store"
    SlotTerminal  PluginSlot = "terminal"
)

type Plugin interface {
    Name() string
    Init(ctx context.Context) error
    Close() error
}
```

**Step 4: Implement Stage types**

```go
// internal/core/stage.go
package core

import "time"

type StageID string

const (
    StageRequirements  StageID = "requirements"
    StagePlanDraft     StageID = "plan_draft"
    StagePlanReview    StageID = "plan_review"
    StageWorktreeSetup StageID = "worktree_setup"
    StageImplement     StageID = "implement"
    StageCodeReview    StageID = "code_review"
    StageFixup         StageID = "fixup"
    StageE2ETest       StageID = "e2e_test"
    StageMerge         StageID = "merge"
    StageCleanup       StageID = "cleanup"
)

type OnFailure string

const (
    OnFailureRetry OnFailure = "retry"
    OnFailureHuman OnFailure = "human"
    OnFailureSkip  OnFailure = "skip"
    OnFailureAbort OnFailure = "abort"
)

type StageConfig struct {
    Name           StageID       `yaml:"name" json:"name"`
    Agent          string        `yaml:"agent" json:"agent"`
    PromptTemplate string        `yaml:"prompt_template" json:"prompt_template"`
    Timeout        time.Duration `yaml:"timeout" json:"timeout"`
    MaxRetries     int           `yaml:"max_retries" json:"max_retries"`
    RequireHuman   bool          `yaml:"require_human" json:"require_human"`
    OnFailure      OnFailure     `yaml:"on_failure" json:"on_failure"`
}

type CheckpointStatus string

const (
    CheckpointInProgress  CheckpointStatus = "in_progress"
    CheckpointSuccess     CheckpointStatus = "success"
    CheckpointFailed      CheckpointStatus = "failed"
    CheckpointSkipped     CheckpointStatus = "skipped"
    CheckpointInvalidated CheckpointStatus = "invalidated"
)

type Checkpoint struct {
    PipelineID string            `json:"pipeline_id"`
    StageName  StageID           `json:"stage_name"`
    Status     CheckpointStatus  `json:"status"`
    Artifacts  map[string]string `json:"artifacts"`
    StartedAt  time.Time         `json:"started_at"`
    FinishedAt time.Time         `json:"finished_at"`
    AgentUsed  string            `json:"agent_used"`
    TokensUsed int               `json:"tokens_used"`
    RetryCount int               `json:"retry_count"`
    Error      string            `json:"error,omitempty"`
}
```

**Step 5: Implement Pipeline types + ValidateTransition**

```go
// internal/core/pipeline.go
package core

import (
    "fmt"
    "time"
)

type PipelineStatus string

const (
    StatusCreated      PipelineStatus = "created"
    StatusRunning      PipelineStatus = "running"
    StatusWaitingHuman PipelineStatus = "waiting_human"
    StatusPaused       PipelineStatus = "paused"
    StatusDone         PipelineStatus = "done"
    StatusFailed       PipelineStatus = "failed"
    StatusAborted      PipelineStatus = "aborted"
)

var validTransitions = map[PipelineStatus][]PipelineStatus{
    StatusCreated:      {StatusRunning, StatusAborted},
    StatusRunning:      {StatusWaitingHuman, StatusPaused, StatusFailed, StatusDone},
    StatusPaused:       {StatusRunning, StatusAborted},
    StatusWaitingHuman: {StatusRunning, StatusAborted},
    StatusFailed:       {StatusRunning},
}

func ValidateTransition(from, to PipelineStatus) error {
    targets, ok := validTransitions[from]
    if !ok {
        return fmt.Errorf("no transitions from %s", from)
    }
    for _, t := range targets {
        if t == to {
            return nil
        }
    }
    return fmt.Errorf("invalid transition: %s -> %s", from, to)
}

type Pipeline struct {
    ID              string            `json:"id"`
    ProjectID       string            `json:"project_id"`
    Name            string            `json:"name"`
    Description     string            `json:"description"`
    Template        string            `json:"template"`
    Status          PipelineStatus    `json:"status"`
    CurrentStage    StageID           `json:"current_stage"`
    Stages          []StageConfig     `json:"stages"`
    Artifacts       map[string]string `json:"artifacts"`
    Config          map[string]any    `json:"config"`
    BranchName      string            `json:"branch_name"`
    WorktreePath    string            `json:"worktree_path"`
    ErrorMessage    string            `json:"error_message,omitempty"`
    MaxTotalRetries int               `json:"max_total_retries"`
    TotalRetries    int               `json:"total_retries"`
    StartedAt       time.Time         `json:"started_at"`
    FinishedAt      time.Time         `json:"finished_at"`
    CreatedAt       time.Time         `json:"created_at"`
    UpdatedAt       time.Time         `json:"updated_at"`
}
```

**Step 6: Implement Project and Event types**

```go
// internal/core/project.go
package core

import "time"

type Project struct {
    ID          string    `json:"id"`
    Name        string    `json:"name"`
    RepoPath    string    `json:"repo_path"`
    GitHubOwner string    `json:"github_owner,omitempty"`
    GitHubRepo  string    `json:"github_repo,omitempty"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

```go
// internal/core/events.go
package core

import "time"

type EventType string

const (
    EventStageStart    EventType = "stage_start"
    EventStageComplete EventType = "stage_complete"
    EventStageFailed   EventType = "stage_failed"
    EventHumanRequired EventType = "human_required"
    EventPipelineDone  EventType = "pipeline_done"
    EventPipelineFailed EventType = "pipeline_failed"
    EventAgentOutput   EventType = "agent_output"
    EventPipelineStuck EventType = "pipeline_stuck"
)

type Event struct {
    Type       EventType         `json:"type"`
    PipelineID string            `json:"pipeline_id"`
    ProjectID  string            `json:"project_id"`
    Stage      StageID           `json:"stage,omitempty"`
    Agent      string            `json:"agent,omitempty"`
    Data       map[string]string `json:"data,omitempty"`
    Error      string            `json:"error,omitempty"`
    Timestamp  time.Time         `json:"timestamp"`
}
```

**Step 7: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestValidateTransition -v`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/core/
git commit -m "feat: add core domain types — Pipeline, Stage, Checkpoint, Project, Events, Plugin interfaces"
```

---

## Task 3: Event Bus

**Files:**
- Create: `internal/eventbus/bus.go`
- Test: `internal/eventbus/bus_test.go`

**Step 1: Write failing test**

```go
// internal/eventbus/bus_test.go
package eventbus

import (
    "testing"
    "time"

    "github.com/user/ai-workflow/internal/core"
)

func TestBusPubSub(t *testing.T) {
    bus := New()
    defer bus.Close()

    ch := bus.Subscribe()
    defer bus.Unsubscribe(ch)

    evt := core.Event{Type: core.EventStageStart, PipelineID: "p1", Timestamp: time.Now()}
    bus.Publish(evt)

    select {
    case got := <-ch:
        if got.PipelineID != "p1" {
            t.Fatalf("expected p1, got %s", got.PipelineID)
        }
    case <-time.After(time.Second):
        t.Fatal("timeout waiting for event")
    }
}

func TestBusMultipleSubscribers(t *testing.T) {
    bus := New()
    defer bus.Close()

    ch1 := bus.Subscribe()
    ch2 := bus.Subscribe()

    evt := core.Event{Type: core.EventAgentOutput, PipelineID: "p2", Timestamp: time.Now()}
    bus.Publish(evt)

    for _, ch := range []<-chan core.Event{ch1, ch2} {
        select {
        case got := <-ch:
            if got.PipelineID != "p2" {
                t.Fatalf("expected p2, got %s", got.PipelineID)
            }
        case <-time.After(time.Second):
            t.Fatal("timeout")
        }
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/eventbus/ -v`
Expected: FAIL — package not found

**Step 3: Implement EventBus**

```go
// internal/eventbus/bus.go
package eventbus

import (
    "sync"

    "github.com/user/ai-workflow/internal/core"
)

type Bus struct {
    mu   sync.RWMutex
    subs map[chan core.Event]struct{}
}

func New() *Bus {
    return &Bus{subs: make(map[chan core.Event]struct{})}
}

func (b *Bus) Subscribe() <-chan core.Event {
    ch := make(chan core.Event, 64)
    b.mu.Lock()
    b.subs[ch] = struct{}{}
    b.mu.Unlock()
    return ch
}

func (b *Bus) Unsubscribe(ch <-chan core.Event) {
    c := ch.(chan core.Event) // safe: we created it
    b.mu.Lock()
    delete(b.subs, c)
    b.mu.Unlock()
    close(c)
}

func (b *Bus) Publish(evt core.Event) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    for ch := range b.subs {
        select {
        case ch <- evt:
        default: // drop if subscriber is slow
        }
    }
}

func (b *Bus) Close() {
    b.mu.Lock()
    defer b.mu.Unlock()
    for ch := range b.subs {
        close(ch)
        delete(b.subs, ch)
    }
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/eventbus/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/eventbus/
git commit -m "feat: add EventBus — channel-based pub/sub for core events"
```

---

## Task 4: Config System

**Files:**
- Create: `internal/config/types.go`
- Create: `internal/config/loader.go`
- Create: `internal/config/merge.go`
- Create: `internal/config/defaults.go`
- Create: `configs/defaults.yaml`
- Test: `internal/config/config_test.go`

**Step 1: Write failing test for config merge**

```go
// internal/config/config_test.go
package config

import "testing"

func TestMergeAgentConfig(t *testing.T) {
    global := &AgentConfig{Binary: ptr("claude"), MaxTurns: ptr(30)}
    project := &AgentConfig{MaxTurns: ptr(50)} // override turns only

    merged := MergeAgentConfig(global, project)

    if *merged.Binary != "claude" {
        t.Errorf("expected binary claude, got %s", *merged.Binary)
    }
    if *merged.MaxTurns != 50 {
        t.Errorf("expected max_turns 50, got %d", *merged.MaxTurns)
    }
}

func TestLoadDefaults(t *testing.T) {
    cfg := Defaults()
    if cfg.Pipeline.DefaultTemplate != "standard" {
        t.Errorf("expected default template standard, got %s", cfg.Pipeline.DefaultTemplate)
    }
    if cfg.Scheduler.MaxGlobalAgents != 3 {
        t.Errorf("expected max_global_agents 3, got %d", cfg.Scheduler.MaxGlobalAgents)
    }
}

func ptr[T any](v T) *T { return &v }
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL

**Step 3: Implement config types with pointer fields**

```go
// internal/config/types.go
package config

import "time"

type Config struct {
    Agents    AgentsConfig    `yaml:"agents"`
    Pipeline  PipelineConfig  `yaml:"pipeline"`
    Scheduler SchedulerConfig `yaml:"scheduler"`
    Store     StoreConfig     `yaml:"store"`
    Log       LogConfig       `yaml:"log"`
}

type AgentsConfig struct {
    Claude   *AgentConfig `yaml:"claude"`
    Codex    *AgentConfig `yaml:"codex"`
    OpenSpec *AgentConfig `yaml:"openspec"`
}

type AgentConfig struct {
    Binary    *string `yaml:"binary"`
    MaxTurns  *int    `yaml:"default_max_turns"`
    Model     *string `yaml:"model"`
    Reasoning *string `yaml:"reasoning"`
    Sandbox   *string `yaml:"sandbox"`
    Approval  *string `yaml:"approval"`
}

type PipelineConfig struct {
    DefaultTemplate   string        `yaml:"default_template"`
    GlobalTimeout     time.Duration `yaml:"global_timeout"`
    AutoInferTemplate bool          `yaml:"auto_infer_template"`
    MaxTotalRetries   int           `yaml:"max_total_retries"`
}

type SchedulerConfig struct {
    MaxGlobalAgents     int `yaml:"max_global_agents"`
    MaxProjectPipelines int `yaml:"max_project_pipelines"`
}

type StoreConfig struct {
    Driver string `yaml:"driver"`
    Path   string `yaml:"path"`
}

type LogConfig struct {
    Level      string `yaml:"level"`
    File       string `yaml:"file"`
    MaxSizeMB  int    `yaml:"max_size_mb"`
    MaxAgeDays int    `yaml:"max_age_days"`
}
```

**Step 4: Implement merge functions**

```go
// internal/config/merge.go
package config

func MergeAgentConfig(base, override *AgentConfig) *AgentConfig {
    if override == nil {
        return base
    }
    if base == nil {
        return override
    }
    out := *base
    if override.Binary != nil {
        out.Binary = override.Binary
    }
    if override.MaxTurns != nil {
        out.MaxTurns = override.MaxTurns
    }
    if override.Model != nil {
        out.Model = override.Model
    }
    if override.Reasoning != nil {
        out.Reasoning = override.Reasoning
    }
    if override.Sandbox != nil {
        out.Sandbox = override.Sandbox
    }
    if override.Approval != nil {
        out.Approval = override.Approval
    }
    return &out
}
```

**Step 5: Implement defaults**

```go
// internal/config/defaults.go
package config

import "time"

func Defaults() Config {
    return Config{
        Agents: AgentsConfig{
            Claude: &AgentConfig{
                Binary:   ptr("claude"),
                MaxTurns: ptr(30),
            },
            Codex: &AgentConfig{
                Binary:    ptr("codex"),
                Model:     ptr("gpt-5.3-codex"),
                Reasoning: ptr("high"),
                Sandbox:   ptr("workspace-write"),
                Approval:  ptr("never"),
            },
            OpenSpec: &AgentConfig{
                Binary: ptr("openspec"),
            },
        },
        Pipeline: PipelineConfig{
            DefaultTemplate:   "standard",
            GlobalTimeout:     2 * time.Hour,
            AutoInferTemplate: true,
            MaxTotalRetries:   5,
        },
        Scheduler: SchedulerConfig{
            MaxGlobalAgents:     3,
            MaxProjectPipelines: 2,
        },
        Store: StoreConfig{
            Driver: "sqlite",
            Path:   "~/.ai-workflow/data.db",
        },
        Log: LogConfig{
            Level:      "info",
            File:       "~/.ai-workflow/logs/app.log",
            MaxSizeMB:  100,
            MaxAgeDays: 30,
        },
    }
}

func ptr[T any](v T) *T { return &v }
```

**Step 6: Implement YAML loader**

```go
// internal/config/loader.go
package config

import (
    "os"

    "gopkg.in/yaml.v3"
)

func LoadFile(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    cfg := Defaults()
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, err
    }
    return &cfg, nil
}
```

**Step 7: Run tests**

Run: `go test ./internal/config/ -v`
Expected: PASS

**Step 8: Add yaml.v3 dependency and verify build**

Run: `go get gopkg.in/yaml.v3 && go test ./internal/config/ -v`
Expected: PASS

**Step 9: Commit**

```bash
git add internal/config/ configs/ go.mod go.sum
git commit -m "feat: add config system — types, YAML loader, merge with pointer fields, defaults"
```

---

## Task 5: SQLite Store

**Files:**
- Create: `internal/plugins/store-sqlite/store.go`
- Create: `internal/plugins/store-sqlite/migrations.go`
- Create: `internal/core/store.go`
- Test: `internal/plugins/store-sqlite/store_test.go`

**Step 1: Define Store interface in core**

```go
// internal/core/store.go
package core

type ProjectFilter struct {
    NameContains string
}

type PipelineFilter struct {
    Status string
    Limit  int
    Offset int
}

type LogEntry struct {
    ID         int64  `json:"id"`
    PipelineID string `json:"pipeline_id"`
    Stage      string `json:"stage"`
    Type       string `json:"type"`
    Agent      string `json:"agent"`
    Content    string `json:"content"`
    Timestamp  string `json:"timestamp"`
}

type HumanAction struct {
    ID         int64  `json:"id"`
    PipelineID string `json:"pipeline_id"`
    Stage      string `json:"stage"`
    Action     string `json:"action"`
    Message    string `json:"message"`
    Source     string `json:"source"`
    UserID     string `json:"user_id"`
    CreatedAt  string `json:"created_at"`
}

type Store interface {
    ListProjects(filter ProjectFilter) ([]Project, error)
    GetProject(id string) (*Project, error)
    CreateProject(p *Project) error
    UpdateProject(p *Project) error
    DeleteProject(id string) error

    ListPipelines(projectID string, filter PipelineFilter) ([]Pipeline, error)
    GetPipeline(id string) (*Pipeline, error)
    SavePipeline(p *Pipeline) error
    GetActivePipelines() ([]Pipeline, error)

    SaveCheckpoint(cp *Checkpoint) error
    GetCheckpoints(pipelineID string) ([]Checkpoint, error)
    GetLastSuccessCheckpoint(pipelineID string) (*Checkpoint, error)

    AppendLog(entry LogEntry) error
    GetLogs(pipelineID string, stage string, limit int, offset int) ([]LogEntry, int, error)

    RecordAction(action HumanAction) error
    GetActions(pipelineID string) ([]HumanAction, error)

    Close() error
}
```

**Step 2: Write failing test for SQLite store**

```go
// internal/plugins/store-sqlite/store_test.go
package storesqlite

import (
    "testing"

    "github.com/user/ai-workflow/internal/core"
)

func TestProjectCRUD(t *testing.T) {
    s, err := New(":memory:")
    if err != nil {
        t.Fatal(err)
    }
    defer s.Close()

    p := &core.Project{ID: "test-1", Name: "Test", RepoPath: "/tmp/test"}
    if err := s.CreateProject(p); err != nil {
        t.Fatal(err)
    }

    got, err := s.GetProject("test-1")
    if err != nil {
        t.Fatal(err)
    }
    if got.Name != "Test" {
        t.Errorf("expected Test, got %s", got.Name)
    }

    got.Name = "Updated"
    if err := s.UpdateProject(got); err != nil {
        t.Fatal(err)
    }

    got2, _ := s.GetProject("test-1")
    if got2.Name != "Updated" {
        t.Errorf("expected Updated, got %s", got2.Name)
    }

    if err := s.DeleteProject("test-1"); err != nil {
        t.Fatal(err)
    }
    _, err = s.GetProject("test-1")
    if err == nil {
        t.Error("expected error after delete")
    }
}

func TestPipelineSaveAndGet(t *testing.T) {
    s, err := New(":memory:")
    if err != nil {
        t.Fatal(err)
    }
    defer s.Close()

    s.CreateProject(&core.Project{ID: "proj-1", Name: "P", RepoPath: "/tmp/p"})

    pipe := &core.Pipeline{
        ID: "20260228-aabbccddeeff", ProjectID: "proj-1",
        Name: "test-pipe", Template: "standard",
        Status: core.StatusCreated,
        Stages: []core.StageConfig{{Name: core.StageImplement, Agent: "claude"}},
        Artifacts: map[string]string{},
        MaxTotalRetries: 5,
    }
    if err := s.SavePipeline(pipe); err != nil {
        t.Fatal(err)
    }

    got, err := s.GetPipeline("20260228-aabbccddeeff")
    if err != nil {
        t.Fatal(err)
    }
    if got.Template != "standard" {
        t.Errorf("expected standard, got %s", got.Template)
    }
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/plugins/store-sqlite/ -v`
Expected: FAIL

**Step 4: Implement SQLite store — migrations**

```go
// internal/plugins/store-sqlite/migrations.go
package storesqlite

const schema = `
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    repo_path   TEXT NOT NULL UNIQUE,
    github_owner TEXT,
    github_repo  TEXT,
    config_json TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS pipelines (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    name        TEXT NOT NULL,
    description TEXT,
    template    TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'created',
    current_stage TEXT,
    stages_json TEXT NOT NULL,
    artifacts_json TEXT DEFAULT '{}',
    config_json TEXT DEFAULT '{}',
    issue_number INTEGER,
    pr_number   INTEGER,
    branch_name TEXT,
    worktree_path TEXT,
    error_message TEXT,
    max_total_retries INTEGER DEFAULT 5,
    total_retries INTEGER DEFAULT 0,
    started_at  DATETIME,
    finished_at DATETIME,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pipelines_project ON pipelines(project_id);
CREATE INDEX IF NOT EXISTS idx_pipelines_status ON pipelines(status);

CREATE TABLE IF NOT EXISTS checkpoints (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id TEXT NOT NULL REFERENCES pipelines(id),
    stage       TEXT NOT NULL,
    status      TEXT NOT NULL,
    agent_used  TEXT,
    artifacts_json TEXT DEFAULT '{}',
    tokens_used INTEGER DEFAULT 0,
    retry_count INTEGER DEFAULT 0,
    error_message TEXT,
    started_at  DATETIME NOT NULL,
    finished_at DATETIME,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_checkpoints_pipeline ON checkpoints(pipeline_id);

CREATE TABLE IF NOT EXISTS logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id TEXT NOT NULL REFERENCES pipelines(id),
    stage       TEXT NOT NULL,
    type        TEXT NOT NULL,
    agent       TEXT,
    content     TEXT NOT NULL,
    timestamp   DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_logs_pipeline_stage ON logs(pipeline_id, stage);
CREATE INDEX IF NOT EXISTS idx_logs_id ON logs(id);

CREATE TABLE IF NOT EXISTS human_actions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id TEXT NOT NULL REFERENCES pipelines(id),
    stage       TEXT NOT NULL,
    action      TEXT NOT NULL,
    message     TEXT,
    source      TEXT NOT NULL,
    user_id     TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_human_actions_pipeline ON human_actions(pipeline_id);
`
```

**Step 5: Implement SQLite store — core CRUD**

```go
// internal/plugins/store-sqlite/store.go
package storesqlite

import (
    "database/sql"
    "encoding/json"
    "fmt"

    "github.com/user/ai-workflow/internal/core"
    _ "modernc.org/sqlite"
)

type SQLiteStore struct {
    db *sql.DB
}

func New(path string) (*SQLiteStore, error) {
    db, err := sql.Open("sqlite", path)
    if err != nil {
        return nil, fmt.Errorf("open db: %w", err)
    }
    if _, err := db.Exec(schema); err != nil {
        db.Close()
        return nil, fmt.Errorf("init schema: %w", err)
    }
    return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) CreateProject(p *core.Project) error {
    _, err := s.db.Exec(
        `INSERT INTO projects (id, name, repo_path, github_owner, github_repo) VALUES (?,?,?,?,?)`,
        p.ID, p.Name, p.RepoPath, p.GitHubOwner, p.GitHubRepo,
    )
    return err
}

func (s *SQLiteStore) GetProject(id string) (*core.Project, error) {
    p := &core.Project{}
    err := s.db.QueryRow(`SELECT id, name, repo_path, github_owner, github_repo, created_at, updated_at FROM projects WHERE id=?`, id).
        Scan(&p.ID, &p.Name, &p.RepoPath, &p.GitHubOwner, &p.GitHubRepo, &p.CreatedAt, &p.UpdatedAt)
    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("project %s not found", id)
    }
    return p, err
}

func (s *SQLiteStore) UpdateProject(p *core.Project) error {
    _, err := s.db.Exec(
        `UPDATE projects SET name=?, repo_path=?, github_owner=?, github_repo=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
        p.Name, p.RepoPath, p.GitHubOwner, p.GitHubRepo, p.ID,
    )
    return err
}

func (s *SQLiteStore) DeleteProject(id string) error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()
    for _, tbl := range []string{"human_actions", "logs", "checkpoints", "pipelines"} {
        if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE pipeline_id IN (SELECT id FROM pipelines WHERE project_id=?)", tbl), id); err != nil && tbl != "pipelines" {
            return err
        }
    }
    if _, err := tx.Exec("DELETE FROM pipelines WHERE project_id=?", id); err != nil {
        return err
    }
    if _, err := tx.Exec("DELETE FROM projects WHERE id=?", id); err != nil {
        return err
    }
    return tx.Commit()
}

func (s *SQLiteStore) ListProjects(filter core.ProjectFilter) ([]core.Project, error) {
    rows, err := s.db.Query(`SELECT id, name, repo_path, github_owner, github_repo, created_at, updated_at FROM projects ORDER BY name`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []core.Project
    for rows.Next() {
        var p core.Project
        if err := rows.Scan(&p.ID, &p.Name, &p.RepoPath, &p.GitHubOwner, &p.GitHubRepo, &p.CreatedAt, &p.UpdatedAt); err != nil {
            return nil, err
        }
        out = append(out, p)
    }
    return out, nil
}

func (s *SQLiteStore) SavePipeline(p *core.Pipeline) error {
    stagesJSON, _ := json.Marshal(p.Stages)
    artifactsJSON, _ := json.Marshal(p.Artifacts)
    configJSON, _ := json.Marshal(p.Config)
    _, err := s.db.Exec(`
        INSERT INTO pipelines (id, project_id, name, description, template, status, current_stage,
            stages_json, artifacts_json, config_json, branch_name, worktree_path, error_message,
            max_total_retries, total_retries, started_at, finished_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
        ON CONFLICT(id) DO UPDATE SET
            status=excluded.status, current_stage=excluded.current_stage,
            stages_json=excluded.stages_json, artifacts_json=excluded.artifacts_json,
            branch_name=excluded.branch_name, worktree_path=excluded.worktree_path,
            error_message=excluded.error_message, total_retries=excluded.total_retries,
            started_at=excluded.started_at, finished_at=excluded.finished_at,
            updated_at=CURRENT_TIMESTAMP`,
        p.ID, p.ProjectID, p.Name, p.Description, p.Template, p.Status, p.CurrentStage,
        stagesJSON, artifactsJSON, configJSON, p.BranchName, p.WorktreePath, p.ErrorMessage,
        p.MaxTotalRetries, p.TotalRetries, p.StartedAt, p.FinishedAt,
    )
    return err
}

func (s *SQLiteStore) GetPipeline(id string) (*core.Pipeline, error) {
    p := &core.Pipeline{}
    var stagesJSON, artifactsJSON, configJSON string
    err := s.db.QueryRow(`SELECT id, project_id, name, description, template, status, current_stage,
        stages_json, artifacts_json, config_json, branch_name, worktree_path, error_message,
        max_total_retries, total_retries, created_at, updated_at
        FROM pipelines WHERE id=?`, id).
        Scan(&p.ID, &p.ProjectID, &p.Name, &p.Description, &p.Template, &p.Status, &p.CurrentStage,
            &stagesJSON, &artifactsJSON, &configJSON, &p.BranchName, &p.WorktreePath, &p.ErrorMessage,
            &p.MaxTotalRetries, &p.TotalRetries, &p.CreatedAt, &p.UpdatedAt)
    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("pipeline %s not found", id)
    }
    if err != nil {
        return nil, err
    }
    json.Unmarshal([]byte(stagesJSON), &p.Stages)
    json.Unmarshal([]byte(artifactsJSON), &p.Artifacts)
    json.Unmarshal([]byte(configJSON), &p.Config)
    return p, nil
}

func (s *SQLiteStore) ListPipelines(projectID string, filter core.PipelineFilter) ([]core.Pipeline, error) {
    // Simplified for P0 — returns all pipelines for project
    rows, err := s.db.Query(`SELECT id, project_id, name, template, status, current_stage, created_at FROM pipelines WHERE project_id=? ORDER BY created_at DESC`, projectID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []core.Pipeline
    for rows.Next() {
        var p core.Pipeline
        rows.Scan(&p.ID, &p.ProjectID, &p.Name, &p.Template, &p.Status, &p.CurrentStage, &p.CreatedAt)
        out = append(out, p)
    }
    return out, nil
}

func (s *SQLiteStore) GetActivePipelines() ([]core.Pipeline, error) {
    rows, err := s.db.Query(`SELECT id FROM pipelines WHERE status IN ('running','paused','waiting_human')`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []core.Pipeline
    for rows.Next() {
        var id string
        rows.Scan(&id)
        p, err := s.GetPipeline(id)
        if err == nil {
            out = append(out, *p)
        }
    }
    return out, nil
}

func (s *SQLiteStore) SaveCheckpoint(cp *core.Checkpoint) error {
    artifactsJSON, _ := json.Marshal(cp.Artifacts)
    _, err := s.db.Exec(`INSERT INTO checkpoints (pipeline_id, stage, status, agent_used, artifacts_json, tokens_used, retry_count, error_message, started_at, finished_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
        cp.PipelineID, cp.StageName, cp.Status, cp.AgentUsed, artifactsJSON, cp.TokensUsed, cp.RetryCount, cp.Error, cp.StartedAt, cp.FinishedAt)
    return err
}

func (s *SQLiteStore) GetCheckpoints(pipelineID string) ([]core.Checkpoint, error) {
    rows, err := s.db.Query(`SELECT pipeline_id, stage, status, agent_used, artifacts_json, tokens_used, retry_count, error_message, started_at, finished_at FROM checkpoints WHERE pipeline_id=? ORDER BY id`, pipelineID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []core.Checkpoint
    for rows.Next() {
        var cp core.Checkpoint
        var artifactsJSON string
        rows.Scan(&cp.PipelineID, &cp.StageName, &cp.Status, &cp.AgentUsed, &artifactsJSON, &cp.TokensUsed, &cp.RetryCount, &cp.Error, &cp.StartedAt, &cp.FinishedAt)
        json.Unmarshal([]byte(artifactsJSON), &cp.Artifacts)
        out = append(out, cp)
    }
    return out, nil
}

func (s *SQLiteStore) GetLastSuccessCheckpoint(pipelineID string) (*core.Checkpoint, error) {
    var cp core.Checkpoint
    var artifactsJSON string
    err := s.db.QueryRow(`SELECT pipeline_id, stage, status, agent_used, artifacts_json, started_at, finished_at FROM checkpoints WHERE pipeline_id=? AND status='success' ORDER BY id DESC LIMIT 1`, pipelineID).
        Scan(&cp.PipelineID, &cp.StageName, &cp.Status, &cp.AgentUsed, &artifactsJSON, &cp.StartedAt, &cp.FinishedAt)
    if err == sql.ErrNoRows {
        return nil, nil
    }
    json.Unmarshal([]byte(artifactsJSON), &cp.Artifacts)
    return &cp, err
}

func (s *SQLiteStore) AppendLog(entry core.LogEntry) error {
    _, err := s.db.Exec(`INSERT INTO logs (pipeline_id, stage, type, agent, content, timestamp) VALUES (?,?,?,?,?,?)`,
        entry.PipelineID, entry.Stage, entry.Type, entry.Agent, entry.Content, entry.Timestamp)
    return err
}

func (s *SQLiteStore) GetLogs(pipelineID string, stage string, limit int, offset int) ([]core.LogEntry, int, error) {
    var total int
    s.db.QueryRow(`SELECT COUNT(*) FROM logs WHERE pipeline_id=? AND (? = '' OR stage=?)`, pipelineID, stage, stage).Scan(&total)

    if limit <= 0 {
        limit = 100
    }
    rows, err := s.db.Query(`SELECT id, pipeline_id, stage, type, agent, content, timestamp FROM logs WHERE pipeline_id=? AND (? = '' OR stage=?) ORDER BY id LIMIT ? OFFSET ?`,
        pipelineID, stage, stage, limit, offset)
    if err != nil {
        return nil, 0, err
    }
    defer rows.Close()
    var out []core.LogEntry
    for rows.Next() {
        var e core.LogEntry
        rows.Scan(&e.ID, &e.PipelineID, &e.Stage, &e.Type, &e.Agent, &e.Content, &e.Timestamp)
        out = append(out, e)
    }
    return out, total, nil
}

func (s *SQLiteStore) RecordAction(a core.HumanAction) error {
    _, err := s.db.Exec(`INSERT INTO human_actions (pipeline_id, stage, action, message, source, user_id) VALUES (?,?,?,?,?,?)`,
        a.PipelineID, a.Stage, a.Action, a.Message, a.Source, a.UserID)
    return err
}

func (s *SQLiteStore) GetActions(pipelineID string) ([]core.HumanAction, error) {
    rows, err := s.db.Query(`SELECT id, pipeline_id, stage, action, message, source, user_id, created_at FROM human_actions WHERE pipeline_id=? ORDER BY id`, pipelineID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []core.HumanAction
    for rows.Next() {
        var a core.HumanAction
        rows.Scan(&a.ID, &a.PipelineID, &a.Stage, &a.Action, &a.Message, &a.Source, &a.UserID, &a.CreatedAt)
        out = append(out, a)
    }
    return out, nil
}
```

**Step 6: Add modernc.org/sqlite dependency**

Run: `go get modernc.org/sqlite`

**Step 7: Run tests**

Run: `go test ./internal/plugins/store-sqlite/ -v`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/core/store.go internal/plugins/store-sqlite/ go.mod go.sum
git commit -m "feat: add SQLite store — schema, migrations, full CRUD for all tables"
```

---

## Task 6: Git Operations

**Files:**
- Create: `internal/git/runner.go`
- Create: `internal/git/worktree.go`
- Create: `internal/git/branch.go`
- Test: `internal/git/worktree_test.go`

**Step 1: Write failing test**

```go
// internal/git/worktree_test.go
package git

import (
    "os"
    "os/exec"
    "path/filepath"
    "testing"
)

func setupTestRepo(t *testing.T) string {
    t.Helper()
    dir := t.TempDir()
    for _, cmd := range [][]string{
        {"git", "init", dir},
        {"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
    } {
        c := exec.Command(cmd[0], cmd[1:]...)
        if out, err := c.CombinedOutput(); err != nil {
            t.Fatalf("cmd %v: %s %v", cmd, out, err)
        }
    }
    return dir
}

func TestWorktreeCreateAndRemove(t *testing.T) {
    repo := setupTestRepo(t)
    runner := NewRunner(repo)

    wtPath := filepath.Join(t.TempDir(), "wt-test")
    branch := "feature/test-wt"

    if err := runner.WorktreeAdd(wtPath, branch); err != nil {
        t.Fatalf("worktree add: %v", err)
    }
    if _, err := os.Stat(wtPath); err != nil {
        t.Fatalf("worktree dir not created: %v", err)
    }

    if err := runner.WorktreeRemove(wtPath); err != nil {
        t.Fatalf("worktree remove: %v", err)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/git/ -v`
Expected: FAIL

**Step 3: Implement git runner**

```go
// internal/git/runner.go
package git

import (
    "bytes"
    "fmt"
    "os/exec"
    "strings"
)

type Runner struct {
    repoDir string
}

func NewRunner(repoDir string) *Runner {
    return &Runner{repoDir: repoDir}
}

func (r *Runner) run(args ...string) (string, error) {
    cmd := exec.Command("git", append([]string{"-C", r.repoDir}, args...)...)
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), stderr.String(), err)
    }
    return strings.TrimSpace(stdout.String()), nil
}
```

**Step 4: Implement worktree operations**

```go
// internal/git/worktree.go
package git

func (r *Runner) WorktreeAdd(path, branch string) error {
    _, err := r.run("worktree", "add", "-b", branch, path)
    return err
}

func (r *Runner) WorktreeRemove(path string) error {
    _, err := r.run("worktree", "remove", path, "--force")
    return err
}

func (r *Runner) WorktreeClean(path string) error {
    cmd1 := exec.Command("git", "-C", path, "checkout", ".")
    if err := cmd1.Run(); err != nil {
        return err
    }
    cmd2 := exec.Command("git", "-C", path, "clean", "-fd")
    return cmd2.Run()
}
```

**Step 5: Implement branch operations**

```go
// internal/git/branch.go
package git

func (r *Runner) BranchDelete(name string) error {
    _, err := r.run("branch", "-D", name)
    return err
}

func (r *Runner) CurrentBranch() (string, error) {
    return r.run("rev-parse", "--abbrev-ref", "HEAD")
}

func (r *Runner) Merge(branch string) (string, error) {
    return r.run("merge", branch, "--no-ff", "-m", "Merge "+branch)
}

func (r *Runner) Checkout(branch string) error {
    _, err := r.run("checkout", branch)
    return err
}
```

**Step 6: Run tests**

Run: `go test ./internal/git/ -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/git/
git commit -m "feat: add git operations — worktree add/remove/clean, branch, merge"
```

---

## Task 7: Agent Plugin — Claude Code

**Files:**
- Create: `internal/plugins/agent-claude/claude.go`
- Create: `internal/plugins/agent-claude/parser.go`
- Create: `internal/core/agent.go`
- Test: `internal/plugins/agent-claude/claude_test.go`

**Step 1: Define Agent interfaces in core**

```go
// internal/core/agent.go
package core

import (
    "context"
    "io"
    "time"
)

type ExecOpts struct {
    Prompt        string
    WorkDir       string
    AllowedTools  []string
    MaxTurns      int
    Timeout       time.Duration
    Env           map[string]string
    AppendContext string
}

type StreamEvent struct {
    Type      string    `json:"type"`
    Content   string    `json:"content"`
    ToolName  string    `json:"tool_name,omitempty"`
    ToolInput string    `json:"tool_input,omitempty"`
    Timestamp time.Time `json:"timestamp"`
}

type StreamParser interface {
    Next() (*StreamEvent, error)
}

type AgentPlugin interface {
    Plugin
    BuildCommand(opts ExecOpts) ([]string, error)
    NewStreamParser(r io.Reader) StreamParser
}

type RuntimeOpts struct {
    WorkDir string
    Env     map[string]string
    Command []string
}

type Session struct {
    ID     string
    Stdin  io.WriteCloser
    Stdout io.Reader
    Stderr io.Reader
    Wait   func() error
}

type RuntimePlugin interface {
    Plugin
    Create(ctx context.Context, opts RuntimeOpts) (*Session, error)
    Kill(sessionID string) error
}
```

**Step 2: Write failing test for Claude command builder**

```go
// internal/plugins/agent-claude/claude_test.go
package agentclaude

import (
    "strings"
    "testing"
    "time"

    "github.com/user/ai-workflow/internal/core"
)

func TestBuildCommand(t *testing.T) {
    a := New("claude")
    cmd, err := a.BuildCommand(core.ExecOpts{
        Prompt:       "implement feature X",
        MaxTurns:     20,
        AllowedTools: []string{"Read(*)", "Write(*)"},
    })
    if err != nil {
        t.Fatal(err)
    }
    joined := strings.Join(cmd, " ")
    if !strings.Contains(joined, "--output-format stream-json") {
        t.Error("missing stream-json flag")
    }
    if !strings.Contains(joined, "--max-turns 20") {
        t.Error("missing max-turns")
    }
    if !strings.Contains(joined, `--allowedTools "Read(*),Write(*)"`) {
        t.Errorf("missing allowedTools, got: %s", joined)
    }
}

func TestClaudeStreamParser(t *testing.T) {
    input := `{"type":"assistant","message":{"content":[{"type":"text","text":"hello world"}]}}
{"type":"result","result":"done","duration_ms":1234}
`
    parser := &ClaudeStreamParser{scanner: newScanner(strings.NewReader(input))}

    evt, err := parser.Next()
    if err != nil {
        t.Fatal(err)
    }
    if evt.Type != "text" || evt.Content != "hello world" {
        t.Errorf("unexpected event: %+v", evt)
    }

    evt2, err := parser.Next()
    if err != nil {
        t.Fatal(err)
    }
    if evt2.Type != "done" {
        t.Errorf("expected done, got %s", evt2.Type)
    }
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/plugins/agent-claude/ -v`
Expected: FAIL

**Step 4: Implement Claude agent**

```go
// internal/plugins/agent-claude/claude.go
package agentclaude

import (
    "context"
    "fmt"
    "io"
    "strings"

    "github.com/user/ai-workflow/internal/core"
)

type ClaudeAgent struct {
    binary string
}

func New(binary string) *ClaudeAgent {
    return &ClaudeAgent{binary: binary}
}

func (a *ClaudeAgent) Name() string                       { return "claude" }
func (a *ClaudeAgent) Init(_ context.Context) error        { return nil }
func (a *ClaudeAgent) Close() error                        { return nil }

func (a *ClaudeAgent) BuildCommand(opts core.ExecOpts) ([]string, error) {
    prompt := opts.Prompt
    if opts.AppendContext != "" {
        prompt = opts.AppendContext + "\n\n" + prompt
    }

    args := []string{a.binary, "-p", prompt, "--output-format", "stream-json"}

    if opts.MaxTurns > 0 {
        args = append(args, "--max-turns", fmt.Sprintf("%d", opts.MaxTurns))
    }
    if len(opts.AllowedTools) > 0 {
        args = append(args, "--allowedTools", fmt.Sprintf(`"%s"`, strings.Join(opts.AllowedTools, ",")))
    }
    return args, nil
}

func (a *ClaudeAgent) NewStreamParser(r io.Reader) core.StreamParser {
    return NewClaudeStreamParser(r)
}
```

**Step 5: Implement Claude stream parser**

```go
// internal/plugins/agent-claude/parser.go
package agentclaude

import (
    "bufio"
    "encoding/json"
    "io"
    "time"

    "github.com/user/ai-workflow/internal/core"
)

func newScanner(r io.Reader) *bufio.Scanner {
    return bufio.NewScanner(r)
}

type ClaudeStreamParser struct {
    scanner *bufio.Scanner
}

func NewClaudeStreamParser(r io.Reader) *ClaudeStreamParser {
    return &ClaudeStreamParser{scanner: newScanner(r)}
}

func (p *ClaudeStreamParser) Next() (*core.StreamEvent, error) {
    for p.scanner.Scan() {
        line := p.scanner.Bytes()
        if len(line) == 0 {
            continue
        }
        var raw map[string]any
        if err := json.Unmarshal(line, &raw); err != nil {
            // non-JSON line, treat as plain text
            return &core.StreamEvent{Type: "text", Content: string(line), Timestamp: time.Now()}, nil
        }

        typ, _ := raw["type"].(string)

        switch typ {
        case "assistant":
            msg, _ := raw["message"].(map[string]any)
            contents, _ := msg["content"].([]any)
            for _, c := range contents {
                cm, _ := c.(map[string]any)
                ct, _ := cm["type"].(string)
                switch ct {
                case "text":
                    text, _ := cm["text"].(string)
                    return &core.StreamEvent{Type: "text", Content: text, Timestamp: time.Now()}, nil
                case "tool_use":
                    name, _ := cm["name"].(string)
                    input, _ := json.Marshal(cm["input"])
                    return &core.StreamEvent{Type: "tool_call", ToolName: name, ToolInput: string(input), Timestamp: time.Now()}, nil
                }
            }
        case "result":
            return &core.StreamEvent{Type: "done", Content: fmt.Sprint(raw["result"]), Timestamp: time.Now()}, nil
        }
    }
    if err := p.scanner.Err(); err != nil {
        return nil, err
    }
    return nil, io.EOF
}

// fmt needed for Sprint
func init() { _ = fmt.Sprint }
```

Add missing import in parser.go: `"fmt"` is already there via init().

**Step 6: Run tests**

Run: `go test ./internal/plugins/agent-claude/ -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/core/agent.go internal/plugins/agent-claude/
git commit -m "feat: add Claude Code agent plugin — command builder + NDJSON stream parser"
```

---

## Task 8: Agent Plugin — Codex CLI

**Files:**
- Create: `internal/plugins/agent-codex/codex.go`
- Create: `internal/plugins/agent-codex/parser.go`
- Test: `internal/plugins/agent-codex/codex_test.go`

**Step 1: Write failing test**

```go
// internal/plugins/agent-codex/codex_test.go
package agentcodex

import (
    "strings"
    "testing"

    "github.com/user/ai-workflow/internal/core"
)

func TestBuildCommand(t *testing.T) {
    a := New("codex", "gpt-5.3-codex", "high")
    cmd, err := a.BuildCommand(core.ExecOpts{
        Prompt:  "fix the bug",
        WorkDir: "/tmp/project",
    })
    if err != nil {
        t.Fatal(err)
    }
    joined := strings.Join(cmd, " ")
    if !strings.Contains(joined, "exec") {
        t.Error("missing exec subcommand")
    }
    if !strings.Contains(joined, "--sandbox workspace-write") {
        t.Error("missing sandbox flag")
    }
    if !strings.Contains(joined, "-a never") {
        t.Error("missing approval flag")
    }
    if !strings.Contains(joined, "-m gpt-5.3-codex") {
        t.Error("missing model flag")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/plugins/agent-codex/ -v`
Expected: FAIL

**Step 3: Implement Codex agent**

```go
// internal/plugins/agent-codex/codex.go
package agentcodex

import (
    "context"
    "io"

    "github.com/user/ai-workflow/internal/core"
)

type CodexAgent struct {
    binary    string
    model     string
    reasoning string
}

func New(binary, model, reasoning string) *CodexAgent {
    return &CodexAgent{binary: binary, model: model, reasoning: reasoning}
}

func (a *CodexAgent) Name() string                       { return "codex" }
func (a *CodexAgent) Init(_ context.Context) error        { return nil }
func (a *CodexAgent) Close() error                        { return nil }

func (a *CodexAgent) BuildCommand(opts core.ExecOpts) ([]string, error) {
    prompt := opts.Prompt
    if opts.AppendContext != "" {
        prompt = opts.AppendContext + "\n\n" + prompt
    }

    args := []string{
        a.binary, "exec", prompt,
        "--sandbox", "workspace-write",
        "-a", "never",
        "-m", a.model,
        "-c", "model_reasoning_effort=" + a.reasoning,
    }
    if opts.WorkDir != "" {
        args = append(args, "-C", opts.WorkDir)
    }
    return args, nil
}

func (a *CodexAgent) NewStreamParser(r io.Reader) core.StreamParser {
    return NewCodexStreamParser(r)
}
```

**Step 4: Implement Codex stream parser**

```go
// internal/plugins/agent-codex/parser.go
package agentcodex

import (
    "bufio"
    "io"
    "time"

    "github.com/user/ai-workflow/internal/core"
)

type CodexStreamParser struct {
    scanner *bufio.Scanner
}

func NewCodexStreamParser(r io.Reader) *CodexStreamParser {
    return &CodexStreamParser{scanner: bufio.NewScanner(r)}
}

func (p *CodexStreamParser) Next() (*core.StreamEvent, error) {
    if p.scanner.Scan() {
        line := p.scanner.Text()
        if line == "" {
            return p.Next()
        }
        // P0: treat all Codex output as plain text
        return &core.StreamEvent{Type: "text", Content: line, Timestamp: time.Now()}, nil
    }
    if err := p.scanner.Err(); err != nil {
        return nil, err
    }
    return nil, io.EOF
}
```

**Step 5: Run tests**

Run: `go test ./internal/plugins/agent-codex/ -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/plugins/agent-codex/
git commit -m "feat: add Codex CLI agent plugin — command builder + stream parser"
```

---

## Task 9: Runtime Plugin — Process

**Files:**
- Create: `internal/plugins/runtime-process/runtime.go`
- Test: `internal/plugins/runtime-process/runtime_test.go`

**Step 1: Write failing test**

```go
// internal/plugins/runtime-process/runtime_test.go
package runtimeprocess

import (
    "context"
    "io"
    "testing"

    "github.com/user/ai-workflow/internal/core"
)

func TestCreateAndWait(t *testing.T) {
    rt := New()
    rt.Init(context.Background())

    sess, err := rt.Create(context.Background(), core.RuntimeOpts{
        Command: []string{"echo", "hello"},
    })
    if err != nil {
        t.Fatal(err)
    }

    out, _ := io.ReadAll(sess.Stdout)
    if err := sess.Wait(); err != nil {
        t.Fatal(err)
    }
    if string(out) == "" {
        t.Error("expected output from echo")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/plugins/runtime-process/ -v`
Expected: FAIL

**Step 3: Implement process runtime**

```go
// internal/plugins/runtime-process/runtime.go
package runtimeprocess

import (
    "context"
    "fmt"
    "os/exec"
    "sync/atomic"

    "github.com/user/ai-workflow/internal/core"
)

type ProcessRuntime struct {
    counter atomic.Int64
}

func New() *ProcessRuntime { return &ProcessRuntime{} }

func (r *ProcessRuntime) Name() string                { return "process" }
func (r *ProcessRuntime) Init(_ context.Context) error { return nil }
func (r *ProcessRuntime) Close() error                 { return nil }

func (r *ProcessRuntime) Create(ctx context.Context, opts core.RuntimeOpts) (*core.Session, error) {
    if len(opts.Command) == 0 {
        return nil, fmt.Errorf("empty command")
    }

    cmd := exec.CommandContext(ctx, opts.Command[0], opts.Command[1:]...)
    if opts.WorkDir != "" {
        cmd.Dir = opts.WorkDir
    }
    for k, v := range opts.Env {
        cmd.Env = append(cmd.Environ(), k+"="+v)
    }

    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return nil, err
    }
    stderr, err := cmd.StderrPipe()
    if err != nil {
        return nil, err
    }
    stdin, err := cmd.StdinPipe()
    if err != nil {
        return nil, err
    }

    if err := cmd.Start(); err != nil {
        return nil, fmt.Errorf("start %v: %w", opts.Command, err)
    }

    id := fmt.Sprintf("proc-%d", r.counter.Add(1))
    return &core.Session{
        ID:     id,
        Stdin:  stdin,
        Stdout: stdout,
        Stderr: stderr,
        Wait:   cmd.Wait,
    }, nil
}

func (r *ProcessRuntime) Kill(sessionID string) error {
    // P0: simplified — Kill not tracked by session ID
    return nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/plugins/runtime-process/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/plugins/runtime-process/
git commit -m "feat: add process runtime plugin — os/exec based session management"
```

---

## Task 10: Pipeline Executor

**Files:**
- Create: `internal/engine/executor.go`
- Create: `internal/engine/templates.go`
- Create: `internal/engine/id.go`
- Test: `internal/engine/executor_test.go`

**Step 1: Implement template definitions**

```go
// internal/engine/templates.go
package engine

import "github.com/user/ai-workflow/internal/core"

var Templates = map[string][]core.StageID{
    "full": {
        core.StageRequirements, core.StagePlanDraft, core.StagePlanReview,
        core.StageWorktreeSetup, core.StageImplement, core.StageCodeReview,
        core.StageFixup, core.StageMerge, core.StageCleanup,
    },
    "standard": {
        core.StageRequirements, core.StageWorktreeSetup, core.StageImplement,
        core.StageCodeReview, core.StageFixup, core.StageMerge, core.StageCleanup,
    },
    "quick": {
        core.StageRequirements, core.StageWorktreeSetup, core.StageImplement,
        core.StageCodeReview, core.StageMerge, core.StageCleanup,
    },
    "hotfix": {
        core.StageWorktreeSetup, core.StageImplement, core.StageMerge, core.StageCleanup,
    },
}
```

**Step 2: Implement Pipeline ID generator**

```go
// internal/engine/id.go
package engine

import (
    "crypto/rand"
    "fmt"
    "time"
)

func NewPipelineID() string {
    b := make([]byte, 6) // 48 bits = 12 hex chars
    rand.Read(b)
    return fmt.Sprintf("%s-%x", time.Now().Format("20060102"), b)
}
```

**Step 3: Write failing test for executor**

```go
// internal/engine/executor_test.go
package engine

import (
    "testing"
)

func TestNewPipelineID(t *testing.T) {
    id := NewPipelineID()
    // format: YYYYMMDD-12hexchars
    if len(id) != 8+1+12 {
        t.Errorf("unexpected ID length: %s (len=%d)", id, len(id))
    }
}

func TestTemplatesDefined(t *testing.T) {
    for _, name := range []string{"full", "standard", "quick", "hotfix"} {
        stages, ok := Templates[name]
        if !ok {
            t.Errorf("template %s not defined", name)
        }
        if len(stages) == 0 {
            t.Errorf("template %s has no stages", name)
        }
    }
    // quick and hotfix must include worktree_setup and cleanup
    for _, name := range []string{"quick", "hotfix"} {
        stages := Templates[name]
        hasWT, hasCL := false, false
        for _, s := range stages {
            if s == "worktree_setup" { hasWT = true }
            if s == "cleanup" { hasCL = true }
        }
        if !hasWT {
            t.Errorf("%s missing worktree_setup", name)
        }
        if !hasCL {
            t.Errorf("%s missing cleanup", name)
        }
    }
}
```

**Step 4: Run tests**

Run: `go test ./internal/engine/ -v`
Expected: PASS

**Step 5: Implement executor skeleton**

```go
// internal/engine/executor.go
package engine

import (
    "context"
    "fmt"
    "log/slog"
    "time"

    "github.com/user/ai-workflow/internal/core"
    "github.com/user/ai-workflow/internal/eventbus"
)

type Executor struct {
    store   core.Store
    bus     *eventbus.Bus
    agents  map[string]core.AgentPlugin
    runtime core.RuntimePlugin
    logger  *slog.Logger
}

func NewExecutor(store core.Store, bus *eventbus.Bus, agents map[string]core.AgentPlugin, runtime core.RuntimePlugin, logger *slog.Logger) *Executor {
    return &Executor{store: store, bus: bus, agents: agents, runtime: runtime, logger: logger}
}

func (e *Executor) CreatePipeline(projectID, name, description, template string) (*core.Pipeline, error) {
    stageIDs, ok := Templates[template]
    if !ok {
        return nil, fmt.Errorf("unknown template: %s", template)
    }

    stages := make([]core.StageConfig, len(stageIDs))
    for i, sid := range stageIDs {
        stages[i] = defaultStageConfig(sid)
    }

    p := &core.Pipeline{
        ID:              NewPipelineID(),
        ProjectID:       projectID,
        Name:            name,
        Description:     description,
        Template:        template,
        Status:          core.StatusCreated,
        Stages:          stages,
        Artifacts:       make(map[string]string),
        MaxTotalRetries: 5,
        CreatedAt:       time.Now(),
        UpdatedAt:       time.Now(),
    }

    if err := e.store.SavePipeline(p); err != nil {
        return nil, err
    }
    return p, nil
}

func (e *Executor) Run(ctx context.Context, pipelineID string) error {
    p, err := e.store.GetPipeline(pipelineID)
    if err != nil {
        return err
    }

    if err := core.ValidateTransition(p.Status, core.StatusRunning); err != nil {
        return err
    }
    p.Status = core.StatusRunning
    p.StartedAt = time.Now()
    e.store.SavePipeline(p)

    for i, stage := range p.Stages {
        p.CurrentStage = stage.Name
        e.store.SavePipeline(p)

        e.bus.Publish(core.Event{
            Type: core.EventStageStart, PipelineID: p.ID, ProjectID: p.ProjectID,
            Stage: stage.Name, Timestamp: time.Now(),
        })

        // Write in_progress checkpoint
        cp := &core.Checkpoint{
            PipelineID: p.ID, StageName: stage.Name,
            Status: core.CheckpointInProgress, StartedAt: time.Now(),
            AgentUsed: stage.Agent,
        }
        e.store.SaveCheckpoint(cp)

        err := e.executeStage(ctx, p, &p.Stages[i])

        cp.FinishedAt = time.Now()
        if err != nil {
            cp.Status = core.CheckpointFailed
            cp.Error = err.Error()
            e.store.SaveCheckpoint(cp)

            e.bus.Publish(core.Event{
                Type: core.EventStageFailed, PipelineID: p.ID,
                Stage: stage.Name, Error: err.Error(), Timestamp: time.Now(),
            })

            p.TotalRetries++
            if p.TotalRetries >= p.MaxTotalRetries {
                p.Status = core.StatusFailed
                p.ErrorMessage = fmt.Sprintf("retry budget exhausted at stage %s: %v", stage.Name, err)
                p.FinishedAt = time.Now()
                e.store.SavePipeline(p)
                e.bus.Publish(core.Event{Type: core.EventPipelineFailed, PipelineID: p.ID, Timestamp: time.Now()})
                return fmt.Errorf("pipeline failed: %w", err)
            }

            // on_failure: human → wait for human action
            if stage.OnFailure == core.OnFailureHuman {
                p.Status = core.StatusWaitingHuman
                e.store.SavePipeline(p)
                e.bus.Publish(core.Event{Type: core.EventHumanRequired, PipelineID: p.ID, Stage: stage.Name, Timestamp: time.Now()})
                return nil // paused, will be resumed later
            }

            // on_failure: abort
            if stage.OnFailure == core.OnFailureAbort {
                p.Status = core.StatusFailed
                p.FinishedAt = time.Now()
                e.store.SavePipeline(p)
                return fmt.Errorf("stage %s failed, aborting: %w", stage.Name, err)
            }

            continue // retry or skip handled by simplified logic
        }

        cp.Status = core.CheckpointSuccess
        e.store.SaveCheckpoint(cp)

        e.bus.Publish(core.Event{
            Type: core.EventStageComplete, PipelineID: p.ID,
            Stage: stage.Name, Timestamp: time.Now(),
        })

        // Check for human approval
        if stage.RequireHuman {
            p.Status = core.StatusWaitingHuman
            e.store.SavePipeline(p)
            e.bus.Publish(core.Event{Type: core.EventHumanRequired, PipelineID: p.ID, Stage: stage.Name, Timestamp: time.Now()})
            return nil // will be resumed by human action
        }
    }

    p.Status = core.StatusDone
    p.FinishedAt = time.Now()
    e.store.SavePipeline(p)
    e.bus.Publish(core.Event{Type: core.EventPipelineDone, PipelineID: p.ID, Timestamp: time.Now()})
    return nil
}

func (e *Executor) executeStage(ctx context.Context, p *core.Pipeline, stage *core.StageConfig) error {
    // Non-agent stages
    switch stage.Name {
    case core.StageWorktreeSetup, core.StageMerge, core.StageCleanup:
        e.logger.Info("executing built-in stage", "stage", stage.Name, "pipeline", p.ID)
        return nil // will be implemented with git ops integration
    }

    agent, ok := e.agents[stage.Agent]
    if !ok {
        return fmt.Errorf("agent %q not found", stage.Agent)
    }

    opts := core.ExecOpts{
        Prompt:   fmt.Sprintf("Execute stage: %s\nDescription: %s", stage.Name, p.Description),
        WorkDir:  p.WorktreePath,
        MaxTurns: 30,
        Timeout:  stage.Timeout,
    }

    cmd, err := agent.BuildCommand(opts)
    if err != nil {
        return fmt.Errorf("build command: %w", err)
    }

    stageCtx := ctx
    if stage.Timeout > 0 {
        var cancel context.CancelFunc
        stageCtx, cancel = context.WithTimeout(ctx, stage.Timeout)
        defer cancel()
    }

    sess, err := e.runtime.Create(stageCtx, core.RuntimeOpts{
        Command: cmd, WorkDir: p.WorktreePath,
    })
    if err != nil {
        return fmt.Errorf("create session: %w", err)
    }

    parser := agent.NewStreamParser(sess.Stdout)
    for {
        evt, err := parser.Next()
        if err != nil {
            break
        }
        e.bus.Publish(core.Event{
            Type: core.EventAgentOutput, PipelineID: p.ID,
            Stage: stage.Name, Agent: stage.Agent,
            Data: map[string]string{"content": evt.Content, "type": evt.Type},
            Timestamp: evt.Timestamp,
        })
    }

    return sess.Wait()
}

func defaultStageConfig(id core.StageID) core.StageConfig {
    cfg := core.StageConfig{
        Name:           id,
        Timeout:        30 * time.Minute,
        MaxRetries:     1,
        OnFailure:      core.OnFailureHuman,
    }
    switch id {
    case core.StageRequirements, core.StagePlanDraft, core.StagePlanReview, core.StageCodeReview:
        cfg.Agent = "claude"
    case core.StageImplement, core.StageFixup:
        cfg.Agent = "codex"
    case core.StageWorktreeSetup, core.StageMerge, core.StageCleanup:
        cfg.Agent = "" // built-in, no agent
        cfg.Timeout = 2 * time.Minute
    }
    cfg.PromptTemplate = string(id)
    return cfg
}
```

**Step 6: Run tests**

Run: `go test ./internal/engine/ -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/engine/
git commit -m "feat: add Pipeline Executor — stage loop, checkpoint management, event publishing, template definitions"
```

---

## Task 11: CLI Entry Point

**Files:**
- Modify: `cmd/ai-flow/main.go`
- Create: `cmd/ai-flow/commands.go`

**Step 1: Implement CLI with basic commands**

```go
// cmd/ai-flow/commands.go
package main

import (
    "context"
    "fmt"
    "log/slog"
    "os"
    "text/tabwriter"

    "github.com/user/ai-workflow/internal/core"
    "github.com/user/ai-workflow/internal/engine"
    "github.com/user/ai-workflow/internal/eventbus"
    agentclaude "github.com/user/ai-workflow/internal/plugins/agent-claude"
    agentcodex "github.com/user/ai-workflow/internal/plugins/agent-codex"
    runtimeprocess "github.com/user/ai-workflow/internal/plugins/runtime-process"
    storesqlite "github.com/user/ai-workflow/internal/plugins/store-sqlite"
)

func bootstrap() (*engine.Executor, core.Store, error) {
    home, _ := os.UserHomeDir()
    dbPath := home + "/.ai-workflow/data.db"
    os.MkdirAll(home+"/.ai-workflow", 0755)

    store, err := storesqlite.New(dbPath)
    if err != nil {
        return nil, nil, err
    }

    bus := eventbus.New()
    logger := slog.Default()

    agents := map[string]core.AgentPlugin{
        "claude": agentclaude.New("claude"),
        "codex":  agentcodex.New("codex", "gpt-5.3-codex", "high"),
    }
    runtime := runtimeprocess.New()

    exec := engine.NewExecutor(store, bus, agents, runtime, logger)
    return exec, store, nil
}

func cmdProjectAdd(args []string) error {
    if len(args) < 2 {
        return fmt.Errorf("usage: ai-flow project add <id> <repo-path>")
    }
    _, store, err := bootstrap()
    if err != nil {
        return err
    }
    defer store.Close()

    p := &core.Project{ID: args[0], Name: args[0], RepoPath: args[1]}
    return store.CreateProject(p)
}

func cmdProjectList() error {
    _, store, err := bootstrap()
    if err != nil {
        return err
    }
    defer store.Close()

    projects, err := store.ListProjects(core.ProjectFilter{})
    if err != nil {
        return err
    }

    w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
    fmt.Fprintln(w, "ID\tNAME\tPATH")
    for _, p := range projects {
        fmt.Fprintf(w, "%s\t%s\t%s\n", p.ID, p.Name, p.RepoPath)
    }
    return w.Flush()
}

func cmdPipelineCreate(args []string) error {
    if len(args) < 3 {
        return fmt.Errorf("usage: ai-flow pipeline create <project-id> <name> <description> [template]")
    }
    exec, store, err := bootstrap()
    if err != nil {
        return err
    }
    defer store.Close()

    template := "standard"
    if len(args) > 3 {
        template = args[3]
    }

    p, err := exec.CreatePipeline(args[0], args[1], args[2], template)
    if err != nil {
        return err
    }
    fmt.Printf("Pipeline created: %s (template: %s, stages: %d)\n", p.ID, p.Template, len(p.Stages))
    return nil
}

func cmdPipelineStart(args []string) error {
    if len(args) < 1 {
        return fmt.Errorf("usage: ai-flow pipeline start <pipeline-id>")
    }
    exec, store, err := bootstrap()
    if err != nil {
        return err
    }
    defer store.Close()

    return exec.Run(context.Background(), args[0])
}

func cmdPipelineStatus(args []string) error {
    if len(args) < 1 {
        return fmt.Errorf("usage: ai-flow pipeline status <pipeline-id>")
    }
    _, store, err := bootstrap()
    if err != nil {
        return err
    }
    defer store.Close()

    p, err := store.GetPipeline(args[0])
    if err != nil {
        return err
    }
    fmt.Printf("Pipeline: %s\n", p.ID)
    fmt.Printf("Status:   %s\n", p.Status)
    fmt.Printf("Stage:    %s\n", p.CurrentStage)
    fmt.Printf("Template: %s\n", p.Template)
    return nil
}
```

**Step 2: Update main.go with command routing**

```go
// cmd/ai-flow/main.go
package main

import (
    "fmt"
    "os"
)

func main() {
    if err := run(); err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }
}

func run() error {
    args := os.Args[1:]
    if len(args) == 0 {
        printUsage()
        return nil
    }

    switch args[0] {
    case "version":
        fmt.Println("ai-flow v0.1.0-dev")
    case "project":
        if len(args) < 2 {
            return fmt.Errorf("usage: ai-flow project <add|list>")
        }
        switch args[1] {
        case "add":
            return cmdProjectAdd(args[2:])
        case "list", "ls":
            return cmdProjectList()
        default:
            return fmt.Errorf("unknown project command: %s", args[1])
        }
    case "pipeline":
        if len(args) < 2 {
            return fmt.Errorf("usage: ai-flow pipeline <create|start|status>")
        }
        switch args[1] {
        case "create":
            return cmdPipelineCreate(args[2:])
        case "start":
            return cmdPipelineStart(args[2:])
        case "status":
            return cmdPipelineStatus(args[2:])
        default:
            return fmt.Errorf("unknown pipeline command: %s", args[1])
        }
    default:
        return fmt.Errorf("unknown command: %s", args[0])
    }
    return nil
}

func printUsage() {
    fmt.Println(`ai-flow — AI Workflow Orchestrator

Usage:
  ai-flow version                                Show version
  ai-flow project add <id> <repo-path>           Register project
  ai-flow project list                           List projects
  ai-flow pipeline create <pid> <name> <desc> [template]  Create pipeline
  ai-flow pipeline start <pipeline-id>           Start pipeline
  ai-flow pipeline status <pipeline-id>          Show pipeline status`)
}
```

**Step 3: Build and verify**

Run: `go build ./cmd/ai-flow/ && ./ai-flow version`
Expected: `ai-flow v0.1.0-dev`

**Step 4: Commit**

```bash
git add cmd/ai-flow/
git commit -m "feat: add CLI entry point — project add/list, pipeline create/start/status"
```

---

## Task 12: TUI (Bubble Tea)

> This is the largest task. Implement a basic TUI with pipeline list view and stage progress.

**Files:**
- Create: `internal/tui/app.go`
- Create: `internal/tui/views/pipeline_list.go`
- Create: `internal/tui/styles.go`

**Step 1: Add Bubble Tea dependency**

Run: `go get github.com/charmbracelet/bubbletea github.com/charmbracelet/lipgloss`

**Step 2: Implement styles**

```go
// internal/tui/styles.go
package tui

import "github.com/charmbracelet/lipgloss"

var (
    StyleTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
    StyleStatus = map[string]lipgloss.Style{
        "created":       lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
        "running":       lipgloss.NewStyle().Foreground(lipgloss.Color("32")),
        "waiting_human": lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
        "done":          lipgloss.NewStyle().Foreground(lipgloss.Color("34")),
        "failed":        lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
        "aborted":       lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
        "paused":        lipgloss.NewStyle().Foreground(lipgloss.Color("226")),
    }
    StyleHelp = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)
```

**Step 3: Implement TUI app with pipeline list**

```go
// internal/tui/app.go
package tui

import (
    "fmt"
    "strings"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/user/ai-workflow/internal/core"
)

type Model struct {
    store     core.Store
    pipelines []core.Pipeline
    cursor    int
    err       error
}

func NewModel(store core.Store) Model {
    return Model{store: store}
}

type pipelinesLoadedMsg []core.Pipeline
type errMsg error

func (m Model) Init() tea.Cmd {
    return func() tea.Msg {
        projects, err := m.store.ListProjects(core.ProjectFilter{})
        if err != nil {
            return errMsg(err)
        }
        var all []core.Pipeline
        for _, proj := range projects {
            pipes, _ := m.store.ListPipelines(proj.ID, core.PipelineFilter{})
            all = append(all, pipes...)
        }
        return pipelinesLoadedMsg(all)
    }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case pipelinesLoadedMsg:
        m.pipelines = msg
    case errMsg:
        m.err = msg
    case tea.KeyMsg:
        switch msg.String() {
        case "q", "ctrl+c":
            return m, tea.Quit
        case "up", "k":
            if m.cursor > 0 {
                m.cursor--
            }
        case "down", "j":
            if m.cursor < len(m.pipelines)-1 {
                m.cursor++
            }
        }
    }
    return m, nil
}

func (m Model) View() string {
    var b strings.Builder
    b.WriteString(StyleTitle.Render("AI Workflow Orchestrator") + "\n\n")

    if m.err != nil {
        b.WriteString(fmt.Sprintf("Error: %v\n", m.err))
        return b.String()
    }

    if len(m.pipelines) == 0 {
        b.WriteString("No pipelines found. Use `ai-flow pipeline create` to get started.\n")
    } else {
        for i, p := range m.pipelines {
            cursor := "  "
            if i == m.cursor {
                cursor = "> "
            }
            status := string(p.Status)
            if s, ok := StyleStatus[status]; ok {
                status = s.Render(status)
            }
            b.WriteString(fmt.Sprintf("%s%-21s %-20s %s\n", cursor, p.ID, p.Name, status))
        }
    }

    b.WriteString("\n" + StyleHelp.Render("↑/↓ navigate • q quit"))
    return b.String()
}

func Run(store core.Store) error {
    p := tea.NewProgram(NewModel(store))
    _, err := p.Run()
    return err
}
```

**Step 4: Add TUI command to CLI**

Add to `cmd/ai-flow/main.go` switch:
```go
case "tui":
    _, store, err := bootstrap()
    if err != nil {
        return err
    }
    defer store.Close()
    return tui.Run(store)
```

**Step 5: Build and run TUI**

Run: `go build ./cmd/ai-flow/ && ./ai-flow tui`
Expected: TUI renders with empty pipeline list

**Step 6: Commit**

```bash
git add internal/tui/ cmd/ai-flow/ go.mod go.sum
git commit -m "feat: add Bubble Tea TUI — pipeline list view with status colors"
```

---

## Task 13: Prompt Templates

**Files:**
- Create: `configs/prompts/requirements.tmpl`
- Create: `configs/prompts/implement.tmpl`
- Create: `configs/prompts/code_review.tmpl`
- Create: `configs/prompts/fixup.tmpl`
- Create: `internal/engine/prompts.go`

**Step 1: Create prompt template files**

See spec-agent-drivers.md §十 for template variables. Create minimal but functional templates.

**Step 2: Implement template loader**

```go
// internal/engine/prompts.go
package engine

import (
    "embed"
    "fmt"
    "strings"
    "text/template"
)

//go:embed ../../configs/prompts/*.tmpl
var promptFS embed.FS

type PromptVars struct {
    ProjectName    string
    ChangeName     string
    RepoPath       string
    WorktreePath   string
    Requirements   string
    SpecPath       string
    TasksMD        string
    PreviousReview string
    HumanFeedback  string
    RetryError     string
    RetryCount     int
}

func RenderPrompt(stage string, vars PromptVars) (string, error) {
    data, err := promptFS.ReadFile(fmt.Sprintf("configs/prompts/%s.tmpl", stage))
    if err != nil {
        // Fallback: return a generic prompt
        return fmt.Sprintf("Execute stage: %s\nRequirements: %s", stage, vars.Requirements), nil
    }
    tmpl, err := template.New(stage).Parse(string(data))
    if err != nil {
        return "", err
    }
    var b strings.Builder
    if err := tmpl.Execute(&b, vars); err != nil {
        return "", err
    }
    return b.String(), nil
}
```

**Step 3: Create template files**

```
// configs/prompts/requirements.tmpl
你正在项目 {{.ProjectName}} ({{.RepoPath}}) 中工作。

请将以下需求结构化，输出一份清晰的需求文档：

{{.Requirements}}

要求：
1. 明确功能边界
2. 列出验收标准
3. 识别技术约束
```

```
// configs/prompts/implement.tmpl
你正在项目 {{.ProjectName}} 的 worktree ({{.WorktreePath}}) 中工作。

{{if .RetryError}}上次执行失败，错误信息：{{.RetryError}}
请避免同样的问题。{{end}}
{{if .HumanFeedback}}用户反馈：{{.HumanFeedback}}
请根据以上反馈调整方案。{{end}}

请根据以下需求实现代码：

{{.Requirements}}

{{if .TasksMD}}任务清单：
{{.TasksMD}}{{end}}
```

**Step 4: Commit**

```bash
git add configs/prompts/ internal/engine/prompts.go
git commit -m "feat: add prompt template system — Go text/template with embed.FS"
```

---

## Task 14: Integration Wiring & Smoke Test

**Step 1: Wire all components in bootstrap**

Ensure `cmd/ai-flow/commands.go` correctly constructs all plugins and executor.

**Step 2: End-to-end smoke test**

```bash
./ai-flow project add demo /tmp/demo-repo
./ai-flow project list
./ai-flow pipeline create demo test-pipe "test pipeline" quick
./ai-flow pipeline status <pipeline-id>
./ai-flow tui
```

**Step 3: Run all tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 4: Final commit**

```bash
git add -A
git commit -m "feat: P0 MVP — complete local orchestrator with CLI, TUI, Agent drivers, Pipeline engine"
```

---

Plan complete and saved to `docs/plans/2026-02-28-p0-implementation.md`.
