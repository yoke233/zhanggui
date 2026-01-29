package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yoke233/zhanggui/internal/sha256sum"
	"github.com/yoke233/zhanggui/internal/taskbundle"
	"github.com/yoke233/zhanggui/internal/taskdir"
)

func NewApproveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve",
		Short: "对指定 pack（Bundle）写入审批事件（不修改 evidence.zip，B 档）",
	}

	cmd.AddCommand(newApproveRequestCmd())
	cmd.AddCommand(newApproveGrantCmd())
	cmd.AddCommand(newApproveDenyCmd())

	return cmd
}

func newApproveRequestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "request <task_dir>",
		Short: "创建一条 APPROVAL_REQUESTED（如需对后续动作人工确认）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			td, err := taskdir.Open(args[0])
			if err != nil {
				return err
			}
			taskRootAbs := td.Root()

			packID, _ := cmd.Flags().GetString("pack-id")
			if strings.TrimSpace(packID) == "" {
				p, err := taskbundle.ReadLatestPointer(taskRootAbs)
				if err != nil {
					return err
				}
				packID = p.PackID
			}

			corr, err := taskbundle.ReadBundleCorrelation(taskRootAbs, packID)
			if err != nil {
				return err
			}

			actorID, _ := cmd.Flags().GetString("actor-id")
			if strings.TrimSpace(actorID) == "" {
				actorID = strings.TrimSpace(os.Getenv("USERNAME"))
			}
			if actorID == "" {
				actorID = "local_user"
			}
			reqFor, _ := cmd.Flags().GetString("for")
			reason, _ := cmd.Flags().GetString("reason")

			bundleRootAbs := filepath.Join(taskRootAbs, "packs", packID)
			refs := collectBundleRefs(bundleRootAbs)

			approvalID, err := taskbundle.RequestApproval(taskbundle.RequestApprovalOptions{
				TaskRootAbs:  taskRootAbs,
				PackID:       packID,
				TaskID:       nonEmpty(corr.TaskID, td.TaskID()),
				RunID:        corr.RunID,
				Rev:          corr.Rev,
				RequestedFor: reqFor,
				Reason:       reason,
				Actor:        taskbundle.LedgerActor{Type: "human", ID: actorID, Role: "approver"},
				Refs:         refs,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), approvalID)
			return nil
		},
	}
	cmd.Flags().String("pack-id", "", "指定 pack_id（默认取 pack/latest.json）")
	cmd.Flags().String("for", "UNKNOWN", "本次审批请求用于什么动作（如 UPLOAD|EXPORT|PUBLISH）")
	cmd.Flags().String("reason", "", "请求原因（可选，注意脱敏）")
	cmd.Flags().String("actor-id", "", "审批人 ID（默认取环境变量 USERNAME）")
	return cmd
}

func newApproveGrantCmd() *cobra.Command {
	return newApproveDecideCmd("grant")
}

func newApproveDenyCmd() *cobra.Command {
	return newApproveDecideCmd("deny")
}

func newApproveDecideCmd(mode string) *cobra.Command {
	event := "GRANTED"
	if mode == "deny" {
		event = "DENIED"
	}

	cmd := &cobra.Command{
		Use:   mode + " <task_dir>",
		Short: "写入 APPROVAL_" + event + "（不会回写 evidence.zip）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			td, err := taskdir.Open(args[0])
			if err != nil {
				return err
			}
			taskRootAbs := td.Root()

			packID, _ := cmd.Flags().GetString("pack-id")
			if strings.TrimSpace(packID) == "" {
				p, err := taskbundle.ReadLatestPointer(taskRootAbs)
				if err != nil {
					return err
				}
				packID = p.PackID
			}

			corr, err := taskbundle.ReadBundleCorrelation(taskRootAbs, packID)
			if err != nil {
				return err
			}

			ledgerAbs := filepath.Join(taskRootAbs, "packs", packID, "ledger", "events.jsonl")
			approvalID, _ := cmd.Flags().GetString("approval-id")
			if strings.TrimSpace(approvalID) == "" {
				id, ok, err := taskbundle.FindLatestPendingApprovalID(ledgerAbs)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("未找到待审批项（ledger 中没有 pending APPROVAL_REQUESTED）")
				}
				approvalID = id
			}

			actorID, _ := cmd.Flags().GetString("actor-id")
			if strings.TrimSpace(actorID) == "" {
				actorID = strings.TrimSpace(os.Getenv("USERNAME"))
			}
			if actorID == "" {
				actorID = "local_user"
			}
			notes, _ := cmd.Flags().GetString("notes")

			_, _, err = taskbundle.ApproveDecision(taskbundle.ApproveDecisionOptions{
				TaskRootAbs: taskRootAbs,
				PackID:      packID,
				TaskID:      nonEmpty(corr.TaskID, td.TaskID()),
				RunID:       corr.RunID,
				Rev:         corr.Rev,
				ApprovalID:  approvalID,
				Decision:    event,
				Actor:       taskbundle.LedgerActor{Type: "human", ID: actorID, Role: "approver"},
				Notes:       notes,
			})
			return err
		},
	}
	cmd.Flags().String("pack-id", "", "指定 pack_id（默认取 pack/latest.json）")
	cmd.Flags().String("approval-id", "", "指定 approval_id（默认取 ledger 中最新的 pending）")
	cmd.Flags().String("actor-id", "", "审批人 ID（默认取环境变量 USERNAME）")
	cmd.Flags().String("notes", "", "备注（可选，注意脱敏）")
	return cmd
}

func collectBundleRefs(bundleRootAbs string) []taskbundle.Ref {
	type item struct {
		kind string
		rel  string
	}
	candidates := []item{
		{kind: "report", rel: filepath.ToSlash(filepath.Join("verify", "report.json"))},
		{kind: "artifact", rel: filepath.ToSlash(filepath.Join("artifacts", "manifest.json"))},
		{kind: "artifact", rel: filepath.ToSlash(filepath.Join("pack", "artifacts.zip"))},
		{kind: "artifact", rel: filepath.ToSlash(filepath.Join("pack", "evidence.zip"))},
	}

	var refs []taskbundle.Ref
	for _, it := range candidates {
		abs := filepath.Join(bundleRootAbs, filepath.FromSlash(it.rel))
		shaHex, err := sha256sum.FileHex(abs)
		if err != nil {
			continue
		}
		refs = append(refs, taskbundle.Ref{
			Kind:   it.kind,
			ID:     "sha256:" + shaHex,
			Path:   it.rel,
			Sha256: "sha256:" + shaHex,
		})
	}
	return refs
}

func nonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
