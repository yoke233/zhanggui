package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/yoke233/zhanggui/internal/execution"
	"github.com/yoke233/zhanggui/internal/gateway"
	"github.com/yoke233/zhanggui/internal/logging"
	"github.com/yoke233/zhanggui/internal/sandbox"
	"github.com/yoke233/zhanggui/internal/state"
	"github.com/yoke233/zhanggui/internal/taskbundle"
	"github.com/yoke233/zhanggui/internal/taskdir"
	"github.com/yoke233/zhanggui/internal/verify"
)

const (
	flagBaseDir        = "base-dir"
	flagTaskID         = "task-id"
	flagSandboxMode    = "sandbox-mode"
	flagSandboxImage   = "sandbox-image"
	flagSandboxNetwork = "sandbox-network"
	flagTimeoutSeconds = "timeout-seconds"
	flagEntrypoint     = "entrypoint"
	flagWorkflow       = "workflow"
	flagLogLevel       = "log-level"
	flagApprovalPolicy = "approval-policy"
	flagApprovalGate   = "approval-gate"
)

func NewRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "创建任务目录并执行（SANDBOX_RUN → VERIFY → PACK）",
		RunE: func(cmd *cobra.Command, args []string) error {
			baseDir := viper.GetString(flagBaseDir)
			taskID := viper.GetString(flagTaskID)
			if taskID == "" {
				var err error
				taskID, err = taskdir.NewTaskID(time.Now())
				if err != nil {
					return err
				}
			}

			runID, err := taskdir.NewRunID(time.Now())
			if err != nil {
				return err
			}

			td, err := taskdir.CreateNew(baseDir, taskID)
			if err != nil {
				return err
			}

			logPath := filepath.Join(td.LogsDir(), "run.log")
			logger, closeLogger, err := logging.NewLogger(logging.Options{
				Stdout:  os.Stderr,
				LogPath: logPath,
				Level:   logging.ParseLevel(viper.GetString(flagLogLevel)),
			})
			if err != nil {
				return err
			}
			defer func() { _ = closeLogger() }()
			logger = logger.With("task_id", taskID, "run_id", runID)
			logger.Info("task created", "task_dir", td.Root())

			auditor, err := gateway.NewAuditor(filepath.Join(td.LogsDir(), "tool_audit.jsonl"))
			if err != nil {
				return err
			}
			defer func() { _ = auditor.Close() }()

			rev := "r1"
			gw, err := gateway.New(td.Root(), gateway.Actor{AgentID: "taskctl", Role: "system"}, gateway.Linkage{TaskID: taskID, RunID: runID, Rev: rev}, gateway.Policy{
				AllowedWritePrefixes: []string{"task.json", "state.json", "logs/", "revs/", "packs/", "pack/", "verify/"},
				SingleWriterPrefixes: []string{"state.json", "pack/", "verify/"},
				SingleWriterRoles:    []string{"system"},
				LockFile:             "logs/locks/taskctl.lock",
			}, auditor)
			if err != nil {
				return err
			}
			if err := gw.AcquireLock(); err != nil {
				return err
			}
			defer func() { _ = gw.ReleaseLock() }()

			task := taskdir.Task{
				SchemaVersion: 1,
				TaskID:        taskID,
				RunID:         runID,
				CreatedAt:     time.Now().Format(time.RFC3339),
				ToolVersion:   configToolVersion(),
				Sandbox: taskdir.SandboxSpec{
					Mode:           viper.GetString(flagSandboxMode),
					Image:          viper.GetString(flagSandboxImage),
					Network:        viper.GetString(flagSandboxNetwork),
					TimeoutSeconds: viper.GetInt(flagTimeoutSeconds),
				},
				Workspace: taskdir.WorkspaceSpec{
					OutputRWPath: filepath.ToSlash(filepath.Join(td.Root(), "revs", "r1")),
				},
				Params: taskdir.ParamsSpec{
					Entrypoint: viper.GetStringSlice(flagEntrypoint),
				},
			}
			workflow := strings.TrimSpace(viper.GetString(flagWorkflow))
			if workflow != "" && len(task.Params.Entrypoint) > 0 {
				return fmt.Errorf("--%s 与 --%s 互斥", flagWorkflow, flagEntrypoint)
			}

			{
				b, err := json.MarshalIndent(task, "", "  ")
				if err != nil {
					return err
				}
				b = append(b, '\n')
				if err := gw.CreateFile("task.json", b, 0o644, "write task.json"); err != nil {
					return err
				}
			}

			st := state.New(taskID, runID)
			st.StartStep(state.StepInit)
			writeState := func(detail string) error {
				b, err := json.MarshalIndent(st, "", "  ")
				if err != nil {
					return err
				}
				b = append(b, '\n')
				return gw.ReplaceFile("state.json", b, 0o644, detail)
			}
			if err := writeState("state: INIT start"); err != nil {
				return err
			}
			st.EndStepSuccess()
			if err := writeState("state: INIT done"); err != nil {
				return err
			}

			revDir := td.RevDir(rev)
			if err := gw.MkdirAll(filepath.ToSlash(filepath.Join("revs", rev)), 0o755, "create rev dir"); err != nil {
				return err
			}
			logger.Info("rev created", "rev", rev, "rev_dir", revDir)

			st.StartStep(state.StepSandboxRun)
			_ = writeState("state: SANDBOX_RUN start")
			logger.Info("step start", "step", string(state.StepSandboxRun))

			mode := strings.ToLower(strings.TrimSpace(task.Sandbox.Mode))
			if mode == "" {
				mode = "docker"
			}
			runResult := sandbox.Result{Mode: mode, ExitCode: 0}

			if workflow != "" {
				// workflow 模式下同样确保 summary.md 存在（VERIFY 需要）。
				summaryAbs := filepath.Join(revDir, "summary.md")
				if _, statErr := os.Stat(summaryAbs); statErr != nil {
					if !os.IsNotExist(statErr) {
						return statErr
					}
					content := []byte("# Summary\n\n" +
						"- task_id: " + taskID + "\n" +
						"- run_id: " + runID + "\n" +
						"- rev: " + rev + "\n" +
						"- workflow: " + workflow + "\n" +
						"- generated_at: " + time.Now().Format(time.RFC3339) + "\n\n" +
						"本次使用内置 workflow 执行。\n")
					if err := gw.CreateFile(filepath.ToSlash(filepath.Join("revs", rev, "summary.md")), content, 0o644, "workflow: summary.md"); err != nil {
						st.FailStep(state.ErrorInfo{
							Code:       "E_SANDBOX_RUN",
							Message:    err.Error(),
							OccurredAt: time.Now().Format(time.RFC3339),
						})
						_ = writeState("state: SANDBOX_RUN workflow summary fail")
						return err
					}
				}

				reg := execution.NewRegistry()
				_ = reg.Register(execution.NewDemo04Workflow())
				wf, err := reg.Get(workflow)
				if err != nil {
					st.FailStep(state.ErrorInfo{
						Code:       "E_SANDBOX_RUN",
						Message:    err.Error(),
						OccurredAt: time.Now().Format(time.RFC3339),
					})
					_ = writeState("state: SANDBOX_RUN workflow unknown")
					return err
				}

				ctx := context.Background()
				if task.Sandbox.TimeoutSeconds > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, time.Duration(task.Sandbox.TimeoutSeconds)*time.Second)
					defer cancel()
				}

				res, err := wf.Run(execution.Context{Ctx: ctx, GW: gw, TaskID: taskID, RunID: runID, Rev: rev})
				if err != nil {
					st.FailStep(state.ErrorInfo{
						Code:       "E_SANDBOX_RUN",
						Message:    err.Error(),
						OccurredAt: time.Now().Format(time.RFC3339),
					})
					_ = writeState("state: SANDBOX_RUN workflow fail")
					return err
				}
				if res.HasBlocker() {
					logger.Info("workflow finished with blockers (will be handled by VERIFY)", "workflow", workflow)
				}
				runResult = sandbox.Result{Mode: "workflow", ExitCode: 0}
			} else if len(task.Params.Entrypoint) == 0 {
				summaryAbs := filepath.Join(revDir, "summary.md")
				if _, statErr := os.Stat(summaryAbs); statErr != nil {
					if !os.IsNotExist(statErr) {
						return statErr
					}
					content := []byte("# Summary\n\n" +
						"- task_id: " + taskID + "\n" +
						"- run_id: " + runID + "\n" +
						"- rev: " + rev + "\n" +
						"- generated_at: " + time.Now().Format(time.RFC3339) + "\n\n" +
						"本次未指定沙箱执行命令，已生成默认产物。\n")
					if err := gw.CreateFile(filepath.ToSlash(filepath.Join("revs", rev, "summary.md")), content, 0o644, "sandbox: default summary.md"); err != nil {
						st.FailStep(state.ErrorInfo{
							Code:       "E_SANDBOX_RUN",
							Message:    err.Error(),
							OccurredAt: time.Now().Format(time.RFC3339),
						})
						_ = writeState("state: SANDBOX_RUN default artifacts fail")
						return err
					}
				}
			} else {
				runner, err := sandbox.NewRunner(task.Sandbox)
				if err != nil {
					st.FailStep(state.ErrorInfo{
						Code:       "E_SANDBOX_INIT",
						Message:    err.Error(),
						OccurredAt: time.Now().Format(time.RFC3339),
					})
					_ = writeState("state: SANDBOX_RUN init fail")
					return err
				}

				ctx := context.Background()
				if task.Sandbox.TimeoutSeconds > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, time.Duration(task.Sandbox.TimeoutSeconds)*time.Second)
					defer cancel()
				}

				runResult, err = runner.Run(ctx, sandbox.RunSpec{
					TaskID:     taskID,
					RunID:      runID,
					Rev:        rev,
					OutputDir:  revDir,
					Entrypoint: task.Params.Entrypoint,
				})
				if err != nil {
					logger.Error("sandbox run failed", "exit_code", runResult.ExitCode, "stderr", runResult.Stderr)
					{
						out := verify.IssuesFile{
							SchemaVersion: 1,
							TaskID:        taskID,
							Rev:           rev,
							Issues: []verify.Issue{{
								Severity: "blocker",
								Where:    "sandbox",
								What:     err.Error(),
								Action:   "检查沙箱命令/镜像/超时配置",
							}},
						}
						b, merr := json.MarshalIndent(out, "", "  ")
						if merr == nil {
							b = append(b, '\n')
							_ = gw.ReplaceFile(filepath.ToSlash(filepath.Join("revs", rev, "issues.json")), b, 0o644, "sandbox: write issues.json")
						}
					}
					st.FailStep(state.ErrorInfo{
						Code:       "E_SANDBOX_RUN",
						Message:    err.Error(),
						OccurredAt: time.Now().Format(time.RFC3339),
					})
					_ = writeState("state: SANDBOX_RUN fail")
					_ = json.NewEncoder(os.Stdout).Encode(runResult)
					return err
				}
			}
			logger.Info("sandbox run done", "exit_code", runResult.ExitCode)

			// 确保最小必交文件存在：issues.json（允许空）
			if _, statErr := os.Stat(filepath.Join(revDir, "issues.json")); statErr != nil {
				out := verify.IssuesFile{
					SchemaVersion: 1,
					TaskID:        taskID,
					Rev:           rev,
					Issues:        []verify.Issue{},
				}
				b, err := json.MarshalIndent(out, "", "  ")
				if err != nil {
					return err
				}
				b = append(b, '\n')
				if err := gw.ReplaceFile(filepath.ToSlash(filepath.Join("revs", rev, "issues.json")), b, 0o644, "ensure issues.json"); err != nil {
					return err
				}
			}

			st.EndStepSuccess()
			_ = writeState("state: SANDBOX_RUN done")
			logger.Info("step done", "step", string(state.StepSandboxRun))

			st.StartStep(state.StepVerify)
			_ = writeState("state: VERIFY start")
			logger.Info("step start", "step", string(state.StepVerify))
			verifyRes, err := verify.VerifyTaskRev(gw, taskID, rev)
			if err != nil {
				st.FailStep(state.ErrorInfo{
					Code:       "E_VERIFY",
					Message:    err.Error(),
					OccurredAt: time.Now().Format(time.RFC3339),
				})
				_ = writeState("state: VERIFY fail")
				return err
			}
			st.EndStepSuccess()
			_ = writeState("state: VERIFY done")
			logger.Info("step done", "step", string(state.StepVerify), "has_blocker", verifyRes.HasBlocker)

			if verifyRes.HasBlocker {
				st.FailStep(state.ErrorInfo{
					Code:       "E_VERIFY_BLOCKER",
					Message:    "verify failed with blocker issues",
					OccurredAt: time.Now().Format(time.RFC3339),
				})
				_ = writeState("state: VERIFY blocker")
				return fmt.Errorf("VERIFY 未通过（存在 blocker）")
			}

			st.StartStep(state.StepPack)
			_ = writeState("state: PACK start")
			logger.Info("step start", "step", string(state.StepPack))
			bundleRes, err := taskbundle.CreatePackBundle(taskbundle.CreatePackBundleOptions{
				TaskGW:              gw,
				TaskID:              taskID,
				RunID:               runID,
				Rev:                 rev,
				ToolVersion:         configToolVersion(),
				IncludeLatestCopies: true,
				ApprovalPolicy:      viper.GetString(flagApprovalPolicy),
				ApprovalGates:       viper.GetStringSlice(flagApprovalGate),
			})
			if err != nil {
				st.FailStep(state.ErrorInfo{
					Code:       "E_PACK",
					Message:    err.Error(),
					OccurredAt: time.Now().Format(time.RFC3339),
				})
				_ = writeState("state: PACK fail")
				return err
			}

			st.EndStepSuccess()
			st.MarkDone()
			_ = writeState("state: DONE")
			logger.Info("step done", "step", string(state.StepPack), "pack_id", bundleRes.PackID, "evidence_zip", bundleRes.EvidenceZipAbs)

			fmt.Fprintln(cmd.OutOrStdout(), td.Root())
			return nil
		},
	}

	cmd.Flags().String(flagBaseDir, "fs/taskctl", "任务根目录（会在其中创建 task_id 子目录）")
	cmd.Flags().String(flagTaskID, "", "任务 ID（可选；不传则自动生成）")
	cmd.Flags().String(flagSandboxMode, "docker", "沙箱模式：docker|local")
	cmd.Flags().String(flagSandboxImage, "alpine:3.20", "docker 沙箱镜像（sandbox-mode=docker）")
	cmd.Flags().String(flagSandboxNetwork, "none", "docker 网络：none|bridge|host（sandbox-mode=docker）")
	cmd.Flags().Int(flagTimeoutSeconds, 900, "沙箱超时（秒）")
	cmd.Flags().StringArray(flagEntrypoint, nil, "沙箱内执行命令（多次传入表示 argv；不传则生成默认产物）")
	cmd.Flags().String(flagWorkflow, "", "内置 workflow（可选；例如 demo04；与 --entrypoint 互斥）")
	cmd.Flags().String(flagLogLevel, "info", "日志级别：debug|info|warn|error")
	cmd.Flags().String(flagApprovalPolicy, "always", "审批策略：always|warn|gate|never（默认 always）")
	cmd.Flags().StringArray(flagApprovalGate, nil, "审批门禁（仅 approval-policy=gate 时生效；匹配 issues.where）")

	_ = viper.BindPFlags(cmd.Flags())

	return cmd
}

func configToolVersion() string {
	return "0.1.0-dev"
}
