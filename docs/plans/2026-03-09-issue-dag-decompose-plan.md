# Issue DAG 拆解 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 用户一句话输入需求，Team Leader 自动拆成 Issue DAG，前端可视化预览编辑后批量创建，按严格依赖调度执行。

**Architecture:** 新增 decompose planner 调用 LLM 产出 Proposal JSON，通过两阶段 API (decompose → confirm) 分离方案生成和 Issue 创建。前端用已安装的 @xyflow/react 渲染 DAG 预览。启用现有但被禁用的 DependsOn/Blocks 字段，加强 scheduler 的依赖检查。

**Tech Stack:** Go / ACP (LLM) / chi router / React / @xyflow/react / Tailwind / TypeScript

**Design doc:** `docs/plans/2026-03-09-issue-dag-decompose-design.md`

**Dependencies:** TaskStep 事件溯源（v3）合并后可选集成，但不阻塞本计划。

---

## 前置知识

### 已有基础设施（不需要重新实现）

- `@xyflow/react` v12.10.1 已安装（`web/package.json:15`）
- `IssueDagNode`/`IssueDagEdge`/`IssueDagResponse` 类型已定义（`web/src/types/api.ts`）
- `apiClient.getIssueDag()` 方法已实现（`web/src/lib/apiClient.ts:802-805`）
- `DecomposeFunc` 回调模式已有（`internal/teamleader/decompose_handler.go:26-28`）
- `Manager.CreateIssues()` 已支持批量创建（`internal/teamleader/manager.go:145-218`）
- Issue 模型已有 `DependsOn`/`Blocks` 字段（`internal/core/issue.go:136-137`）

### 当前限制（需要修复）

- `DependsOn`/`Blocks` 在 V2 中被硬编码为 nil（`manager.go:192-194`）
- `markReadyByProfileQueueLocked()` 不检查依赖（`scheduler_dispatch.go:214-257`）
- 没有 decompose API 端点
- 没有 DAG 预览组件

---

## Task 1: Proposal 数据模型

**Files:**
- Create: `internal/core/proposal.go`
- Test: `internal/core/proposal_test.go`

**Step 1: Write the test**

```go
// internal/core/proposal_test.go
package core

import (
	"testing"
)

func TestProposalValidate(t *testing.T) {
	valid := DecomposeProposal{
		ID:      "prop-20260309-abcd",
		Summary: "用户注册系统",
		Items: []ProposalItem{
			{TempID: "A", Title: "设计 DB schema", Body: "...", DependsOn: nil},
			{TempID: "B", Title: "实现注册 API", Body: "...", DependsOn: []string{"A"}},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Empty items
	empty := valid
	empty.Items = nil
	if err := empty.Validate(); err == nil {
		t.Fatal("expected error for empty items")
	}

	// Duplicate temp_id
	dup := valid
	dup.Items = append(dup.Items, ProposalItem{TempID: "A", Title: "dup"})
	if err := dup.Validate(); err == nil {
		t.Fatal("expected error for duplicate temp_id")
	}

	// Missing dependency reference
	badDep := DecomposeProposal{
		ID:      "prop-xxx",
		Summary: "test",
		Items: []ProposalItem{
			{TempID: "A", Title: "task A", DependsOn: []string{"Z"}},
		},
	}
	if err := badDep.Validate(); err == nil {
		t.Fatal("expected error for missing dependency")
	}
}

func TestProposalDetectCycle(t *testing.T) {
	cyclic := DecomposeProposal{
		ID:      "prop-xxx",
		Summary: "test",
		Items: []ProposalItem{
			{TempID: "A", Title: "A", DependsOn: []string{"B"}},
			{TempID: "B", Title: "B", DependsOn: []string{"A"}},
		},
	}
	if err := cyclic.Validate(); err == nil {
		t.Fatal("expected error for cyclic dependency")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestProposal -v`
Expected: FAIL (types not defined)

**Step 3: Write the implementation**

```go
// internal/core/proposal.go
package core

import (
	"fmt"
	"strings"
	"time"
)

// DecomposeProposal is a draft DAG of issues produced by Team Leader,
// pending user review before actual Issue creation.
type DecomposeProposal struct {
	ID        string         `json:"proposal_id"`
	ProjectID string         `json:"project_id"`
	Prompt    string         `json:"prompt"`
	Summary   string         `json:"summary"`
	Items     []ProposalItem `json:"issues"`
	CreatedAt time.Time      `json:"created_at"`
}

// ProposalItem is one node in the proposed DAG.
type ProposalItem struct {
	TempID    string   `json:"temp_id"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Labels    []string `json:"labels"`
	DependsOn []string `json:"depends_on"`
	Template  string   `json:"template,omitempty"`
	AutoMerge *bool    `json:"auto_merge,omitempty"`
}

// Validate checks the proposal for structural integrity:
// non-empty items, unique temp_ids, valid dependency refs, no cycles.
func (p DecomposeProposal) Validate() error {
	if len(p.Items) == 0 {
		return fmt.Errorf("proposal must have at least one item")
	}
	ids := make(map[string]struct{}, len(p.Items))
	for _, item := range p.Items {
		id := strings.TrimSpace(item.TempID)
		if id == "" {
			return fmt.Errorf("proposal item missing temp_id")
		}
		if strings.TrimSpace(item.Title) == "" {
			return fmt.Errorf("proposal item %q missing title", id)
		}
		if _, dup := ids[id]; dup {
			return fmt.Errorf("duplicate temp_id %q", id)
		}
		ids[id] = struct{}{}
	}
	// Check dependency references exist.
	for _, item := range p.Items {
		for _, dep := range item.DependsOn {
			if _, ok := ids[dep]; !ok {
				return fmt.Errorf("item %q depends on unknown temp_id %q", item.TempID, dep)
			}
		}
	}
	// Check for cycles via topological sort.
	return p.detectCycle()
}

func (p DecomposeProposal) detectCycle() error {
	inDegree := make(map[string]int, len(p.Items))
	adj := make(map[string][]string, len(p.Items))
	for _, item := range p.Items {
		inDegree[item.TempID] += 0 // ensure key exists
		for _, dep := range item.DependsOn {
			adj[dep] = append(adj[dep], item.TempID)
			inDegree[item.TempID]++
		}
	}
	queue := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, next := range adj[node] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if visited != len(p.Items) {
		return fmt.Errorf("cyclic dependency detected in proposal")
	}
	return nil
}

// NewProposalID generates a proposal ID.
func NewProposalID() string {
	return fmt.Sprintf("prop-%s-%s", time.Now().Format("20060102"), randomHex(4))
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestProposal -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/core/proposal.go internal/core/proposal_test.go
git commit -m "feat(core): add DecomposeProposal model with DAG validation"
```

---

## Task 2: Decompose Planner — LLM 调用产出 Proposal

**Files:**
- Create: `internal/teamleader/decompose_planner.go`
- Test: `internal/teamleader/decompose_planner_test.go`

**Step 1: Write the test**

```go
// internal/teamleader/decompose_planner_test.go
package teamleader

import (
	"context"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestParseDecomposeResponse(t *testing.T) {
	raw := `{
		"summary": "用户注册系统",
		"issues": [
			{"temp_id": "A", "title": "设计DB schema", "body": "设计用户表", "depends_on": [], "labels": ["backend"]},
			{"temp_id": "B", "title": "注册API", "body": "POST /register", "depends_on": ["A"], "labels": ["backend"]}
		]
	}`
	proposal, err := parseDecomposeResponse("proj-1", "做用户注册", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proposal.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(proposal.Items))
	}
	if proposal.Items[1].DependsOn[0] != "A" {
		t.Fatalf("expected B depends on A")
	}
	if proposal.Summary != "用户注册系统" {
		t.Fatalf("summary = %q", proposal.Summary)
	}
}

func TestParseDecomposeResponse_ExtractJSON(t *testing.T) {
	// LLM might wrap JSON in markdown code block
	raw := "这是我的分析：\n```json\n{\"summary\":\"test\",\"issues\":[{\"temp_id\":\"A\",\"title\":\"t\",\"body\":\"b\",\"depends_on\":[]}]}\n```\n以上是方案。"
	proposal, err := parseDecomposeResponse("proj-1", "prompt", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proposal.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(proposal.Items))
	}
}

func TestParseDecomposeResponse_Invalid(t *testing.T) {
	_, err := parseDecomposeResponse("proj-1", "prompt", "not json at all")
	if err == nil {
		t.Fatal("expected error for invalid response")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/teamleader/ -run TestParseDecomposeResponse -v`
Expected: FAIL (function not defined)

**Step 3: Write the implementation**

```go
// internal/teamleader/decompose_planner.go
package teamleader

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

// decomposeSystemPrompt is sent to the Team Leader LLM for DAG decomposition.
const decomposeSystemPrompt = `你是技术项目主管。用户给了一个需求，请分解成多个独立可执行的任务。

规则：
1. 每个任务应该是一个独立的代码变更，可以独立开发和测试
2. 明确任务之间的依赖关系（哪个任务必须先完成）
3. 尽量让无依赖的任务可以并行执行
4. 每个任务给出清晰的标题和描述，描述中包含验收标准
5. 输出纯 JSON，不要其他文字

输出格式：
{
  "summary": "方案概述（一句话）",
  "issues": [
    {
      "temp_id": "A",
      "title": "任务标题",
      "body": "任务描述，包含验收标准",
      "depends_on": [],
      "labels": ["backend"/"frontend"/"test"/...]
    }
  ]
}`

// DecomposePlanner calls LLM to produce a DecomposeProposal from a user prompt.
type DecomposePlanner struct {
	// chatFn sends a message to the Team Leader LLM and returns the reply text.
	chatFn func(ctx context.Context, systemPrompt, userMessage string) (string, error)
}

// NewDecomposePlanner creates a planner with the given chat function.
func NewDecomposePlanner(chatFn func(ctx context.Context, systemPrompt, userMessage string) (string, error)) *DecomposePlanner {
	return &DecomposePlanner{chatFn: chatFn}
}

// Plan calls the LLM and returns a validated DecomposeProposal.
func (p *DecomposePlanner) Plan(ctx context.Context, projectID, prompt string) (*core.DecomposeProposal, error) {
	reply, err := p.chatFn(ctx, decomposeSystemPrompt, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}
	proposal, err := parseDecomposeResponse(projectID, prompt, reply)
	if err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}
	if err := proposal.Validate(); err != nil {
		return nil, fmt.Errorf("invalid proposal: %w", err)
	}
	return proposal, nil
}

// parseDecomposeResponse extracts JSON from LLM reply and builds a DecomposeProposal.
func parseDecomposeResponse(projectID, prompt, raw string) (*core.DecomposeProposal, error) {
	jsonStr := extractJSON(raw)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in LLM response")
	}

	var parsed struct {
		Summary string               `json:"summary"`
		Issues  []core.ProposalItem  `json:"issues"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	proposal := &core.DecomposeProposal{
		ID:        core.NewProposalID(),
		ProjectID: projectID,
		Prompt:    prompt,
		Summary:   parsed.Summary,
		Items:     parsed.Issues,
	}
	return proposal, nil
}

// jsonBlockRe matches ```json ... ``` code blocks.
var jsonBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(\\{.*?\\})\\s*```")

// extractJSON tries to find JSON in the LLM response:
// 1. Look for ```json ... ``` code block
// 2. Look for first { ... } pair
// 3. Return raw string as-is
func extractJSON(raw string) string {
	// Try code block first.
	if m := jsonBlockRe.FindStringSubmatch(raw); len(m) > 1 {
		return m[1]
	}
	// Try raw JSON object.
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return raw[start : end+1]
	}
	return ""
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/teamleader/ -run TestParseDecomposeResponse -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/teamleader/decompose_planner.go internal/teamleader/decompose_planner_test.go
git commit -m "feat(teamleader): add DecomposePlanner with LLM response parsing"
```

---

## Task 3: 启用 DependsOn/Blocks 字段

**Files:**
- Modify: `internal/teamleader/manager.go:192-194`
- Test: verify existing tests still pass

**Step 1: Remove the hardcoded nil**

In `internal/teamleader/manager.go`, find where `CreateIssues()` sets `DependsOn: nil, Blocks: nil` and remove the hardcoded override. The Issue should preserve whatever DependsOn/Blocks values are passed in.

Look for lines like:
```go
// V2 removes runtime dependency graph; dependency fields are ignored.
DependsOn:   nil,
Blocks:      nil,
```

Change to:
```go
DependsOn:   issue.DependsOn,
Blocks:      issue.Blocks,
```

Or simply remove the override lines so the incoming values are preserved.

**Step 2: Run existing tests**

Run: `go test ./internal/teamleader/ -v -count=1`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/teamleader/manager.go
git commit -m "feat(teamleader): enable DependsOn/Blocks fields in Issue creation"
```

---

## Task 4: Scheduler 严格依赖检查

**Files:**
- Modify: `internal/teamleader/scheduler_dispatch.go`
- Test: `internal/teamleader/scheduler_dispatch_test.go` (create if needed)

**Step 1: Write the test**

```go
// internal/teamleader/scheduler_dep_test.go
package teamleader

import (
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestAreDependenciesMet(t *testing.T) {
	issues := map[string]*core.Issue{
		"A": {ID: "A", Status: core.IssueStatusDone},
		"B": {ID: "B", Status: core.IssueStatusDone},
		"C": {ID: "C", Status: core.IssueStatusExecuting},
	}

	lookup := func(id string) *core.Issue {
		return issues[id]
	}

	// All deps done → met
	if !areDependenciesMet([]string{"A", "B"}, lookup) {
		t.Fatal("expected deps A,B to be met")
	}

	// One dep not done → not met
	if areDependenciesMet([]string{"A", "C"}, lookup) {
		t.Fatal("expected deps A,C to not be met")
	}

	// No deps → met
	if !areDependenciesMet(nil, lookup) {
		t.Fatal("expected nil deps to be met")
	}

	// Unknown dep → not met
	if areDependenciesMet([]string{"Z"}, lookup) {
		t.Fatal("expected unknown dep to not be met")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/teamleader/ -run TestAreDependenciesMet -v`
Expected: FAIL (function not defined)

**Step 3: Add dependency check function and integrate into scheduler**

Add to `internal/teamleader/scheduler_dispatch.go`:

```go
// areDependenciesMet checks if all DependsOn issues are in done status.
func areDependenciesMet(dependsOn []string, lookup func(string) *core.Issue) bool {
	for _, depID := range dependsOn {
		dep := lookup(depID)
		if dep == nil || dep.Status != core.IssueStatusDone {
			return false
		}
	}
	return true
}
```

Then modify `markReadyByProfileQueueLocked()` to add a dependency gate before transitioning queued → ready.

In the loop that iterates over queued issues, add before the `transitionIssueStatus` call:

```go
// Check strict dependency: all DependsOn must be done.
if len(issue.DependsOn) > 0 {
	lookup := func(id string) *core.Issue {
		if iss, ok := rs.IssueByID[id]; ok {
			return iss
		}
		// Fallback to store for cross-session issues.
		iss, _ := s.store.GetIssue(id)
		return iss
	}
	if !areDependenciesMet(issue.DependsOn, lookup) {
		continue // skip, deps not ready
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/teamleader/ -run TestAreDependenciesMet -v`
Expected: PASS

**Step 5: Run all teamleader tests**

Run: `go test ./internal/teamleader/ -v -count=1`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/teamleader/scheduler_dispatch.go internal/teamleader/scheduler_dep_test.go
git commit -m "feat(teamleader): add strict dependency check in scheduler"
```

---

## Task 5: Decompose + Confirm API 端点

**Files:**
- Create: `internal/web/handlers_decompose.go`
- Modify: `internal/web/handlers_v3.go` (route registration)

**Step 1: Write the handler**

```go
// internal/web/handlers_decompose.go
package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/teamleader"
)

type decomposeHandlers struct {
	planner *teamleader.DecomposePlanner
	store   core.Store
	manager IssueManager
}

type decomposeRequest struct {
	Prompt string `json:"prompt"`
}

type confirmRequest struct {
	ProposalID string              `json:"proposal_id"`
	Issues     []core.ProposalItem `json:"issues"`
}

type confirmResponse struct {
	CreatedIssues []createdIssueRef `json:"created_issues"`
}

type createdIssueRef struct {
	TempID  string `json:"temp_id"`
	IssueID string `json:"issue_id"`
}

func (h *decomposeHandlers) decompose(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	if projectID == "" {
		http.Error(w, "project id required", http.StatusBadRequest)
		return
	}

	var req decomposeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	proposal, err := h.planner.Plan(r.Context(), projectID, prompt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, proposal)
}

func (h *decomposeHandlers) confirm(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")

	var req confirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate as a proposal.
	proposal := core.DecomposeProposal{
		ID:    req.ProposalID,
		Items: req.Issues,
	}
	if err := proposal.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Map temp_id → real issue_id.
	tempToReal := make(map[string]string, len(req.Issues))
	var created []createdIssueRef

	for _, item := range req.Issues {
		issueID := core.NewIssueID()
		tempToReal[item.TempID] = issueID
	}

	// Create issues with resolved dependencies.
	for _, item := range req.Issues {
		realDeps := make([]string, 0, len(item.DependsOn))
		for _, dep := range item.DependsOn {
			if realID, ok := tempToReal[dep]; ok {
				realDeps = append(realDeps, realID)
			}
		}

		template := strings.TrimSpace(item.Template)
		if template == "" {
			template = "standard"
		}

		autoMerge := true
		if item.AutoMerge != nil {
			autoMerge = *item.AutoMerge
		}

		issue := &core.Issue{
			ID:         tempToReal[item.TempID],
			ProjectID:  projectID,
			Title:      item.Title,
			Body:       item.Body,
			Labels:     item.Labels,
			Template:   template,
			AutoMerge:  autoMerge,
			DependsOn:  realDeps,
			State:      core.IssueStateOpen,
			Status:     core.IssueStatusQueued,
			FailPolicy: core.FailBlock,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := h.store.CreateIssue(issue); err != nil {
			http.Error(w, "create issue: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Set Blocks on dependency issues (reverse edges).
		for _, depID := range realDeps {
			depIssue, err := h.store.GetIssue(depID)
			if err != nil {
				continue
			}
			depIssue.Blocks = appendUnique(depIssue.Blocks, issue.ID)
			h.store.SaveIssue(depIssue)
		}

		created = append(created, createdIssueRef{
			TempID:  item.TempID,
			IssueID: issue.ID,
		})
	}

	writeJSON(w, http.StatusCreated, confirmResponse{CreatedIssues: created})
}

func appendUnique(slice []string, val string) []string {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}
```

**Step 2: Register routes**

In `internal/web/handlers_v3.go`, add to the `registerV1Routes` function:

```go
// Decompose routes (requires DecomposePlanner to be passed in).
if decomposePlanner != nil {
	decH := &decomposeHandlers{
		planner: decomposePlanner,
		store:   store,
		manager: issueManager,
	}
	r.With(RequireScope(ScopeChatWrite)).Post("/projects/{projectId}/decompose", decH.decompose)
	r.With(RequireScope(ScopeChatWrite)).Post("/projects/{projectId}/decompose/confirm", decH.confirm)
}
```

Note: `registerV1Routes` needs a new `decomposePlanner *teamleader.DecomposePlanner` parameter. Update the function signature and callers.

**Step 3: Build check**

Run: `go build ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/web/handlers_decompose.go internal/web/handlers_v3.go
git commit -m "feat(api): add POST /decompose and /decompose/confirm endpoints"
```

---

## Task 6: 服务器启动集成 — 初始化 DecomposePlanner

**Files:**
- Modify: `cmd/ai-flow/server.go` (or equivalent server bootstrap file)

**Step 1: Create and wire DecomposePlanner**

In the server bootstrap, after the chat assistant is created, create a `DecomposePlanner` that uses the same ACP chat mechanism:

```go
// Create DecomposePlanner using the chat assistant's LLM capability.
var decomposePlanner *teamleader.DecomposePlanner
if chatAssistant != nil {
	decomposePlanner = teamleader.NewDecomposePlanner(
		func(ctx context.Context, systemPrompt, userMessage string) (string, error) {
			// Use the chat assistant to call LLM.
			// Combine system prompt and user message.
			combined := systemPrompt + "\n\n用户需求：" + userMessage
			resp, err := chatAssistant.Reply(ctx, web.ChatAssistantRequest{
				Message:   combined,
				ProjectID: "", // Will be set per-call
				WorkDir:   defaultWorkDir,
			})
			if err != nil {
				return "", err
			}
			return resp.Reply, nil
		},
	)
}
```

Pass `decomposePlanner` to `registerV1Routes()`.

**Step 2: Build and start check**

Run: `go build ./cmd/ai-flow/`
Expected: PASS

**Step 3: Commit**

```bash
git add cmd/ai-flow/
git commit -m "feat(server): wire DecomposePlanner into server bootstrap"
```

---

## Task 7: 前端类型和 API 客户端

**Files:**
- Modify: `web/src/types/api.ts`
- Modify: `web/src/lib/apiClient.ts`

**Step 1: Add types**

Add to `web/src/types/api.ts`:

```typescript
// Decompose Proposal types
export interface DecomposeRequest {
  prompt: string;
}

export interface ProposalItem {
  temp_id: string;
  title: string;
  body: string;
  labels: string[];
  depends_on: string[];
  template?: string;
  auto_merge?: boolean;
}

export interface DecomposeProposal {
  proposal_id: string;
  project_id: string;
  prompt: string;
  summary: string;
  issues: ProposalItem[];
}

export interface ConfirmDecomposeRequest {
  proposal_id: string;
  issues: ProposalItem[];
}

export interface ConfirmDecomposeResponse {
  created_issues: Array<{ temp_id: string; issue_id: string }>;
}
```

**Step 2: Add API client methods**

Add to `web/src/lib/apiClient.ts`:

```typescript
decompose: (projectId: string, body: DecomposeRequest) =>
  request<DecomposeProposal, DecomposeRequest>({
    path: `/api/v1/projects/${projectId}/decompose`,
    method: "POST",
    body,
  }),

confirmDecompose: (projectId: string, body: ConfirmDecomposeRequest) =>
  request<ConfirmDecomposeResponse, ConfirmDecomposeRequest>({
    path: `/api/v1/projects/${projectId}/decompose/confirm`,
    method: "POST",
    body,
  }),
```

**Step 3: Type check**

Run: `npm --prefix web run typecheck`
Expected: PASS

**Step 4: Commit**

```bash
git add web/src/types/api.ts web/src/lib/apiClient.ts
git commit -m "feat(web): add decompose/confirm types and API client methods"
```

---

## Task 8: QuickInput 组件

**Files:**
- Create: `web/src/components/QuickInput.tsx`

**Step 1: Build the component**

```typescript
// web/src/components/QuickInput.tsx
import { useState, useCallback } from "react";

interface QuickInputProps {
  placeholder?: string;
  loading?: boolean;
  onSubmit: (prompt: string) => void;
}

export function QuickInput({ placeholder, loading, onSubmit }: QuickInputProps) {
  const [value, setValue] = useState("");

  const handleSubmit = useCallback(() => {
    const trimmed = value.trim();
    if (!trimmed || loading) return;
    onSubmit(trimmed);
    setValue("");
  }, [value, loading, onSubmit]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        handleSubmit();
      }
    },
    [handleSubmit]
  );

  return (
    <div className="flex items-center gap-2">
      <input
        type="text"
        className="flex-1 rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm text-zinc-900 placeholder:text-zinc-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        placeholder={placeholder ?? "描述你的需求..."}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        disabled={loading}
      />
      <button
        className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
        onClick={handleSubmit}
        disabled={!value.trim() || loading}
      >
        {loading ? "分析中..." : "拆解"}
      </button>
    </div>
  );
}
```

**Step 2: Type check**

Run: `npm --prefix web run typecheck`
Expected: PASS

**Step 3: Commit**

```bash
git add web/src/components/QuickInput.tsx
git commit -m "feat(web): add QuickInput component for BoardView"
```

---

## Task 9: DagPreview 组件（@xyflow/react）

**Files:**
- Create: `web/src/components/DagPreview.tsx`

**Step 1: Build the component**

```typescript
// web/src/components/DagPreview.tsx
import { useCallback, useMemo, useState } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  type Node,
  type Edge,
  Position,
  MarkerType,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { ProposalItem } from "../types/api";

interface DagPreviewProps {
  items: ProposalItem[];
  summary: string;
  loading?: boolean;
  onConfirm: (items: ProposalItem[]) => void;
  onCancel: () => void;
  onUpdateItem?: (index: number, item: ProposalItem) => void;
  onRemoveItem?: (index: number) => void;
}

function layoutNodes(items: ProposalItem[]): Node[] {
  // Simple layered layout based on dependency depth.
  const depth: Record<string, number> = {};
  const getDepth = (id: string): number => {
    if (depth[id] !== undefined) return depth[id];
    const item = items.find((i) => i.temp_id === id);
    if (!item || item.depends_on.length === 0) {
      depth[id] = 0;
      return 0;
    }
    depth[id] = Math.max(...item.depends_on.map((d) => getDepth(d))) + 1;
    return depth[id];
  };
  items.forEach((i) => getDepth(i.temp_id));

  // Group by depth for x positioning.
  const byDepth: Record<number, string[]> = {};
  for (const [id, d] of Object.entries(depth)) {
    byDepth[d] = byDepth[d] || [];
    byDepth[d].push(id);
  }

  return items.map((item) => {
    const d = depth[item.temp_id];
    const siblings = byDepth[d];
    const idx = siblings.indexOf(item.temp_id);
    return {
      id: item.temp_id,
      data: { label: `${item.temp_id}. ${item.title}` },
      position: { x: idx * 220 - ((siblings.length - 1) * 220) / 2 + 400, y: d * 120 + 40 },
      sourcePosition: Position.Bottom,
      targetPosition: Position.Top,
      style: {
        background: "#fff",
        border: "1px solid #d0d7de",
        borderRadius: "6px",
        padding: "8px 12px",
        fontSize: "13px",
        minWidth: "160px",
        textAlign: "center" as const,
      },
    };
  });
}

function layoutEdges(items: ProposalItem[]): Edge[] {
  const edges: Edge[] = [];
  for (const item of items) {
    for (const dep of item.depends_on) {
      edges.push({
        id: `${dep}-${item.temp_id}`,
        source: dep,
        target: item.temp_id,
        markerEnd: { type: MarkerType.ArrowClosed },
        style: { stroke: "#6b7280" },
      });
    }
  }
  return edges;
}

export function DagPreview({
  items,
  summary,
  loading,
  onConfirm,
  onCancel,
}: DagPreviewProps) {
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const nodes = useMemo(() => layoutNodes(items), [items]);
  const edges = useMemo(() => layoutEdges(items), [items]);

  return (
    <div className="flex flex-col rounded-lg border border-zinc-200 bg-white shadow-lg">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-zinc-200 px-4 py-3">
        <div>
          <h3 className="text-sm font-semibold text-zinc-900">确认需求拆解方案</h3>
          {summary && <p className="mt-0.5 text-xs text-zinc-500">{summary}</p>}
        </div>
        <div className="flex gap-2">
          <button
            className="rounded-md border border-zinc-300 px-3 py-1.5 text-xs text-zinc-700 hover:bg-zinc-50"
            onClick={onCancel}
            disabled={loading}
          >
            取消
          </button>
          <button
            className="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            onClick={() => onConfirm(items)}
            disabled={loading}
          >
            {loading ? "创建中..." : `创建 ${items.length} 个 Issue`}
          </button>
        </div>
      </div>

      {/* DAG Graph */}
      <div className="h-[300px] border-b border-zinc-200">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          fitView
          nodesDraggable={false}
          nodesConnectable={false}
          elementsSelectable={false}
          proOptions={{ hideAttribution: true }}
        >
          <Background />
          <Controls showInteractive={false} />
        </ReactFlow>
      </div>

      {/* Issue List */}
      <div className="max-h-[240px] overflow-y-auto p-3">
        <div className="space-y-2">
          {items.map((item, idx) => (
            <div
              key={item.temp_id}
              className="flex items-start gap-2 rounded border border-zinc-100 p-2 text-sm hover:bg-zinc-50"
              onClick={() => setEditingIndex(editingIndex === idx ? null : idx)}
            >
              <span className="mt-0.5 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded bg-zinc-100 text-xs font-medium text-zinc-600">
                {item.temp_id}
              </span>
              <div className="min-w-0 flex-1">
                <div className="font-medium text-zinc-900">{item.title}</div>
                {item.depends_on.length > 0 && (
                  <div className="mt-0.5 text-xs text-zinc-500">
                    依赖: {item.depends_on.join(", ")}
                  </div>
                )}
                {editingIndex === idx && (
                  <div className="mt-1 text-xs text-zinc-600 whitespace-pre-wrap">
                    {item.body}
                  </div>
                )}
              </div>
              {item.labels?.length > 0 && (
                <div className="flex gap-1">
                  {item.labels.map((l) => (
                    <span key={l} className="rounded-full bg-blue-50 px-2 py-0.5 text-xs text-blue-700">
                      {l}
                    </span>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
```

**Step 2: Type check**

Run: `npm --prefix web run typecheck`
Expected: PASS

**Step 3: Commit**

```bash
git add web/src/components/DagPreview.tsx
git commit -m "feat(web): add DagPreview component with @xyflow/react visualization"
```

---

## Task 10: BoardView 集成 — QuickInput + DagPreview

**Files:**
- Modify: `web/src/views/BoardView.tsx`

**Step 1: Add imports and state**

Add imports at the top:

```typescript
import { QuickInput } from "../components/QuickInput";
import { DagPreview } from "../components/DagPreview";
import type { DecomposeProposal, ProposalItem } from "../types/api";
```

Add state variables alongside existing state:

```typescript
const [decomposeLoading, setDecomposeLoading] = useState(false);
const [proposal, setProposal] = useState<DecomposeProposal | null>(null);
const [confirmLoading, setConfirmLoading] = useState(false);
```

**Step 2: Add handler functions**

```typescript
const handleDecompose = useCallback(async (prompt: string) => {
  if (!projectId) return;
  setDecomposeLoading(true);
  try {
    const result = await apiClient.decompose(projectId, { prompt });
    setProposal(result);
  } catch (err) {
    // Show error notification
    console.error("decompose failed:", err);
  } finally {
    setDecomposeLoading(false);
  }
}, [projectId]);

const handleConfirmDecompose = useCallback(async (items: ProposalItem[]) => {
  if (!projectId || !proposal) return;
  setConfirmLoading(true);
  try {
    await apiClient.confirmDecompose(projectId, {
      proposal_id: proposal.proposal_id,
      issues: items,
    });
    setProposal(null);
    // Refresh issue list
    refreshTasks();
  } catch (err) {
    console.error("confirm failed:", err);
  } finally {
    setConfirmLoading(false);
  }
}, [projectId, proposal]);
```

**Step 3: Add QuickInput to header**

In the BoardView header section, add QuickInput:

```tsx
<header className="rounded-md border border-[#d0d7de] bg-white p-4">
  <div className="flex items-center justify-between">
    <h1 className="text-xl font-semibold text-[#24292f]">Issues</h1>
  </div>
  <div className="mt-3">
    <QuickInput
      onSubmit={handleDecompose}
      loading={decomposeLoading}
      placeholder="描述你的需求，AI 将自动拆解为任务..."
    />
  </div>
  {/* existing refresh controls */}
</header>
```

**Step 4: Add DagPreview modal**

After the header, add the DagPreview overlay:

```tsx
{proposal && (
  <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
    <div className="w-full max-w-3xl">
      <DagPreview
        items={proposal.issues}
        summary={proposal.summary}
        loading={confirmLoading}
        onConfirm={handleConfirmDecompose}
        onCancel={() => setProposal(null)}
      />
    </div>
  </div>
)}
```

**Step 5: Type check and build**

Run: `npm --prefix web run typecheck && npm --prefix web run build`
Expected: PASS

**Step 6: Commit**

```bash
git add web/src/views/BoardView.tsx
git commit -m "feat(web): integrate QuickInput and DagPreview into BoardView"
```

---

## Task 11: 全量构建和测试

**Step 1: Run all backend tests**

Run: `pwsh -NoProfile -File ./scripts/test/backend-all.ps1`
Expected: PASS

**Step 2: Run frontend build**

Run: `npm --prefix web run build`
Expected: PASS

**Step 3: Commit any fixes**

```bash
git add -A
git commit -m "fix: address build/test issues from DAG decompose integration"
```

---

## Task 12: （可选，v3 合并后）TaskStep 集成

**前置条件:** TaskStep 事件溯源（v3）已合并。

**Files:**
- Modify: `internal/web/handlers_decompose.go`

**Step 1: Add TaskStep writes in confirm handler**

After each `CreateIssue()` call in the confirm handler, add:

```go
// Write TaskStep for creation + queuing (requires v3 TaskStep).
if _, err := h.store.SaveTaskStep(&core.TaskStep{
    ID:        core.NewTaskStepID(),
    IssueID:   issue.ID,
    Action:    core.StepCreated,
    AgentID:   "system",
    Note:      "created from DAG decompose",
    CreatedAt: time.Now(),
}); err != nil {
    slog.Warn("save task step", "error", err)
}
if _, err := h.store.SaveTaskStep(&core.TaskStep{
    ID:        core.NewTaskStepID(),
    IssueID:   issue.ID,
    Action:    core.StepQueued,
    AgentID:   "system",
    Note:      "auto-queued from DAG confirm",
    CreatedAt: time.Now(),
}); err != nil {
    slog.Warn("save task step", "error", err)
}
```

**Step 2: Build and test**

Run: `go build ./... && go test ./internal/web/ -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/web/handlers_decompose.go
git commit -m "feat(api): write TaskStep on DAG decompose confirm"
```

---

## Summary

| Task | Description | Key Files | Depends On |
|------|-------------|-----------|------------|
| 1 | Proposal 数据模型 + DAG 校验 | core/proposal.go | — |
| 2 | DecomposePlanner (LLM 调用) | teamleader/decompose_planner.go | Task 1 |
| 3 | 启用 DependsOn/Blocks 字段 | teamleader/manager.go | — |
| 4 | Scheduler 严格依赖检查 | teamleader/scheduler_dispatch.go | Task 3 |
| 5 | Decompose/Confirm API 端点 | web/handlers_decompose.go | Task 1, 2 |
| 6 | 服务器启动集成 | cmd/ai-flow/server.go | Task 2, 5 |
| 7 | 前端类型 + API 客户端 | web/src/types, apiClient | — |
| 8 | QuickInput 组件 | web/src/components/QuickInput.tsx | — |
| 9 | DagPreview 组件 | web/src/components/DagPreview.tsx | Task 7 |
| 10 | BoardView 集成 | web/src/views/BoardView.tsx | Task 8, 9 |
| 11 | 全量构建测试 | all | Task 1-10 |
| 12 | TaskStep 集成（可选） | web/handlers_decompose.go | v3 合并 |

可并行的 Task 组:
- **后端组**: Task 1→2→5→6 + Task 3→4 （两条线独立）
- **前端组**: Task 7→8→9→10 （独立于后端）
- **Task 12**: 等 v3 合并后单独做
