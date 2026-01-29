package taskbundle

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoke233/zhanggui/internal/gateway"
	"github.com/yoke233/zhanggui/internal/manifest"
	"github.com/yoke233/zhanggui/internal/pack"
	"github.com/yoke233/zhanggui/internal/reporoot"
	"github.com/yoke233/zhanggui/internal/sha256sum"
	"github.com/yoke233/zhanggui/internal/uuidv7"
	"github.com/yoke233/zhanggui/internal/verify"
)

type CreatePackBundleOptions struct {
	TaskGW *gateway.Gateway

	TaskID string
	RunID  string
	Rev    string

	ToolVersion string

	// Optional override. If empty, defaults to docs/proposals/acceptance_criteria_v1.yaml under repo root.
	CriteriaPathAbs string

	// If true, also maintains task_root/pack/** and task_root/verify/report.json as "latest" convenience copies.
	IncludeLatestCopies bool

	// ApprovalPolicy controls whether to emit APPROVAL_REQUESTED in the bundle ledger.
	// Allowed values: always|warn|gate|never. Default: always.
	ApprovalPolicy string
	// ApprovalGates works with ApprovalPolicy=gate: match Issue.Where in issues.json.
	ApprovalGates []string

	// If true, skips generating pack/evidence.zip and omits EVIDENCE_PACK_CREATED.
	// This makes it easy to evolve to an "A" flow later (generate evidence.zip after approvals).
	SkipEvidenceZip bool
}

type CreatePackBundleResult struct {
	PackID string

	BundleRootRel string
	BundleRootAbs string

	LedgerRel string
	LedgerAbs string

	ArtifactsZipRel string
	ArtifactsZipAbs string

	EvidenceZipRel string
	EvidenceZipAbs string
}

func CreatePackBundle(opts CreatePackBundleOptions) (CreatePackBundleResult, error) {
	if opts.TaskGW == nil {
		return CreatePackBundleResult{}, fmt.Errorf("TaskGW 不能为空")
	}
	if strings.TrimSpace(opts.TaskID) == "" {
		return CreatePackBundleResult{}, fmt.Errorf("TaskID 不能为空")
	}
	if strings.TrimSpace(opts.Rev) == "" {
		return CreatePackBundleResult{}, fmt.Errorf("Rev 不能为空")
	}
	if strings.TrimSpace(opts.ToolVersion) == "" {
		return CreatePackBundleResult{}, fmt.Errorf("ToolVersion 不能为空")
	}

	taskRootAbs := opts.TaskGW.Root()

	// Ensure mutable pointers exist.
	_ = opts.TaskGW.MkdirAll("packs", 0o755, "ensure packs dir")
	_ = opts.TaskGW.MkdirAll("pack", 0o755, "ensure pack dir")
	_ = opts.TaskGW.MkdirAll("verify", 0o755, "ensure verify dir")

	packID := uuidv7.New()
	bundleRootRel := filepath.ToSlash(filepath.Join("packs", packID))
	bundleRootAbs := filepath.Join(taskRootAbs, filepath.FromSlash(bundleRootRel))
	if _, err := os.Stat(bundleRootAbs); err == nil {
		return CreatePackBundleResult{}, fmt.Errorf("bundle 已存在: %s", bundleRootRel)
	} else if err != nil && !os.IsNotExist(err) {
		return CreatePackBundleResult{}, err
	}

	bundleAuditAbs := filepath.Join(bundleRootAbs, "logs", "tool_audit.jsonl")
	bundleAuditor, err := gateway.NewAuditor(bundleAuditAbs)
	if err != nil {
		return CreatePackBundleResult{}, err
	}
	defer func() { _ = bundleAuditor.Close() }()

	ledgerRel := filepath.ToSlash(filepath.Join(bundleRootRel, "ledger", "events.jsonl"))
	bundleGW, err := gateway.New(taskRootAbs, gateway.Actor{AgentID: "taskctl", Role: "system"}, gateway.Linkage{
		TaskID: opts.TaskID,
		RunID:  opts.RunID,
		Rev:    opts.Rev,
	}, gateway.Policy{
		AllowedWritePrefixes: []string{bundleRootRel},
		AppendOnlyFiles:      []string{ledgerRel},
	}, bundleAuditor)
	if err != nil {
		return CreatePackBundleResult{}, err
	}

	mkdir := func(rel string) error {
		return bundleGW.MkdirAll(filepath.ToSlash(filepath.Join(bundleRootRel, rel)), 0o755, "bundle: mkdir "+rel)
	}
	for _, rel := range []string{"ledger", "evidence/files", "verify", "artifacts", "pack"} {
		if err := mkdir(rel); err != nil {
			return CreatePackBundleResult{}, err
		}
	}

	// Optional: snapshot task root state.json into bundle root (辅助调试；不作为审计依据).
	if b, err := os.ReadFile(filepath.Join(taskRootAbs, "state.json")); err == nil {
		_ = bundleGW.CreateFile(filepath.ToSlash(filepath.Join(bundleRootRel, "state.json")), b, 0o644, "bundle: snapshot state.json")
	}

	actor := LedgerActor{Type: "system", ID: "taskctl", Role: "system"}
	corr := Correlation{
		BundleID: packID,
		PackID:   packID,
		TaskID:   opts.TaskID,
		RunID:    opts.RunID,
		Rev:      opts.Rev,
	}
	lw, err := NewLedgerWriter(bundleGW, ledgerRel, actor, corr)
	if err != nil {
		return CreatePackBundleResult{}, err
	}

	if err := lw.Append("BUNDLE_CREATED", nil, map[string]any{"tool_version": opts.ToolVersion}); err != nil {
		return CreatePackBundleResult{}, err
	}
	if err := lw.Append("STEP_STARTED", nil, map[string]any{"step": "VERIFY"}); err != nil {
		return CreatePackBundleResult{}, err
	}

	criteriaPathAbs := opts.CriteriaPathAbs
	if strings.TrimSpace(criteriaPathAbs) == "" {
		repoRoot, err := reporoot.FindByGoMod("")
		if err != nil {
			return CreatePackBundleResult{}, err
		}
		criteriaPathAbs = filepath.Join(repoRoot, "docs", "proposals", "acceptance_criteria_v1.yaml")
	}
	criteriaBytes, err := os.ReadFile(criteriaPathAbs)
	if err != nil {
		return CreatePackBundleResult{}, err
	}
	cf, err := parseCriteriaYAML(criteriaBytes)
	if err != nil {
		return CreatePackBundleResult{}, err
	}
	criteriaShaHex := sha256sum.BytesHex(criteriaBytes)
	criteriaRel := filepath.ToSlash(filepath.Join("evidence", "files", criteriaShaHex))
	{
		dst := filepath.ToSlash(filepath.Join(bundleRootRel, criteriaRel))
		if err := bundleGW.CreateFile(dst, criteriaBytes, 0o644, "evidence: snapshot criteria"); err != nil && !os.IsExist(err) {
			return CreatePackBundleResult{}, err
		}
	}
	if err := lw.Append("CRITERIA_SNAPSHOTTED", []Ref{{
		Kind:   "criteria",
		ID:     "sha256:" + criteriaShaHex,
		Path:   criteriaRel,
		Sha256: "sha256:" + criteriaShaHex,
	}}, map[string]any{"criteria_id": cf.CriteriaSet.ID, "criteria_version": cf.CriteriaSet.Version}); err != nil {
		return CreatePackBundleResult{}, err
	}

	// Read issues.json as part of acceptance context.
	issuesPathAbs := filepath.Join(taskRootAbs, "revs", opts.Rev, "issues.json")
	issuesBytes, err := os.ReadFile(issuesPathAbs)
	if err != nil {
		return CreatePackBundleResult{}, err
	}
	var issuesFile verify.IssuesFile
	if err := json.Unmarshal(issuesBytes, &issuesFile); err != nil {
		return CreatePackBundleResult{}, fmt.Errorf("issues.json 不是合法 JSON: %w", err)
	}
	hasBlocker := false
	for _, it := range issuesFile.Issues {
		if strings.EqualFold(strings.TrimSpace(it.Severity), "blocker") {
			hasBlocker = true
			break
		}
	}
	if hasBlocker {
		return CreatePackBundleResult{}, fmt.Errorf("VERIFY 未通过（存在 blocker）")
	}

	if err := lw.Append("STEP_FINISHED", nil, map[string]any{"step": "VERIFY", "outcome": "pass"}); err != nil {
		return CreatePackBundleResult{}, err
	}
	if err := lw.Append("STEP_STARTED", nil, map[string]any{"step": "PACK"}); err != nil {
		return CreatePackBundleResult{}, err
	}

	man, err := manifest.Generate(taskRootAbs, opts.TaskID, opts.Rev)
	if err != nil {
		return CreatePackBundleResult{}, err
	}
	manifestBytes, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return CreatePackBundleResult{}, err
	}
	manifestBytes = append(manifestBytes, '\n')
	manifestShaHex := sha256sum.BytesHex(manifestBytes)
	{
		dst := filepath.ToSlash(filepath.Join(bundleRootRel, "artifacts", "manifest.json"))
		if err := bundleGW.CreateFile(dst, manifestBytes, 0o644, "bundle: write artifacts/manifest.json"); err != nil {
			return CreatePackBundleResult{}, err
		}
	}
	if err := lw.Append("ARTIFACT_MANIFEST_WRITTEN", []Ref{{
		Kind:   "artifact",
		ID:     "sha256:" + manifestShaHex,
		Path:   filepath.ToSlash(filepath.Join("artifacts", "manifest.json")),
		Sha256: "sha256:" + manifestShaHex,
	}}, map[string]any{}); err != nil {
		return CreatePackBundleResult{}, err
	}

	artifactsZipRel := filepath.ToSlash(filepath.Join(bundleRootRel, "pack", "artifacts.zip"))
	if err := bundleGW.CreateFileWith(artifactsZipRel, 0o644, "bundle: create pack/artifacts.zip", func(tmpAbs string) error {
		return pack.CreateZip(taskRootAbs, tmpAbs, man)
	}); err != nil {
		return CreatePackBundleResult{}, err
	}
	artifactsZipAbs := filepath.Join(taskRootAbs, filepath.FromSlash(artifactsZipRel))
	artifactsZipShaHex, err := sha256sum.FileHex(artifactsZipAbs)
	if err != nil {
		return CreatePackBundleResult{}, err
	}
	if err := pack.ValidateZipMatchesManifest(artifactsZipAbs, man); err != nil {
		return CreatePackBundleResult{}, fmt.Errorf("产物包校验失败: %w", err)
	}

	// Build verify/report.json
	report, reportBytes, reportShaHex, err := buildVerifyReport(cf, corr, criteriaShaHex, criteriaRel, manifestShaHex, man, artifactsZipShaHex, issuesFile)
	if err != nil {
		return CreatePackBundleResult{}, err
	}
	{
		dst := filepath.ToSlash(filepath.Join(bundleRootRel, "verify", "report.json"))
		if err := bundleGW.CreateFile(dst, reportBytes, 0o644, "bundle: write verify/report.json"); err != nil {
			return CreatePackBundleResult{}, err
		}
	}
	if err := lw.Append("VERIFY_REPORT_WRITTEN", []Ref{{
		Kind:   "report",
		ID:     "sha256:" + reportShaHex,
		Path:   filepath.ToSlash(filepath.Join("verify", "report.json")),
		Sha256: "sha256:" + reportShaHex,
	}}, map[string]any{"summary": report.Summary}); err != nil {
		return CreatePackBundleResult{}, err
	}

	evidenceZipRel := filepath.ToSlash(filepath.Join(bundleRootRel, "pack", "evidence.zip"))
	evidenceZipAbs := filepath.Join(taskRootAbs, filepath.FromSlash(evidenceZipRel))
	evidenceZipShaHex := ""
	if !opts.SkipEvidenceZip {
		if err := bundleGW.CreateFileWith(evidenceZipRel, 0o644, "bundle: create pack/evidence.zip", func(tmpAbs string) error {
			return CreateEvidenceZip(bundleRootAbs, tmpAbs)
		}); err != nil {
			return CreatePackBundleResult{}, err
		}
		var err error
		evidenceZipShaHex, err = sha256sum.FileHex(evidenceZipAbs)
		if err != nil {
			return CreatePackBundleResult{}, err
		}
	}

	if !opts.SkipEvidenceZip {
		if err := lw.Append("EVIDENCE_PACK_CREATED", []Ref{
			{
				Kind:   "artifact",
				ID:     "sha256:" + artifactsZipShaHex,
				Path:   filepath.ToSlash(filepath.Join("pack", "artifacts.zip")),
				Sha256: "sha256:" + artifactsZipShaHex,
			},
			{
				Kind:   "artifact",
				ID:     "sha256:" + evidenceZipShaHex,
				Path:   filepath.ToSlash(filepath.Join("pack", "evidence.zip")),
				Sha256: "sha256:" + evidenceZipShaHex,
			},
		}, map[string]any{"layout": "nested"}); err != nil {
			return CreatePackBundleResult{}, err
		}
	}
	if err := lw.Append("STEP_FINISHED", nil, map[string]any{"step": "PACK", "outcome": "pass"}); err != nil {
		return CreatePackBundleResult{}, err
	}

	approvalPolicy := strings.TrimSpace(strings.ToLower(opts.ApprovalPolicy))
	if approvalPolicy == "" {
		approvalPolicy = "always"
	}
	if shouldAutoRequestApproval(approvalPolicy, opts.ApprovalGates, issuesFile.Issues) {
		approvalID := uuidv7.New()
		var refs []Ref
		refs = append(refs, Ref{
			Kind:   "report",
			ID:     "sha256:" + reportShaHex,
			Path:   filepath.ToSlash(filepath.Join("verify", "report.json")),
			Sha256: "sha256:" + reportShaHex,
		})
		refs = append(refs, Ref{
			Kind:   "artifact",
			ID:     "sha256:" + manifestShaHex,
			Path:   filepath.ToSlash(filepath.Join("artifacts", "manifest.json")),
			Sha256: "sha256:" + manifestShaHex,
		})
		refs = append(refs, Ref{
			Kind:   "artifact",
			ID:     "sha256:" + artifactsZipShaHex,
			Path:   filepath.ToSlash(filepath.Join("pack", "artifacts.zip")),
			Sha256: "sha256:" + artifactsZipShaHex,
		})
		if !opts.SkipEvidenceZip && evidenceZipShaHex != "" {
			refs = append(refs, Ref{
				Kind:   "artifact",
				ID:     "sha256:" + evidenceZipShaHex,
				Path:   filepath.ToSlash(filepath.Join("pack", "evidence.zip")),
				Sha256: "sha256:" + evidenceZipShaHex,
			})
		}
		payload := map[string]any{
			"approval_id":   approvalID,
			"requested_for": "PACK",
			"policy": map[string]any{
				"mode":  approvalPolicy,
				"gates": opts.ApprovalGates,
			},
		}
		if err := lw.Append("APPROVAL_REQUESTED", refs, payload); err != nil {
			return CreatePackBundleResult{}, err
		}
	}

	// Update latest pointer (single-writer; not audit truth source).
	latest := LatestPointer{
		SchemaVersion: 1,
		TaskID:        opts.TaskID,
		PackID:        packID,
		Rev:           opts.Rev,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Paths: LatestPaths{
			BundleRoot:   bundleRootRel + "/",
			EvidenceZip:  evidenceZipRel,
			ArtifactsZip: artifactsZipRel,
		},
	}
	latestBytes, err := json.MarshalIndent(latest, "", "  ")
	if err != nil {
		return CreatePackBundleResult{}, err
	}
	latestBytes = append(latestBytes, '\n')
	if err := opts.TaskGW.ReplaceFile(filepath.ToSlash(filepath.Join("pack", "latest.json")), latestBytes, 0o644, "pack: update latest.json"); err != nil {
		return CreatePackBundleResult{}, err
	}

	if opts.IncludeLatestCopies {
		_ = opts.TaskGW.ReplaceFile(filepath.ToSlash(filepath.Join("pack", "manifest.json")), manifestBytes, 0o644, "pack: update latest manifest.json")
		_ = opts.TaskGW.ReplaceFile(filepath.ToSlash(filepath.Join("verify", "report.json")), reportBytes, 0o644, "verify: update latest report.json")

		copyFile := func(srcAbs string, dstRel string) error {
			return opts.TaskGW.ReplaceFileWith(dstRel, 0o644, "pack: update latest copy", func(tmpAbs string) error {
				in, err := os.Open(srcAbs)
				if err != nil {
					return err
				}
				defer func() { _ = in.Close() }()

				if err := os.MkdirAll(filepath.Dir(tmpAbs), 0o755); err != nil {
					return err
				}
				out, err := os.Create(tmpAbs)
				if err != nil {
					return err
				}
				_, werr := io.Copy(out, in)
				cerr := out.Close()
				if werr != nil {
					return werr
				}
				return cerr
			})
		}
		_ = copyFile(artifactsZipAbs, filepath.ToSlash(filepath.Join("pack", "artifacts.zip")))
		if !opts.SkipEvidenceZip {
			_ = copyFile(evidenceZipAbs, filepath.ToSlash(filepath.Join("pack", "evidence.zip")))
		}
	}

	return CreatePackBundleResult{
		PackID: packID,

		BundleRootRel: bundleRootRel,
		BundleRootAbs: bundleRootAbs,

		LedgerRel: ledgerRel,
		LedgerAbs: filepath.Join(taskRootAbs, filepath.FromSlash(ledgerRel)),

		ArtifactsZipRel: artifactsZipRel,
		ArtifactsZipAbs: artifactsZipAbs,

		EvidenceZipRel: evidenceZipRel,
		EvidenceZipAbs: evidenceZipAbs,
	}, nil
}
