# Execution Engine v1 (Demo04: Report + PPT) Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 让 `taskctl run` 在不依赖外部 LLM 的情况下，按 `docs/04_walkthrough_report_ppt.md` 跑通一条“Planner → MPU 并行 → Assemble → PPT Adapter/Renderer → Verify → Pack(Bundle+ledger+evidence+approval)”的真实执行流程；并在代码结构上为未来替换为真实 Agent/LLM 留好扩展点。

**Architecture:** 在 `SANDBOX_RUN` 阶段新增一个“内置工作流引擎（in-process）”。工作流会把执行过程拆成 MPU（最小并行单元），用 `internal/scheduler.Limiter` 施加 global/team/role 配额；MPU 产物落到 `revs/{rev}/mpus/**`，再由 Assembler 汇总生成 `revs/{rev}/deliver/**`。任何“可预期的流程失败”（如 adapter 不可生成）通过写 `revs/{rev}/issues.json` 的 `blocker` 表达，让后续 `VERIFY` 阶段统一阻断；只把“无法继续/无法落盘”的错误作为真正的 `error` 抛出。

**Tech Stack:** Go（`cmd/taskctl` / `internal/**`）、Cobra/Viper、`internal/gateway`（审计写入）、`internal/planning`（DeliveryPlan YAML）、`internal/scheduler`（并行配额）、JSON/YAML。

---

## 先读这些（避免迷路）
- 目标演练：`docs/04_walkthrough_report_ppt.md`
- 并行与计划：`docs/02_planning_and_parallelism.md`
- 产物与强协议链：`docs/03_artifact_pipeline.md`
- 现有执行入口：`internal/cli/run.go:37`（`SANDBOX_RUN → VERIFY → PACK`）
- 现有 Bundle/ledger/evidence：`internal/taskbundle/bundle.go`
- 验收报告生成（扩展点）：`internal/taskbundle/report.go`

---

### Task 1: 定义工作流引擎最小接口 + 注册表

**Files:**
- Create: `internal/execution/workflow.go`
- Create: `internal/execution/registry.go`
- Test: `internal/execution/registry_test.go`

**Step 1: 写一个会失败的测试（未知 workflow 应报错）**

```go
package execution

import "testing"

func TestRegistry_Get_Unknown(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("nope"); err == nil {
		t.Fatalf("expected error for unknown workflow")
	}
}
```

**Step 2: 运行测试确认失败**

Run: `go test ./internal/execution -run TestRegistry_Get_Unknown -count=1`  
Expected: FAIL（`NewRegistry`/`Get` 未定义）

**Step 3: 最小实现让测试通过**

```go
package execution

import "fmt"

type Workflow interface {
	Name() string
	Run(ctx Context) (Result, error)
}

type Registry struct{ m map[string]Workflow }

func NewRegistry() *Registry { return &Registry{m: map[string]Workflow{}} }

func (r *Registry) Register(w Workflow) error {
	if w == nil || w.Name() == "" {
		return fmt.Errorf("invalid workflow")
	}
	if _, ok := r.m[w.Name()]; ok {
		return fmt.Errorf("workflow exists: %s", w.Name())
	}
	r.m[w.Name()] = w
	return nil
}

func (r *Registry) Get(name string) (Workflow, error) {
	w, ok := r.m[name]
	if !ok {
		return nil, fmt.Errorf("unknown workflow: %s", name)
	}
	return w, nil
}
```

并在 `workflow.go` 补上 `Context/Result` 的最小定义（先占位，后面会扩展）：

```go
package execution

import (
	"context"
	"strings"

	"github.com/yoke233/zhanggui/internal/gateway"
	"github.com/yoke233/zhanggui/internal/verify"
)

type Context struct {
	Ctx context.Context
	GW  *gateway.Gateway

	TaskID string
	RunID  string
	Rev    string
}

type Result struct {
	Issues []verify.Issue
}

func (r Result) HasBlocker() bool {
	for _, it := range r.Issues {
		if strings.EqualFold(strings.TrimSpace(it.Severity), "blocker") {
			return true
		}
	}
	return false
}
```

**Step 4: 运行测试确认通过**

Run: `go test ./internal/execution -run TestRegistry_Get_Unknown -count=1`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/execution/registry.go internal/execution/workflow.go internal/execution/registry_test.go
git commit -m "feat(execution): scaffold workflow registry"
```

---

### Task 2: Demo04 工作流骨架（先产出 deliver/report.md）

**Files:**
- Create: `internal/execution/demo04.go`
- Test: `internal/execution/demo04_test.go`

**Step 1: 写失败测试（跑完后应有 deliver/report.md）**

```go
package execution

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoke233/zhanggui/internal/gateway"
)

func TestDemo04_Run_GeneratesReport(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "logs"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "revs", "r1"), 0o755)

	aud, err := gateway.NewAuditor(filepath.Join(root, "logs", "tool_audit.jsonl"))
	if err != nil {
		t.Fatalf("NewAuditor: %v", err)
	}
	t.Cleanup(func() { _ = aud.Close() })

	gw, err := gateway.New(root, gateway.Actor{AgentID: "taskctl", Role: "system"}, gateway.Linkage{TaskID: "t1", RunID: "r1", Rev: "r1"}, gateway.Policy{
		AllowedWritePrefixes: []string{"revs/", "logs/"},
	}, aud)
	if err != nil {
		t.Fatalf("gateway.New: %v", err)
	}

	w := NewDemo04Workflow()
	res, err := w.Run(Context{Ctx: context.Background(), GW: gw, TaskID: "t1", RunID: "r1", Rev: "r1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.HasBlocker() {
		t.Fatalf("unexpected blockers: %+v", res.Issues)
	}

	if _, err := os.Stat(filepath.Join(root, "revs", "r1", "deliver", "report.md")); err != nil {
		t.Fatalf("missing deliver/report.md: %v", err)
	}
}
```

**Step 2: 运行测试确认失败**

Run: `go test ./internal/execution -run TestDemo04_Run_GeneratesReport -count=1`  
Expected: FAIL（`NewDemo04Workflow` 不存在）

**Step 3: 最小实现 Demo04（只写 report.md）**

```go
package execution

import (
	"fmt"
	"path/filepath"
	"time"
)

type demo04Workflow struct{}

func NewDemo04Workflow() Workflow { return &demo04Workflow{} }

func (w *demo04Workflow) Name() string { return "demo04" }

func (w *demo04Workflow) Run(ctx Context) (Result, error) {
	if ctx.GW == nil {
		return Result{}, fmt.Errorf("gw missing")
	}
	rev := ctx.Rev
	if rev == "" {
		rev = "r1"
	}
	dst := filepath.ToSlash(filepath.Join("revs", rev, "deliver", "report.md"))
	body := []byte(fmt.Sprintf("# demo04 report\n\ngenerated_at: %s\n", time.Now().Format(time.RFC3339)))
	if err := ctx.GW.ReplaceFile(dst, body, 0o644, "demo04: write deliver/report.md"); err != nil {
		return Result{}, err
	}
	return Result{}, nil
}
```

> 这里先用 `ReplaceFile`（single-writer：system），后续再按“不可变产物”要求收紧到 create-only（需要配套目录策略与迁移；先跑通）。

**Step 4: 运行测试确认通过**

Run: `go test ./internal/execution -run TestDemo04_Run_GeneratesReport -count=1`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/execution/demo04.go internal/execution/demo04_test.go internal/execution/workflow.go
git commit -m "feat(execution): add demo04 workflow skeleton"
```

---

### Task 3: 引入 MPU 模型（落盘 mpus/**/summary.md）并写入 issues.json（空）

**Files:**
- Create: `internal/execution/mpu.go`
- Modify: `internal/execution/demo04.go`
- Test: `internal/execution/demo04_test.go`

**Step 1: 扩展测试：应生成 N 个 MPU summary + rev/issues.json**

```go
// 断言 rev/issues.json 存在且为合法 JSON；mpus/ 下至少 1 个 summary.md。
```

**Step 2: 运行测试确认失败**

Run: `go test ./internal/execution -run TestDemo04_ -count=1`  
Expected: FAIL（文件不存在）

**Step 3: 最小实现**

在 `mpu.go` 定义：

```go
package execution

type MPU struct {
	MPUID  string
	TeamID string
	Role   string
	Kind   string // report_section|ppt_slide|quality
	Title  string
}
```

在 `demo04.go`：
- 生成至少 2 个 MPU（report 章节 + ppt 页面），MPU ID 用 `internal/uuidv7.New()`。
- 为每个 MPU 写 `revs/{rev}/mpus/{mpu_id}/summary.md`
- 写入 `revs/{rev}/issues.json`（空数组，schema_version=1，task_id/rev）

**Step 4: 运行测试确认通过**

Run: `go test ./internal/execution -count=1`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/execution/mpu.go internal/execution/demo04.go internal/execution/demo04_test.go
git commit -m "feat(execution): write demo04 MPU summaries and issues.json"
```

---

### Task 4: 并行执行 MPU（接入 scheduler.Limiter，覆盖 global/team/role cap）

**Files:**
- Create: `internal/execution/runner.go`
- Test: `internal/execution/runner_test.go`
- Modify: `internal/execution/demo04.go`

**Step 1: 写失败测试：cap=1 时最大并发不超过 1**

```go
package execution

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/zhanggui/internal/scheduler"
)

func TestRunMPUs_RespectsGlobalCap(t *testing.T) {
	lim, err := scheduler.NewLimiter(scheduler.Caps{GlobalMax: 1})
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}

	var inFlight int32
	var maxSeen int32

	mpus := []MPU{{TeamID: "team_a", Role: "writer"}, {TeamID: "team_a", Role: "writer"}, {TeamID: "team_a", Role: "writer"}}
	err = RunMPUs(context.Background(), lim, mpus, func(ctx context.Context, m MPU) error {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxSeen)
			if n <= old || atomic.CompareAndSwapInt32(&maxSeen, old, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil
	})
	if err != nil {
		t.Fatalf("RunMPUs: %v", err)
	}
	if maxSeen != 1 {
		t.Fatalf("expected maxSeen=1, got %d", maxSeen)
	}
}
```

**Step 2: 运行测试确认失败**

Run: `go test ./internal/execution -run TestRunMPUs_RespectsGlobalCap -count=1`  
Expected: FAIL（`RunMPUs` 不存在）

**Step 3: 最小实现 RunMPUs**

`runner.go`：
- 对每个 MPU：`limiter.Acquire(ctx, scheduler.Key{TeamID: m.TeamID, Role: m.Role})`
- goroutine 执行 `fn(ctx, m)`；defer `lease.Release()`
- 收集第一个 error 并 cancel context

**Step 4: 运行测试确认通过**

Run: `go test ./internal/execution -run TestRunMPUs_RespectsGlobalCap -count=1`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/execution/runner.go internal/execution/runner_test.go
git commit -m "feat(execution): run MPUs in parallel with scheduler limiter"
```

---

### Task 5: 从 DeliveryPlan YAML 生成 scheduler.Caps（数据驱动并行配额）

**Files:**
- Create: `internal/execution/caps.go`
- Test: `internal/execution/caps_test.go`

**Step 1: 写失败测试：Budgets→Caps 映射**

```go
package execution

import (
	"testing"

	"github.com/yoke233/zhanggui/internal/planning"
)

func TestCapsFromPlan_Defaults(t *testing.T) {
	p := planning.DeliveryPlan{Budgets: planning.Budgets{MaxParallel: 0}}
	c, err := CapsFromPlan(p)
	if err != nil {
		t.Fatalf("CapsFromPlan: %v", err)
	}
	if c.GlobalMax <= 0 {
		t.Fatalf("expected GlobalMax > 0")
	}
}
```

**Step 2: 运行测试确认失败**

Run: `go test ./internal/execution -run TestCapsFromPlan_Defaults -count=1`  
Expected: FAIL（`CapsFromPlan` 不存在）

**Step 3: 最小实现 CapsFromPlan**

规则建议：
- `GlobalMax = plan.Budgets.MaxParallel`，若为 0 则默认 4（或 2；但必须 >0）
- `PerTeam = plan.Budgets.PerTeamParallelCap`
- `PerRole = plan.Budgets.PerRoleParallelCap`

**Step 4: 运行测试确认通过**

Run: `go test ./internal/execution -run TestCapsFromPlan_Defaults -count=1`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/execution/caps.go internal/execution/caps_test.go
git commit -m "feat(execution): derive scheduler caps from delivery plan"
```

---

### Task 6: Demo04 组装器（Assembler）：从 mpus/** 合并为 deliver/report.md + deliver/ppt_ir.json

**Files:**
- Create: `internal/execution/assemble.go`
- Test: `internal/execution/assemble_test.go`
- Modify: `internal/execution/demo04.go`

**Step 1: 写失败测试：report 含固定锚点 + ppt_ir.json 合法 JSON**

断言：
- `deliver/report.md` 包含 `<a id="block-deliver-report-2"></a>`
- `deliver/ppt_ir.json` 能 `json.Unmarshal` 成对象，并含 `schema_version`

**Step 2: 运行测试确认失败**

Run: `go test ./internal/execution -run TestAssemble_ -count=1`  
Expected: FAIL

**Step 3: 最小实现 Assemble**

`assemble.go`：
- 读 `revs/{rev}/mpus/**/summary.md`（允许用 `filepath.WalkDir`）
- 把部分内容拼成 report（先写 1~2 个章节；保持可扩展：按 Kind 分组）
- 生成 `ppt_ir.json`（结构：`schema_version,title,slides[]`）

**Step 4: 运行测试确认通过**

Run: `go test ./internal/execution -count=1`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/execution/assemble.go internal/execution/assemble_test.go internal/execution/demo04.go
git commit -m "feat(execution): assemble demo04 deliverables from mpus"
```

---

### Task 7: PPT Adapter：ppt_ir.json → ppt_renderer_input.json（强协议入口）

**Files:**
- Create: `internal/execution/ppt_adapter.go`
- Test: `internal/execution/ppt_adapter_test.go`
- Modify: `internal/execution/demo04.go`

**Step 1: 写失败测试：输入缺字段时应写 blocker issue（where=adapter）**

断言：
- 传入坏 JSON 时：`issues.json` 至少 1 条 `severity=blocker` 且 `where=adapter`

**Step 2: 运行测试确认失败**

Run: `go test ./internal/execution -run TestPPTAdapter_ -count=1`  
Expected: FAIL

**Step 3: 最小实现 Adapter**

约束：
- Adapter 只做 schema 校验/默认值/长度限制提示；不得引入新事实
- 失败不要 `panic`；返回 `Issue{Severity:"blocker", Where:"adapter", What:"..."}` 供上层写入 issues.json

**Step 4: 运行测试确认通过**

Run: `go test ./internal/execution -count=1`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/execution/ppt_adapter.go internal/execution/ppt_adapter_test.go
git commit -m "feat(execution): add ppt adapter with issue reporting"
```

---

### Task 8: PPT Renderer：ppt_renderer_input.json → slides.html（表现层）

**Files:**
- Create: `internal/execution/ppt_renderer.go`
- Test: `internal/execution/ppt_renderer_test.go`
- Modify: `internal/execution/demo04.go`

**Step 1: 写失败测试：生成 slides.html**

断言：
- `deliver/slides.html` 存在
- HTML 内包含标题 `<h1>` 与至少 1 个 slide 标题

**Step 2: 运行测试确认失败**

Run: `go test ./internal/execution -run TestPPTRenderer_ -count=1`  
Expected: FAIL

**Step 3: 最小实现 Renderer**

实现建议：
- 读取 renderer_input JSON
- 输出一个极简 HTML（后续可替换真正渲染器）

**Step 4: 运行测试确认通过**

Run: `go test ./internal/execution -count=1`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/execution/ppt_renderer.go internal/execution/ppt_renderer_test.go internal/execution/demo04.go
git commit -m "feat(execution): render slides.html from ppt renderer input"
```

---

### Task 9: 把工作流接入 taskctl run（新增 --workflow）

**Files:**
- Modify: `internal/cli/run.go:151`（SANDBOX_RUN 分支）
- Modify: `internal/cli/run.go:329`（flags 区域）
- Test: `internal/execution/demo04_test.go`（或新增 `internal/cli/run_test.go` 做 smoke）
- Docs: `docs/04_walkthrough_report_ppt.md`

**Step 1: 写失败测试（建议做 CLI smoke）**

新增 `internal/cli/run_test.go`（最小 smoke）：
- `base-dir` 指向 `t.TempDir()`
- `--sandbox-mode local`
- `--workflow demo04`
- 运行命令后：检查输出的 task dir 下存在 `revs/r1/deliver/report.md`

**Step 2: 运行测试确认失败**

Run: `go test ./internal/cli -run TestRunCmd_WorkflowDemo04 -count=1`  
Expected: FAIL（flag 不存在 / 未生成文件）

**Step 3: 最小实现接入**

在 `internal/cli/run.go`：
- 新增 flag：`--workflow`（string；默认空）
- 约束：`--workflow` 与 `--entrypoint` 互斥（同时提供则报错）
- 当 `workflow != ""`：跳过 `sandbox.NewRunner`，改为：
  - `reg := execution.NewRegistry(); reg.Register(execution.NewDemo04Workflow())`
  - `wf, _ := reg.Get(workflow)`
  - `res, err := wf.Run(execution.Context{Ctx: ctx, GW: gw, TaskID: taskID, RunID: runID, Rev: rev})`
  - 若 `err != nil`：按“SANDBOX_RUN 失败”处理（返回 error）
  - 若 `res.HasBlocker()`：不要直接 return error，继续走 VERIFY（让 VERIFY 统一阻断并打印原因）

**Step 4: 运行测试确认通过**

Run: `go test ./...`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cli/run.go internal/cli/run_test.go docs/04_walkthrough_report_ppt.md
git commit -m "feat(taskctl): add --workflow and wire demo04 execution"
```

---

### Task 10: 让验收报告真正覆盖 demo04 交付物（可选但推荐）

**Files:**
- Modify: `docs/proposals/acceptance_criteria_v1.yaml`
- Modify: `internal/taskbundle/report.go`
- Test: `internal/taskbundle/bundle_test.go`（或新增针对新 criteria 的测试）

**Step 1: 写失败测试：verify/report.json 里新增 criteria PASS**

新增 criteria（示例）：
- `AC-006`: `revs/{rev}/deliver/report.md` 必须存在
- `AC-007`: `revs/{rev}/deliver/slides.html` 必须存在

先写测试断言 `report.Results` 中存在对应 criteria_id 且 `Status=PASS`。

**Step 2: 运行测试确认失败**

Run: `go test ./internal/taskbundle -run TestCreatePackBundle_ -count=1`  
Expected: FAIL（当前 `report.go` 未实现 AC-006/007）

**Step 3: 最小实现**

在 `internal/taskbundle/report.go` 的 switch 增加 AC-006/AC-007：
- 用 `memberRef("revs/{rev}/deliver/report.md")` / `memberRef("revs/{rev}/deliver/slides.html")`

**Step 4: 运行测试确认通过**

Run: `go test ./internal/taskbundle -count=1`  
Expected: PASS

**Step 5: Commit**

```bash
git add docs/proposals/acceptance_criteria_v1.yaml internal/taskbundle/report.go internal/taskbundle/bundle_test.go
git commit -m "feat(verify): extend acceptance criteria for demo04 deliverables"
```

---

### Task 11: 文档对齐（把 demo04 的“纸面演练”变成“可运行步骤”）

**Files:**
- Modify: `docs/04_walkthrough_report_ppt.md`
- Modify: `README.md`（如需补充命令入口）

**Step 1: 写文档变更（无代码）**

在 `docs/04_walkthrough_report_ppt.md` 追加 “如何运行”：
- `go run ./cmd/taskctl run --sandbox-mode local --workflow demo04 --approval-policy always`
- 展示期望路径：`revs/r1/deliver/report.md`、`revs/r1/deliver/ppt_ir.json`、`revs/r1/deliver/ppt_renderer_input.json`、`revs/r1/deliver/slides.html`
- 展示 pack 产物：`packs/{pack_id}/ledger/events.jsonl`、`packs/{pack_id}/pack/evidence.zip`
- 追加审批示例：`go run ./cmd/taskctl approve grant <task_dir>`

**Step 2: 运行格式检查/测试**

Run: `go test ./...`  
Expected: PASS

**Step 3: Commit**

```bash
git add docs/04_walkthrough_report_ppt.md README.md
git commit -m "docs: make demo04 walkthrough executable"
```

---

## 执行后验收清单（你应该能看到什么）
- `taskctl run --workflow demo04` 输出一个 task 目录（例如 `fs/taskctl/<task_id>`）
- `revs/r1/issues.json` 为合法 JSON，且默认无 blocker
- `revs/r1/deliver/report.md` 与 `revs/r1/deliver/slides.html` 存在
- `packs/{pack_id}/ledger/events.jsonl` 出现 `VERIFY_REPORT_WRITTEN`、`ARTIFACTS_PACK_CREATED`、`EVIDENCE_PACK_CREATED`、（可选）`APPROVAL_REQUESTED`
