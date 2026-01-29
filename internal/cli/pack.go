package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/yoke233/zhanggui/internal/gateway"
	"github.com/yoke233/zhanggui/internal/state"
	"github.com/yoke233/zhanggui/internal/taskbundle"
	"github.com/yoke233/zhanggui/internal/taskdir"
	"github.com/yoke233/zhanggui/internal/verify"
)

func NewPackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pack <task_dir> [--rev rN]",
		Short: "对已有任务目录执行 VERIFY + PACK（不进沙箱）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskDir := args[0]
			td, err := taskdir.Open(taskDir)
			if err != nil {
				return err
			}

			rev, _ := cmd.Flags().GetString("rev")
			if rev == "" {
				rev, err = td.LatestRev()
				if err != nil {
					return err
				}
			}

			statePath := filepath.Join(td.Root(), "state.json")
			st, err := state.ReadJSON(statePath)
			if err != nil {
				return err
			}

			auditor, err := gateway.NewAuditor(filepath.Join(td.LogsDir(), "tool_audit.jsonl"))
			if err != nil {
				return err
			}
			defer func() { _ = auditor.Close() }()

			gw, err := gateway.New(td.Root(), gateway.Actor{AgentID: "taskctl", Role: "system"}, gateway.Linkage{TaskID: st.TaskID, RunID: st.RunID, Rev: rev}, gateway.Policy{
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

			writeState := func(detail string) error {
				b, err := json.MarshalIndent(st, "", "  ")
				if err != nil {
					return err
				}
				b = append(b, '\n')
				return gw.ReplaceFile("state.json", b, 0o644, detail)
			}

			st.StartStep(state.StepVerify)
			_ = writeState("state: VERIFY start")

			verifyRes, err := verify.VerifyTaskRev(gw, st.TaskID, rev)
			if err != nil {
				st.FailStep(state.ErrorInfo{Code: "E_VERIFY", Message: err.Error(), OccurredAt: time.Now().Format(time.RFC3339)})
				_ = writeState("state: VERIFY fail")
				return err
			}
			st.EndStepSuccess()
			_ = writeState("state: VERIFY done")
			if verifyRes.HasBlocker {
				st.FailStep(state.ErrorInfo{Code: "E_VERIFY_BLOCKER", Message: "verify failed with blocker issues", OccurredAt: time.Now().Format(time.RFC3339)})
				_ = writeState("state: VERIFY blocker")
				return fmt.Errorf("VERIFY 未通过（存在 blocker）")
			}

			st.StartStep(state.StepPack)
			_ = writeState("state: PACK start")
			bundleRes, err := taskbundle.CreatePackBundle(taskbundle.CreatePackBundleOptions{
				TaskGW:              gw,
				TaskID:              st.TaskID,
				RunID:               st.RunID,
				Rev:                 rev,
				ToolVersion:         configToolVersion(),
				IncludeLatestCopies: true,
				ApprovalPolicy:      getFlagString(cmd, "approval-policy"),
				ApprovalGates:       getFlagStringArray(cmd, "approval-gate"),
			})
			if err != nil {
				st.FailStep(state.ErrorInfo{Code: "E_PACK", Message: err.Error(), OccurredAt: time.Now().Format(time.RFC3339)})
				_ = writeState("state: PACK fail")
				return err
			}

			st.EndStepSuccess()
			st.MarkDone()
			_ = writeState("state: DONE")

			fmt.Fprintln(cmd.OutOrStdout(), bundleRes.EvidenceZipAbs)
			return nil
		},
	}

	cmd.Flags().String("rev", "", "指定要打包的 revision（默认取最新）")
	cmd.Flags().String("approval-policy", "always", "审批策略：always|warn|gate|never（默认 always）")
	cmd.Flags().StringArray("approval-gate", nil, "审批门禁（仅 approval-policy=gate 时生效；匹配 issues.where）")

	return cmd
}

func getFlagString(cmd *cobra.Command, name string) string {
	s, _ := cmd.Flags().GetString(name)
	return s
}

func getFlagStringArray(cmd *cobra.Command, name string) []string {
	s, _ := cmd.Flags().GetStringArray(name)
	return s
}
