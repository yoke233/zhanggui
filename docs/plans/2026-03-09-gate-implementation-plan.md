# D2 Gate 门禁 — 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 用可配置的 Gate 链替代固定 ReviewGate 插件选择，支持 auto/owner_review 两种策略（peer_review/vote 留占位），可串联多道门禁。同时落地 D1 Decision 代码。

**Architecture:** Gate 是 WorkflowProfile 上的配置属性。GateChain 按顺序执行多道 Gate，每道可重试。每次 gate_check 产生 TaskStep + GateCheck 记录，auto 类型额外产生 Decision 记录。向后兼容：Gates 为空时走 defaultProfileGates 映射，行为与当前一致。

**Tech Stack:** Go 1.22+, SQLite (migration V11+V12), chi router, React/TypeScript

**设计文档:** `docs/plans/2026-03-09-gate-design.md`

---

## Wave 1: 基础设施（D1 代码落地 + Gate 模型）

### Task 1: Decision 核心模型

**Files:**
- Create: `internal/core/decision.go`
- Create: `internal/core/decision_test.go`

**Step 1:** 创建 `internal/core/decision.go`

```go
package core

import (
	"crypto/sha256"
	"fmt"
	"time"
)

const (
	DecisionTypeReview    = "review"
	DecisionTypeDecompose = "decompose"
	DecisionTypeStage     = "stage"
	DecisionTypeChat      = "chat"
	DecisionTypeGateCheck = "gate_check"
)

type Decision struct {
	ID              string    `json:"id"`
	IssueID         string    `json:"issue_id"`
	RunID           string    `json:"run_id,omitempty"`
	StageID         StageID   `json:"stage_id,omitempty"`
	AgentID         string    `json:"agent_id"`
	Type            string    `json:"type"`
	PromptHash      string    `json:"prompt_hash"`
	PromptPreview   string    `json:"prompt_preview"`
	Model           string    `json:"model"`
	Template        string    `json:"template"`
	TemplateVersion string    `json:"template_version"`
	InputTokens     int       `json:"input_tokens"`
	Action          string    `json:"action"`
	Reasoning       string    `json:"reasoning"`
	Confidence      float64   `json:"confidence"`
	OutputTokens    int       `json:"output_tokens"`
	OutputData      string    `json:"output_data"`
	DurationMs      int64     `json:"duration_ms"`
	CreatedAt       time.Time `json:"created_at"`
}

func NewDecisionID() string {
	return fmt.Sprintf("dec-%s-%s", time.Now().Format("20060102-150405"), randomHex(4))
}

func PromptHash(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return fmt.Sprintf("%x", h[:8])
}

func TruncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}
```

**Step 2:** 创建 `internal/core/decision_test.go`

```go
package core

import (
	"strings"
	"testing"
)

func TestNewDecisionID(t *testing.T) {
	id := NewDecisionID()
	if !strings.HasPrefix(id, "dec-") {
		t.Errorf("expected prefix 'dec-', got %q", id)
	}
	if len(id) != 29 {
		t.Errorf("expected length 29, got %d for %q", len(id), id)
	}
}

func TestPromptHash(t *testing.T) {
	hash := PromptHash("hello world")
	if len(hash) != 16 {
		t.Errorf("expected 16 hex chars, got %d: %q", len(hash), hash)
	}
	if PromptHash("hello world") != hash {
		t.Error("PromptHash should be deterministic")
	}
	if PromptHash("goodbye world") == hash {
		t.Error("different inputs should produce different hashes")
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 3, "hel"},
		{"", 5, ""},
		{"你好世界", 2, "你好"},
	}
	for _, tt := range tests {
		got := TruncateString(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("TruncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}
```

**Step 3: 验证**

Run: `go test ./internal/core/... -run "TestNewDecisionID|TestPromptHash|TestTruncateString" -v`
Expected: ALL PASS

**Step 4: Commit**

```
feat(core): add Decision model with ID generation and prompt hashing
```

---

### Task 2: Gate 核心模型

**Files:**
- Create: `internal/core/gate.go`
- Create: `internal/core/gate_test.go`

**Step 1:** 创建 `internal/core/gate.go`

```go
package core

import (
	"context"
	"fmt"
	"time"
)

type GateType string

const (
	GateTypeAuto        GateType = "auto"
	GateTypeOwnerReview GateType = "owner_review"
	GateTypePeerReview  GateType = "peer_review"
	GateTypeVote        GateType = "vote"
)

type GateFallback string

const (
	GateFallbackEscalate  GateFallback = "escalate"
	GateFallbackForcePass GateFallback = "force_pass"
	GateFallbackAbort     GateFallback = "abort"
)

type GateStatus string

const (
	GateStatusPending GateStatus = "pending"
	GateStatusPassed  GateStatus = "passed"
	GateStatusFailed  GateStatus = "failed"
	GateStatusSkipped GateStatus = "skipped"
)

// Gate defines a single checkpoint in the review pipeline.
type Gate struct {
	Name        string       `json:"name"`
	Type        GateType     `json:"type"`
	Rules       string       `json:"rules"`
	MaxAttempts int          `json:"max_attempts,omitempty"`
	Fallback    GateFallback `json:"fallback,omitempty"`
}

// GateCheck records one attempt at passing a gate.
type GateCheck struct {
	ID         string     `json:"id"`
	IssueID    string     `json:"issue_id"`
	GateName   string     `json:"gate_name"`
	GateType   GateType   `json:"gate_type"`
	Attempt    int        `json:"attempt"`
	Status     GateStatus `json:"status"`
	Reason     string     `json:"reason"`
	DecisionID string     `json:"decision_id,omitempty"`
	CheckedBy  string     `json:"checked_by"`
	CreatedAt  time.Time  `json:"created_at"`
}

func NewGateCheckID() string {
	return fmt.Sprintf("gc-%s-%s", time.Now().Format("20060102-150405"), randomHex(4))
}

// GateRunner evaluates a single gate for an issue.
type GateRunner interface {
	Check(ctx context.Context, issue *Issue, gate Gate, attempt int) (*GateCheck, error)
}

// Validate checks Gate fields.
func (g Gate) Validate() error {
	if g.Name == "" {
		return fmt.Errorf("gate name is required")
	}
	switch g.Type {
	case GateTypeAuto, GateTypeOwnerReview, GateTypePeerReview, GateTypeVote:
	default:
		return fmt.Errorf("invalid gate type %q", g.Type)
	}
	if g.MaxAttempts < 0 {
		return fmt.Errorf("max_attempts must be >= 0")
	}
	if g.Fallback != "" {
		switch g.Fallback {
		case GateFallbackEscalate, GateFallbackForcePass, GateFallbackAbort:
		default:
			return fmt.Errorf("invalid gate fallback %q", g.Fallback)
		}
	}
	return nil
}

// ValidateGates checks a gate chain for validity.
func ValidateGates(gates []Gate) error {
	names := make(map[string]struct{}, len(gates))
	for i, g := range gates {
		if err := g.Validate(); err != nil {
			return fmt.Errorf("gate[%d]: %w", i, err)
		}
		if _, exists := names[g.Name]; exists {
			return fmt.Errorf("gate[%d]: duplicate gate name %q", i, g.Name)
		}
		names[g.Name] = struct{}{}
	}
	return nil
}
```

**Step 2:** 创建 `internal/core/gate_test.go`

```go
package core

import (
	"strings"
	"testing"
)

func TestNewGateCheckID(t *testing.T) {
	id := NewGateCheckID()
	if !strings.HasPrefix(id, "gc-") {
		t.Errorf("expected prefix 'gc-', got %q", id)
	}
}

func TestGateValidate(t *testing.T) {
	tests := []struct {
		name    string
		gate    Gate
		wantErr bool
	}{
		{"valid auto", Gate{Name: "test", Type: GateTypeAuto}, false},
		{"valid with all fields", Gate{Name: "review", Type: GateTypeOwnerReview, Rules: "check quality", MaxAttempts: 3, Fallback: GateFallbackEscalate}, false},
		{"missing name", Gate{Type: GateTypeAuto}, true},
		{"invalid type", Gate{Name: "test", Type: "bad"}, true},
		{"invalid fallback", Gate{Name: "test", Type: GateTypeAuto, Fallback: "bad"}, true},
		{"negative max_attempts", Gate{Name: "test", Type: GateTypeAuto, MaxAttempts: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.gate.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateGates(t *testing.T) {
	t.Run("valid chain", func(t *testing.T) {
		gates := []Gate{
			{Name: "lint", Type: GateTypeAuto},
			{Name: "review", Type: GateTypeOwnerReview},
		}
		if err := ValidateGates(gates); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("duplicate names", func(t *testing.T) {
		gates := []Gate{
			{Name: "lint", Type: GateTypeAuto},
			{Name: "lint", Type: GateTypeOwnerReview},
		}
		err := ValidateGates(gates)
		if err == nil {
			t.Error("expected error for duplicate names")
		}
	})

	t.Run("empty chain is valid", func(t *testing.T) {
		if err := ValidateGates(nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
```

**Step 3: 验证**

Run: `go test ./internal/core/... -run "TestNewGateCheckID|TestGateValidate|TestValidateGates" -v`
Expected: ALL PASS

**Step 4: Commit**

```
feat(core): add Gate model with validation and GateCheck record type
```

---

### Task 3: Store 接口扩展 — Decision + GateCheck

**Files:**
- Modify: `internal/core/store.go`

**Step 1:** 在 `SaveTaskStep` 块之后、`ListEvents` 之前，添加：

```go
	// Decision versioning.
	SaveDecision(d *Decision) error
	GetDecision(id string) (*Decision, error)
	ListDecisions(issueID string) ([]Decision, error)

	// Gate checks.
	SaveGateCheck(gc *GateCheck) error
	GetGateChecks(issueID string) ([]GateCheck, error)
	GetLatestGateCheck(issueID, gateName string) (*GateCheck, error)
```

**Step 2: 验证**

Run: `go build ./internal/core/...`
Expected: BUILD SUCCESS

**Step 3: Commit**

```
feat(core): add Decision and GateCheck methods to Store interface
```

---

### Task 4: TaskStep 新增 Gate 相关 actions

**Files:**
- Modify: `internal/core/task_step.go`

**Step 1:** 在 Run-level actions 块中（`StepRunFailed` 之后），添加：

```go
// Gate-level actions (do not change Issue.Status).
const (
	StepGateCheck  TaskStepAction = "gate_check"
	StepGatePassed TaskStepAction = "gate_passed"
	StepGateFailed TaskStepAction = "gate_failed"
)
```

**Step 2:** 在 `validTaskStepActions` map 中添加这三个 action：

```go
	StepGateCheck: {}, StepGatePassed: {}, StepGateFailed: {},
```

注意：**不要**在 `actionToStatus` map 中添加这些——它们是 run-level actions，不改变 Issue.Status。

**Step 3: 验证**

Run: `go test ./internal/core/... -v`
Expected: ALL PASS

**Step 4: Commit**

```
feat(core): add gate_check/gate_passed/gate_failed TaskStep actions
```

---

### Task 5: SQLite Migration V11 — decisions 表

**Files:**
- Modify: `internal/plugins/store-sqlite/migrations.go`

**Step 1:** 在 `migrateAddChatSessionAgentName` 函数之后添加：

```go
func migrateAddDecisions(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS decisions (
	id               TEXT PRIMARY KEY,
	issue_id         TEXT NOT NULL,
	run_id           TEXT NOT NULL DEFAULT '',
	stage_id         TEXT NOT NULL DEFAULT '',
	agent_id         TEXT NOT NULL DEFAULT '',
	type             TEXT NOT NULL,
	prompt_hash      TEXT NOT NULL,
	prompt_preview   TEXT NOT NULL DEFAULT '',
	model            TEXT NOT NULL DEFAULT '',
	template         TEXT NOT NULL DEFAULT '',
	template_version TEXT NOT NULL DEFAULT '',
	input_tokens     INTEGER NOT NULL DEFAULT 0,
	action           TEXT NOT NULL,
	reasoning        TEXT NOT NULL DEFAULT '',
	confidence       REAL NOT NULL DEFAULT 0,
	output_tokens    INTEGER NOT NULL DEFAULT 0,
	output_data      TEXT NOT NULL DEFAULT '{}',
	duration_ms      INTEGER NOT NULL DEFAULT 0,
	created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_decisions_issue ON decisions(issue_id, created_at);
CREATE INDEX IF NOT EXISTS idx_decisions_type  ON decisions(type, created_at);
`)
	return err
}
```

**Step 2:** 在 `applyMigrations` 的 `currentVersion < 10` 块之后添加：

```go
	if currentVersion < 11 {
		if err := migrateAddDecisions(db); err != nil {
			return fmt.Errorf("migration v11 (decisions): %w", err)
		}
	}
```

**Step 3:** 将 `const schemaVersion = 10` 改为 `const schemaVersion = 11`（暂时改为 11，Task 6 会改为 12）

**Step 4: 验证**

Run: `go build ./internal/plugins/store-sqlite/...`
Expected: BUILD FAIL（缺 Store 接口实现，预期行为）

**Step 5: Commit**

```
feat(store): add migration V11 for decisions table
```

---

### Task 6: SQLite Migration V12 — gate_checks 表

**Files:**
- Modify: `internal/plugins/store-sqlite/migrations.go`

**Step 1:** 在 `migrateAddDecisions` 之后添加：

```go
func migrateAddGateChecks(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS gate_checks (
	id          TEXT PRIMARY KEY,
	issue_id    TEXT NOT NULL,
	gate_name   TEXT NOT NULL,
	gate_type   TEXT NOT NULL,
	attempt     INTEGER NOT NULL DEFAULT 1,
	status      TEXT NOT NULL DEFAULT 'pending',
	reason      TEXT NOT NULL DEFAULT '',
	decision_id TEXT NOT NULL DEFAULT '',
	checked_by  TEXT NOT NULL DEFAULT '',
	created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_gate_checks_issue ON gate_checks(issue_id, created_at);
CREATE INDEX IF NOT EXISTS idx_gate_checks_name  ON gate_checks(issue_id, gate_name);
`)
	return err
}
```

**Step 2:** 在 `currentVersion < 11` 块之后添加：

```go
	if currentVersion < 12 {
		if err := migrateAddGateChecks(db); err != nil {
			return fmt.Errorf("migration v12 (gate_checks): %w", err)
		}
	}
```

**Step 3:** 将 `const schemaVersion = 11` 改为 `const schemaVersion = 12`

**Step 4: Commit**

```
feat(store): add migration V12 for gate_checks table
```

---

### Task 7: SQLite Store 实现 — Decision CRUD

**Files:**
- Modify: `internal/plugins/store-sqlite/store.go`

**Step 1:** 在 `GetReviewRecords` 方法之后添加 `SaveDecision`, `GetDecision`, `ListDecisions` 三个方法。

代码参考 D1 计划文档 Task 5（`docs/plans/2026-03-09-decision-versioning-plan.md` Step 1）。模式与 `SaveReviewRecord`/`GetReviewRecords` 保持一致。

关键点：
- SaveDecision: INSERT INTO decisions (19 个字段)
- GetDecision: SELECT ... FROM decisions WHERE id=? → 单行 Scan
- ListDecisions: SELECT ... FROM decisions WHERE issue_id=? ORDER BY created_at → 遍历 rows

**Step 2: Commit**

```
feat(store): implement Decision CRUD for SQLite
```

---

### Task 8: SQLite Store 实现 — GateCheck CRUD

**Files:**
- Modify: `internal/plugins/store-sqlite/store.go`

**Step 1:** 在 Decision 方法之后添加：

```go
func (s *SQLiteStore) SaveGateCheck(gc *core.GateCheck) error {
	_, err := s.db.Exec(
		`INSERT INTO gate_checks (id, issue_id, gate_name, gate_type, attempt, status, reason, decision_id, checked_by, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		gc.ID, gc.IssueID, gc.GateName, string(gc.GateType), gc.Attempt,
		string(gc.Status), gc.Reason, gc.DecisionID, gc.CheckedBy, gc.CreatedAt,
	)
	return err
}

func (s *SQLiteStore) GetGateChecks(issueID string) ([]core.GateCheck, error) {
	rows, err := s.db.Query(
		`SELECT id, issue_id, gate_name, gate_type, attempt, status, reason, decision_id, checked_by, created_at
		 FROM gate_checks WHERE issue_id=? ORDER BY created_at`, issueID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []core.GateCheck
	for rows.Next() {
		var gc core.GateCheck
		var gateType, status string
		if err := rows.Scan(&gc.ID, &gc.IssueID, &gc.GateName, &gateType, &gc.Attempt,
			&status, &gc.Reason, &gc.DecisionID, &gc.CheckedBy, &gc.CreatedAt); err != nil {
			return nil, err
		}
		gc.GateType = core.GateType(gateType)
		gc.Status = core.GateStatus(status)
		out = append(out, gc)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetLatestGateCheck(issueID, gateName string) (*core.GateCheck, error) {
	row := s.db.QueryRow(
		`SELECT id, issue_id, gate_name, gate_type, attempt, status, reason, decision_id, checked_by, created_at
		 FROM gate_checks WHERE issue_id=? AND gate_name=? ORDER BY created_at DESC LIMIT 1`,
		issueID, gateName,
	)
	var gc core.GateCheck
	var gateType, status string
	err := row.Scan(&gc.ID, &gc.IssueID, &gc.GateName, &gateType, &gc.Attempt,
		&status, &gc.Reason, &gc.DecisionID, &gc.CheckedBy, &gc.CreatedAt)
	if err != nil {
		return nil, err
	}
	gc.GateType = core.GateType(gateType)
	gc.Status = core.GateStatus(status)
	return &gc, nil
}
```

**Step 2: Commit**

```
feat(store): implement GateCheck CRUD for SQLite
```

---

### Task 9: Store 集成测试 — Decision + GateCheck

**Files:**
- Create: `internal/plugins/store-sqlite/decision_test.go`
- Create: `internal/plugins/store-sqlite/gate_check_test.go`

**Step 1:** 创建 `decision_test.go`，参考 D1 计划 Task 6 的测试代码。用 `New(":memory:")` 创建 store。

**Step 2:** 创建 `gate_check_test.go`：

```go
package storesqlite

import (
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestGateCheckCRUD(t *testing.T) {
	store, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close()

	project := &core.Project{ID: "proj-gc", Name: "test", RepoPath: t.TempDir()}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	issue := &core.Issue{
		ID: "issue-gc-test", ProjectID: project.ID,
		Title: "gate test", Template: "standard",
		Status: core.IssueStatusDraft, State: core.IssueStateOpen,
	}
	if err := store.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	gc1 := &core.GateCheck{
		ID: core.NewGateCheckID(), IssueID: issue.ID,
		GateName: "lint", GateType: core.GateTypeAuto,
		Attempt: 1, Status: core.GateStatusPassed,
		Reason: "all checks passed", CheckedBy: "auto",
		CreatedAt: time.Now(),
	}
	if err := store.SaveGateCheck(gc1); err != nil {
		t.Fatalf("SaveGateCheck: %v", err)
	}

	gc2 := &core.GateCheck{
		ID: core.NewGateCheckID(), IssueID: issue.ID,
		GateName: "review", GateType: core.GateTypeOwnerReview,
		Attempt: 1, Status: core.GateStatusPending,
		Reason: "", CheckedBy: "human",
		CreatedAt: time.Now(),
	}
	if err := store.SaveGateCheck(gc2); err != nil {
		t.Fatalf("SaveGateCheck(gc2): %v", err)
	}

	// GetGateChecks
	checks, err := store.GetGateChecks(issue.ID)
	if err != nil {
		t.Fatalf("GetGateChecks: %v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(checks))
	}

	// GetLatestGateCheck
	latest, err := store.GetLatestGateCheck(issue.ID, "lint")
	if err != nil {
		t.Fatalf("GetLatestGateCheck: %v", err)
	}
	if latest.Status != core.GateStatusPassed {
		t.Errorf("expected passed, got %q", latest.Status)
	}

	// Not found
	_, err = store.GetLatestGateCheck(issue.ID, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent gate")
	}
}
```

**Step 3: 验证**

Run: `go test ./internal/plugins/store-sqlite/... -run "TestDecisionCRUD|TestGateCheckCRUD" -v`
Expected: ALL PASS

**Step 4: 验证全量编译**

Run: `go build ./...`
Expected: BUILD SUCCESS（如果有其他包依赖 Store 接口的 mock，可能需要更新）

**Step 5: Commit**

```
test(store): add integration tests for Decision and GateCheck CRUD
```

---

### Task 10: 修复编译 — 补全 Store mock/stub

**Files:**
- 需根据编译错误判断。常见位置：
  - `internal/teamleader/*_test.go` 中的 mock store
  - `internal/web/*_test.go` 中的 mock store
  - 任何实现 `core.Store` 接口的地方

**Step 1:** 运行 `go build ./...`，收集所有编译错误

**Step 2:** 为每个 mock/stub 添加新方法的空实现：

```go
func (m *mockStore) SaveDecision(d *core.Decision) error              { return nil }
func (m *mockStore) GetDecision(id string) (*core.Decision, error)    { return nil, nil }
func (m *mockStore) ListDecisions(issueID string) ([]core.Decision, error) { return nil, nil }
func (m *mockStore) SaveGateCheck(gc *core.GateCheck) error           { return nil }
func (m *mockStore) GetGateChecks(issueID string) ([]core.GateCheck, error) { return nil, nil }
func (m *mockStore) GetLatestGateCheck(issueID, gateName string) (*core.GateCheck, error) { return nil, nil }
```

**Step 3: 验证**

Run: `go build ./... && go test ./internal/core/... -v`
Expected: ALL BUILD + ALL PASS

**Step 4: Commit**

```
fix: add Decision and GateCheck stubs to Store mocks for compilation
```

---

## Wave 2: 执行引擎

### Task 11: WorkflowProfile 扩展 Gates 字段 + 默认 Gate 链

**Files:**
- Modify: `internal/core/workflow_profile.go`

**Step 1:** 在 `WorkflowProfile` struct 中添加 `Gates` 字段：

```go
type WorkflowProfile struct {
	Type       WorkflowProfileType `json:"type"`
	SLAMinutes int                 `json:"sla_minutes"`
	Gates      []Gate              `json:"gates,omitempty"`
}
```

**Step 2:** 添加默认 Gate 链映射和解析函数：

```go
var defaultProfileGates = map[WorkflowProfileType][]Gate{
	WorkflowProfileNormal: {
		{Name: "demand_review", Type: GateTypeAuto, Rules: "需求完整性和可行性检查", MaxAttempts: 2, Fallback: GateFallbackEscalate},
	},
	WorkflowProfileStrict: {
		{Name: "demand_review", Type: GateTypeAuto, Rules: "需求完整性和可行性检查", MaxAttempts: 2, Fallback: GateFallbackEscalate},
		{Name: "peer_review", Type: GateTypePeerReview, Rules: "代码和方案质量互审", MaxAttempts: 3, Fallback: GateFallbackEscalate},
	},
	WorkflowProfileFastRelease: {
		{Name: "auto_pass", Type: GateTypeAuto, Rules: "快速通过，仅检查基本格式", MaxAttempts: 1, Fallback: GateFallbackForcePass},
	},
}

// ResolveGates returns the gate chain for this profile.
// If Gates is explicitly set, use that; otherwise use defaults.
func (p WorkflowProfile) ResolveGates() []Gate {
	if len(p.Gates) > 0 {
		return p.Gates
	}
	if gates, ok := defaultProfileGates[p.Type]; ok {
		return gates
	}
	return defaultProfileGates[WorkflowProfileNormal]
}
```

**Step 3:** 在 `Validate()` 中添加 gates 验证：

在现有 SLAMinutes 检查之后添加：
```go
	if err := ValidateGates(p.Gates); err != nil {
		return fmt.Errorf("gates: %w", err)
	}
```

**Step 4:** 添加测试到 `workflow_profile_test.go`：

```go
func TestResolveGates(t *testing.T) {
	t.Run("default normal", func(t *testing.T) {
		p := WorkflowProfile{Type: WorkflowProfileNormal, SLAMinutes: 10}
		gates := p.ResolveGates()
		if len(gates) != 1 || gates[0].Name != "demand_review" {
			t.Errorf("unexpected gates: %+v", gates)
		}
	})
	t.Run("default strict", func(t *testing.T) {
		p := WorkflowProfile{Type: WorkflowProfileStrict, SLAMinutes: 10}
		gates := p.ResolveGates()
		if len(gates) != 2 {
			t.Errorf("expected 2 gates, got %d", len(gates))
		}
	})
	t.Run("custom gates override", func(t *testing.T) {
		custom := []Gate{{Name: "my_gate", Type: GateTypeAuto}}
		p := WorkflowProfile{Type: WorkflowProfileNormal, SLAMinutes: 10, Gates: custom}
		gates := p.ResolveGates()
		if len(gates) != 1 || gates[0].Name != "my_gate" {
			t.Errorf("expected custom gate, got: %+v", gates)
		}
	})
}
```

**Step 5: 验证**

Run: `go test ./internal/core/... -run "TestResolveGates" -v`
Expected: PASS

**Step 6: Commit**

```
feat(core): add Gates field to WorkflowProfile with defaults and resolution
```

---

### Task 12: AutoGateRunner — 复用 DemandReviewer

**Files:**
- Create: `internal/teamleader/gate_runner.go`
- Create: `internal/teamleader/gate_runner_test.go`

**Step 1:** 创建 `gate_runner.go`

```go
package teamleader

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// AutoGateRunner evaluates an auto gate using the existing DemandReviewer.
type AutoGateRunner struct {
	Reviewer DemandReviewer
}

func (r *AutoGateRunner) Check(ctx context.Context, issue *core.Issue, gate core.Gate, attempt int) (*core.GateCheck, error) {
	if r.Reviewer == nil {
		return nil, fmt.Errorf("auto gate runner: reviewer is nil")
	}

	verdict, err := r.Reviewer.Review(ctx, cloneIssueForReview(issue))
	if err != nil {
		return &core.GateCheck{
			ID:        core.NewGateCheckID(),
			IssueID:   issue.ID,
			GateName:  gate.Name,
			GateType:  gate.Type,
			Attempt:   attempt,
			Status:    core.GateStatusFailed,
			Reason:    fmt.Sprintf("review error: %v", err),
			CheckedBy: "auto",
			CreatedAt: time.Now(),
		}, nil
	}

	status := core.GateStatusPassed
	if verdictNeedsFix(verdict) {
		status = core.GateStatusFailed
	}

	return &core.GateCheck{
		ID:        core.NewGateCheckID(),
		IssueID:   issue.ID,
		GateName:  gate.Name,
		GateType:  gate.Type,
		Attempt:   attempt,
		Status:    status,
		Reason:    verdict.Summary,
		CheckedBy: "auto",
		CreatedAt: time.Now(),
	}, nil
}

// OwnerReviewRunner creates a pending gate check that waits for human resolution.
type OwnerReviewRunner struct{}

func (r *OwnerReviewRunner) Check(_ context.Context, issue *core.Issue, gate core.Gate, attempt int) (*core.GateCheck, error) {
	return &core.GateCheck{
		ID:        core.NewGateCheckID(),
		IssueID:   issue.ID,
		GateName:  gate.Name,
		GateType:  gate.Type,
		Attempt:   attempt,
		Status:    core.GateStatusPending,
		Reason:    "awaiting owner review",
		CheckedBy: "human",
		CreatedAt: time.Now(),
	}, nil
}
```

**Step 2:** 创建 `gate_runner_test.go`

```go
package teamleader

import (
	"context"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

type stubDemandReviewer struct {
	verdict core.ReviewVerdict
	err     error
}

func (s *stubDemandReviewer) Review(_ context.Context, _ *core.Issue) (core.ReviewVerdict, error) {
	return s.verdict, s.err
}

func TestAutoGateRunner_Pass(t *testing.T) {
	runner := &AutoGateRunner{
		Reviewer: &stubDemandReviewer{
			verdict: core.ReviewVerdict{Status: "pass", Score: 90, Summary: "looks good"},
		},
	}
	issue := &core.Issue{ID: "issue-1", Title: "test", Template: "standard"}
	gate := core.Gate{Name: "lint", Type: core.GateTypeAuto}

	check, err := runner.Check(context.Background(), issue, gate, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if check.Status != core.GateStatusPassed {
		t.Errorf("expected passed, got %q", check.Status)
	}
}

func TestAutoGateRunner_Fail(t *testing.T) {
	runner := &AutoGateRunner{
		Reviewer: &stubDemandReviewer{
			verdict: core.ReviewVerdict{
				Status: "issues_found", Score: 40,
				Issues: []core.ReviewIssue{{Description: "missing tests"}},
			},
		},
	}
	issue := &core.Issue{ID: "issue-2", Title: "test", Template: "standard"}
	gate := core.Gate{Name: "lint", Type: core.GateTypeAuto}

	check, err := runner.Check(context.Background(), issue, gate, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if check.Status != core.GateStatusFailed {
		t.Errorf("expected failed, got %q", check.Status)
	}
}

func TestOwnerReviewRunner_Pending(t *testing.T) {
	runner := &OwnerReviewRunner{}
	issue := &core.Issue{ID: "issue-3", Title: "test", Template: "standard"}
	gate := core.Gate{Name: "owner", Type: core.GateTypeOwnerReview}

	check, err := runner.Check(context.Background(), issue, gate, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if check.Status != core.GateStatusPending {
		t.Errorf("expected pending, got %q", check.Status)
	}
}
```

**Step 3: 验证**

Run: `go test ./internal/teamleader/... -run "TestAutoGateRunner|TestOwnerReviewRunner" -v`
Expected: ALL PASS

**Step 4: Commit**

```
feat(teamleader): add AutoGateRunner and OwnerReviewRunner implementations
```

---

### Task 13: GateChain 编排器

**Files:**
- Create: `internal/teamleader/gate_chain.go`
- Create: `internal/teamleader/gate_chain_test.go`

**Step 1:** 创建 `gate_chain.go`

```go
package teamleader

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// GateStore is the persistence interface for gate checks.
type GateStore interface {
	SaveGateCheck(gc *core.GateCheck) error
	GetGateChecks(issueID string) ([]core.GateCheck, error)
	GetLatestGateCheck(issueID, gateName string) (*core.GateCheck, error)
	SaveTaskStep(step *core.TaskStep) (core.IssueStatus, error)
}

// GateChain executes a sequence of gates for an issue.
type GateChain struct {
	Store   GateStore
	Runners map[core.GateType]core.GateRunner
}

// GateChainResult holds the outcome of running a gate chain.
type GateChainResult struct {
	AllPassed   bool             // all gates passed
	PendingGate string           // non-empty if waiting for human
	FailedCheck *core.GateCheck  // non-nil if a gate failed with fallback=abort/escalate
	ForcePassed bool             // true if fallback=force_pass was applied
}

// Run executes gates sequentially. Returns result indicating outcome.
func (c *GateChain) Run(ctx context.Context, issue *core.Issue, gates []core.Gate) (*GateChainResult, error) {
	if len(gates) == 0 {
		return &GateChainResult{AllPassed: true}, nil
	}

	for _, gate := range gates {
		runner, ok := c.Runners[gate.Type]
		if !ok {
			slog.Warn("no runner for gate type, skipping", "gate", gate.Name, "type", gate.Type)
			c.recordStep(issue.ID, gate.Name, core.StepGatePassed, "skipped: no runner for type "+string(gate.Type))
			continue
		}

		for attempt := 1; ; attempt++ {
			check, err := runner.Check(ctx, issue, gate, attempt)
			if err != nil {
				return nil, fmt.Errorf("gate %q attempt %d: %w", gate.Name, attempt, err)
			}

			if err := c.Store.SaveGateCheck(check); err != nil {
				slog.Error("failed to save gate check", "err", err)
			}

			switch check.Status {
			case core.GateStatusPassed:
				c.recordStep(issue.ID, gate.Name, core.StepGatePassed, check.Reason)
				goto nextGate

			case core.GateStatusPending:
				c.recordStep(issue.ID, gate.Name, core.StepGateCheck, "awaiting resolution")
				return &GateChainResult{PendingGate: gate.Name}, nil

			case core.GateStatusFailed:
				c.recordStep(issue.ID, gate.Name, core.StepGateCheck, fmt.Sprintf("attempt %d failed: %s", attempt, check.Reason))
				if gate.MaxAttempts > 0 && attempt >= gate.MaxAttempts {
					return c.applyFallback(issue.ID, gate, check)
				}
				// continue retrying

			default:
				return nil, fmt.Errorf("gate %q: unexpected status %q", gate.Name, check.Status)
			}
		}
	nextGate:
	}

	return &GateChainResult{AllPassed: true}, nil
}

func (c *GateChain) applyFallback(issueID string, gate core.Gate, check *core.GateCheck) (*GateChainResult, error) {
	fallback := gate.Fallback
	if fallback == "" {
		fallback = core.GateFallbackEscalate
	}

	switch fallback {
	case core.GateFallbackForcePass:
		c.recordStep(issueID, gate.Name, core.StepGatePassed, "force_pass after max attempts")
		return &GateChainResult{AllPassed: true, ForcePassed: true}, nil

	case core.GateFallbackAbort:
		c.recordStep(issueID, gate.Name, core.StepGateFailed, "aborted after max attempts")
		return &GateChainResult{FailedCheck: check}, nil

	case core.GateFallbackEscalate:
		c.recordStep(issueID, gate.Name, core.StepGateFailed, "escalated after max attempts")
		return &GateChainResult{FailedCheck: check}, nil

	default:
		c.recordStep(issueID, gate.Name, core.StepGateFailed, "unknown fallback: "+string(fallback))
		return &GateChainResult{FailedCheck: check}, nil
	}
}

func (c *GateChain) recordStep(issueID, gateName string, action core.TaskStepAction, note string) {
	step := &core.TaskStep{
		ID:        core.NewTaskStepID(),
		IssueID:   issueID,
		Action:    action,
		Note:      fmt.Sprintf("[gate:%s] %s", gateName, note),
		CreatedAt: time.Now(),
	}
	if _, err := c.Store.SaveTaskStep(step); err != nil {
		slog.Error("failed to save gate task step", "err", err, "gate", gateName)
	}
}
```

**Step 2:** 创建 `gate_chain_test.go` — 测试 all-pass、pending、fail+fallback 三种场景

关键测试用例：
1. **TestGateChain_AllPass** — 两道 auto gate 都通过
2. **TestGateChain_PendingOwner** — auto 通过 + owner_review pending
3. **TestGateChain_FailEscalate** — auto fail + max_attempts=1 + escalate
4. **TestGateChain_FailForcePass** — auto fail + max_attempts=1 + force_pass
5. **TestGateChain_EmptyGates** — 空 gate 链直接 AllPassed

使用 stubDemandReviewer + 内联 GateStore mock 实现测试。

**Step 3: 验证**

Run: `go test ./internal/teamleader/... -run "TestGateChain" -v`
Expected: ALL PASS

**Step 4: Commit**

```
feat(teamleader): add GateChain sequential gate executor with fallback handling
```

---

### Task 14: Manager 集成 GateChain

**Files:**
- Modify: `internal/teamleader/manager.go`

**Step 1:** 在 Manager struct 中添加 `gateChain *GateChain` 字段

**Step 2:** 添加 `SetGateChain(gc *GateChain)` 方法

**Step 3:** 修改 `submitIssues()` 方法，在 gateChain 不为 nil 时优先使用：

```go
func (m *Manager) submitIssues(ctx context.Context, issues []*core.Issue) error {
	// 新路径: GateChain
	if m.gateChain != nil {
		for _, issue := range issues {
			profile := workflowProfileFromIssue(issue)
			wp := core.WorkflowProfile{Type: profile, SLAMinutes: 10}
			gates := wp.ResolveGates()

			result, err := m.gateChain.Run(ctx, issue, gates)
			if err != nil {
				return fmt.Errorf("gate chain for issue %s: %w", issue.ID, err)
			}
			if result.AllPassed {
				m.applyIssueApprove(ctx, issue)
			} else if result.PendingGate != "" {
				// 等待人工介入，不自动 approve
			} else if result.FailedCheck != nil {
				m.applyIssueReject(ctx, issue)
			}
		}
		return nil
	}

	// 旧路径: fallback（保持现有代码不变）
	switch {
	case m.twoPhaseReview != nil:
		// ... 现有代码
	}
}
```

**注意：** 此 Task 需要仔细阅读 `manager.go` 中现有的 `submitIssues` 实现，确保不破坏现有逻辑。旧路径代码完全保留，只在 gateChain != nil 时走新路径。

**Step 4:** 在 `cmd/ai-flow/` 的启动路径中构建并注入 GateChain（如果 review 相关配置存在）

需要在 server 启动时：
```go
gateChain := &teamleader.GateChain{
	Store: store,  // store 实现了 GateStore 接口
	Runners: map[core.GateType]core.GateRunner{
		core.GateTypeAuto:        &teamleader.AutoGateRunner{Reviewer: demandReviewer},
		core.GateTypeOwnerReview: &teamleader.OwnerReviewRunner{},
	},
}
manager.SetGateChain(gateChain)
```

**Step 5: 验证**

Run: `go build ./... && go test ./internal/teamleader/... -v`
Expected: BUILD SUCCESS + ALL PASS

**Step 6: Commit**

```
feat(teamleader): integrate GateChain into Manager with ReviewGate fallback
```

---

## Wave 3: API + 前端

### Task 15: REST API — Gate 查询 + 人工 resolve

**Files:**
- Create: `internal/web/handlers_gate.go`
- Modify: `internal/web/handlers_v3.go`（注册路由）

**Step 1:** 创建 `handlers_gate.go`，包含两个 handler：

1. `GET /api/v1/issues/{id}/gates` — 返回 gate 链状态（聚合 GateChecks）
2. `POST /api/v1/issues/{id}/gates/{gateName}/resolve` — 人工 resolve（pass/fail）

resolve handler 需要：
- 查找最新的 pending GateCheck
- 更新其 Status 和 Reason
- 保存新的 GateCheck 记录（覆盖 pending）
- 记录 TaskStep
- 如果全部 gate 都通过，通知 Manager 执行 applyIssueApprove

**Step 2:** 在 `registerV1Routes` 中注册路由

**Step 3: 验证**

Run: `go build ./internal/web/...`
Expected: BUILD SUCCESS

**Step 4: Commit**

```
feat(api): add GET /issues/{id}/gates and POST /issues/{id}/gates/{gateName}/resolve
```

---

### Task 16: Decision API

**Files:**
- Create: `internal/web/handlers_decisions.go`
- Modify: `internal/web/handlers_v3.go`

**Step 1:** 创建 `handlers_decisions.go`，参考 D1 计划 Task 7 的代码

两个端点：
1. `GET /api/v1/issues/{id}/decisions` — 列出 issue 的所有 Decision
2. `GET /api/v1/decisions/{id}` — 获取单个 Decision 详情

**Step 2:** 注册路由

**Step 3: Commit**

```
feat(api): add Decision list and detail endpoints
```

---

### Task 17: 前端类型 + API 客户端

**Files:**
- Modify: `web/src/types/workflow.ts` 或 `web/src/types/api.ts`
- Modify: `web/src/lib/apiClient.ts`

**Step 1:** 添加 TypeScript 类型：

```typescript
// Gate 相关
export interface Gate {
  name: string;
  type: "auto" | "owner_review" | "peer_review" | "vote";
  rules: string;
  max_attempts?: number;
  fallback?: "escalate" | "force_pass" | "abort";
}

export interface GateCheck {
  id: string;
  issue_id: string;
  gate_name: string;
  gate_type: string;
  attempt: number;
  status: "pending" | "passed" | "failed" | "skipped";
  reason: string;
  decision_id?: string;
  checked_by: string;
  created_at: string;
}

export interface GateStatus {
  name: string;
  type: string;
  status: string;
  attempts: number;
  checks: GateCheck[];
}

// Decision
export interface Decision {
  id: string;
  issue_id: string;
  run_id?: string;
  stage_id?: string;
  agent_id: string;
  type: string;
  prompt_hash: string;
  prompt_preview: string;
  model: string;
  action: string;
  reasoning: string;
  confidence: number;
  duration_ms: number;
  created_at: string;
}
```

**Step 2:** 添加 API 函数：

```typescript
fetchIssueGates(issueId: string): Promise<{gates: GateStatus[]}>
resolveGate(issueId: string, gateName: string, action: "pass" | "fail", reason: string): Promise<void>
fetchIssueDecisions(issueId: string): Promise<Decision[]>
```

**Step 3: 验证**

Run: `npm --prefix web run typecheck`
Expected: NO ERRORS

**Step 4: Commit**

```
feat(web): add Gate and Decision TypeScript types and API client methods
```

---

### Task 18: IssueFlowTree 展示 gate_check 事件

**Files:**
- Modify: `web/src/components/IssueFlowTree.tsx` 或相关组件

**Step 1:** 在 TaskStep 渲染逻辑中，识别 `gate_check`/`gate_passed`/`gate_failed` action：

- `gate_passed` → 绿色 ✓ 图标 + gate name
- `gate_failed` → 红色 ✗ 图标 + gate name + reason
- `gate_check` → 黄色 ⏳ 图标 + gate name（pending/checking）

从 TaskStep.Note 中提取 `[gate:xxx]` 部分作为 gate 名称展示。

**Step 2: 验证**

Run: `npm --prefix web run typecheck`
Expected: NO ERRORS

**Step 3: Commit**

```
feat(web): display gate check events in IssueFlowTree with status icons
```

---

### Task 19: 全量测试 + 编译验证

**Step 1: 后端**

Run: `go build ./... && go test ./internal/core/... ./internal/teamleader/... ./internal/plugins/store-sqlite/... -v`
Expected: ALL PASS

**Step 2: 前端**

Run: `npm --prefix web run typecheck && npm --prefix web run test`
Expected: ALL PASS

**Step 3: 如有失败，修复后 commit**

```
fix: address test failures from gate integration
```

---

## 验收检查清单

- [ ] `go build ./...` 无错误
- [ ] `go test ./internal/core/...` 全通过
- [ ] `go test ./internal/teamleader/...` 全通过
- [ ] `go test ./internal/plugins/store-sqlite/...` 全通过
- [ ] `npm --prefix web run typecheck` 无错误
- [ ] WorkflowProfile normal → 1 道 auto gate，行为与当前审查一致
- [ ] WorkflowProfile strict → 2 道 gate
- [ ] 自定义 Gates 可正常执行
- [ ] gate_check TaskStep 在 IssueFlowTree 中可见
- [ ] owner_review gate 可通过 API resolve
- [ ] 现有 review 测试继续通过（向后兼容）
