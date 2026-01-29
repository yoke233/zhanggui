package taskbundle

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoke233/zhanggui/internal/gateway"
	"github.com/yoke233/zhanggui/internal/sha256sum"
	"github.com/yoke233/zhanggui/internal/uuidv7"
	"github.com/yoke233/zhanggui/internal/verify"
)

type ApprovalRecord struct {
	SchemaVersion int         `json:"schema_version"`
	ApprovalID    string      `json:"approval_id"`
	TS            string      `json:"ts"`
	Decision      string      `json:"decision"`
	Actor         LedgerActor `json:"actor"`
	Correlation   Correlation `json:"correlation"`
	Notes         string      `json:"notes,omitempty"`
	Refs          []Ref       `json:"refs,omitempty"`
}

type ApproveDecisionOptions struct {
	TaskRootAbs string
	PackID      string

	TaskID string
	RunID  string
	Rev    string

	ApprovalID string
	Decision   string // GRANTED|DENIED

	Actor LedgerActor
	Notes string
}

func ApproveDecision(opts ApproveDecisionOptions) (ApprovalRecord, Ref, error) {
	if opts.TaskRootAbs == "" {
		return ApprovalRecord{}, Ref{}, fmt.Errorf("TaskRootAbs 不能为空")
	}
	if strings.TrimSpace(opts.PackID) == "" {
		return ApprovalRecord{}, Ref{}, fmt.Errorf("PackID 不能为空")
	}
	if strings.TrimSpace(opts.TaskID) == "" {
		return ApprovalRecord{}, Ref{}, fmt.Errorf("TaskID 不能为空")
	}
	if strings.TrimSpace(opts.Rev) == "" {
		return ApprovalRecord{}, Ref{}, fmt.Errorf("Rev 不能为空")
	}
	decision := strings.TrimSpace(strings.ToUpper(opts.Decision))
	if decision != "GRANTED" && decision != "DENIED" {
		return ApprovalRecord{}, Ref{}, fmt.Errorf("Decision 仅允许 GRANTED|DENIED")
	}
	approvalID := strings.TrimSpace(opts.ApprovalID)
	if approvalID == "" {
		return ApprovalRecord{}, Ref{}, fmt.Errorf("ApprovalID 不能为空")
	}
	if strings.TrimSpace(opts.Actor.Type) == "" || strings.TrimSpace(opts.Actor.ID) == "" {
		return ApprovalRecord{}, Ref{}, fmt.Errorf("Actor.type/id 不能为空")
	}

	bundleRootRel := filepath.ToSlash(filepath.Join("packs", opts.PackID))
	bundleRootAbs := filepath.Join(opts.TaskRootAbs, filepath.FromSlash(bundleRootRel))
	if _, err := os.Stat(bundleRootAbs); err != nil {
		return ApprovalRecord{}, Ref{}, err
	}

	ledgerRel := filepath.ToSlash(filepath.Join(bundleRootRel, "ledger", "events.jsonl"))
	ledgerAbs := filepath.Join(opts.TaskRootAbs, filepath.FromSlash(ledgerRel))

	// deny double-decision
	if decided, _, err := isApprovalDecided(ledgerAbs, approvalID); err != nil {
		return ApprovalRecord{}, Ref{}, err
	} else if decided {
		return ApprovalRecord{}, Ref{}, fmt.Errorf("approval 已有结论: %s", approvalID)
	}

	bundleAuditor, err := gateway.NewAuditor(filepath.Join(bundleRootAbs, "logs", "tool_audit.jsonl"))
	if err != nil {
		return ApprovalRecord{}, Ref{}, err
	}
	defer func() { _ = bundleAuditor.Close() }()

	bundleGW, err := gateway.New(opts.TaskRootAbs, gateway.Actor{AgentID: "taskctl", Role: "system"}, gateway.Linkage{
		TaskID: opts.TaskID,
		RunID:  opts.RunID,
		Rev:    opts.Rev,
	}, gateway.Policy{
		AllowedWritePrefixes: []string{bundleRootRel},
		AppendOnlyFiles:      []string{ledgerRel},
	}, bundleAuditor)
	if err != nil {
		return ApprovalRecord{}, Ref{}, err
	}

	corr := Correlation{
		BundleID: opts.PackID,
		PackID:   opts.PackID,
		TaskID:   opts.TaskID,
		RunID:    opts.RunID,
		Rev:      opts.Rev,
	}
	lw, err := NewLedgerWriter(bundleGW, ledgerRel, opts.Actor, corr)
	if err != nil {
		return ApprovalRecord{}, Ref{}, err
	}

	reportAbs := filepath.Join(bundleRootAbs, "verify", "report.json")
	reportShaHex, _ := sha256sum.FileHex(reportAbs)
	evidenceZipAbs := filepath.Join(bundleRootAbs, "pack", "evidence.zip")
	evidenceZipShaHex, _ := sha256sum.FileHex(evidenceZipAbs)

	var refs []Ref
	if reportShaHex != "" {
		refs = append(refs, Ref{
			Kind:   "report",
			ID:     "sha256:" + reportShaHex,
			Path:   filepath.ToSlash(filepath.Join("verify", "report.json")),
			Sha256: "sha256:" + reportShaHex,
		})
	}
	if evidenceZipShaHex != "" {
		refs = append(refs, Ref{
			Kind:   "artifact",
			ID:     "sha256:" + evidenceZipShaHex,
			Path:   filepath.ToSlash(filepath.Join("pack", "evidence.zip")),
			Sha256: "sha256:" + evidenceZipShaHex,
		})
	}

	rec := ApprovalRecord{
		SchemaVersion: 1,
		ApprovalID:    approvalID,
		TS:            time.Now().UTC().Format(time.RFC3339Nano),
		Decision:      decision,
		Actor:         opts.Actor,
		Correlation:   corr,
		Notes:         strings.TrimSpace(opts.Notes),
		Refs:          refs,
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return ApprovalRecord{}, Ref{}, err
	}
	b = append(b, '\n')

	shaHex := sha256sum.BytesHex(b)
	fileRel := filepath.ToSlash(filepath.Join(bundleRootRel, "evidence", "files", shaHex))
	if err := bundleGW.CreateFile(fileRel, b, 0o644, "approval: write record"); err != nil && !os.IsExist(err) {
		return ApprovalRecord{}, Ref{}, err
	}

	approvalRef := Ref{
		Kind:   "approval",
		ID:     "sha256:" + shaHex,
		Path:   filepath.ToSlash(filepath.Join("evidence", "files", shaHex)),
		Sha256: "sha256:" + shaHex,
	}

	eventType := "APPROVAL_GRANTED"
	if decision == "DENIED" {
		eventType = "APPROVAL_DENIED"
	}
	payload := map[string]any{
		"approval_id": approvalID,
		"decision":    decision,
	}
	allRefs := append([]Ref{approvalRef}, refs...)
	if err := lw.Append(eventType, allRefs, payload); err != nil {
		return ApprovalRecord{}, Ref{}, err
	}
	return rec, approvalRef, nil
}

type RequestApprovalOptions struct {
	TaskRootAbs string
	PackID      string

	TaskID string
	RunID  string
	Rev    string

	RequestedFor string
	Reason       string
	Actor        LedgerActor

	Refs []Ref
}

func RequestApproval(opts RequestApprovalOptions) (string, error) {
	if opts.TaskRootAbs == "" {
		return "", fmt.Errorf("TaskRootAbs 不能为空")
	}
	if strings.TrimSpace(opts.PackID) == "" {
		return "", fmt.Errorf("PackID 不能为空")
	}
	if strings.TrimSpace(opts.TaskID) == "" {
		return "", fmt.Errorf("TaskID 不能为空")
	}
	if strings.TrimSpace(opts.Rev) == "" {
		return "", fmt.Errorf("Rev 不能为空")
	}
	if strings.TrimSpace(opts.Actor.Type) == "" || strings.TrimSpace(opts.Actor.ID) == "" {
		return "", fmt.Errorf("Actor.type/id 不能为空")
	}

	bundleRootRel := filepath.ToSlash(filepath.Join("packs", opts.PackID))
	bundleRootAbs := filepath.Join(opts.TaskRootAbs, filepath.FromSlash(bundleRootRel))
	if _, err := os.Stat(bundleRootAbs); err != nil {
		return "", err
	}

	ledgerRel := filepath.ToSlash(filepath.Join(bundleRootRel, "ledger", "events.jsonl"))
	bundleAuditor, err := gateway.NewAuditor(filepath.Join(bundleRootAbs, "logs", "tool_audit.jsonl"))
	if err != nil {
		return "", err
	}
	defer func() { _ = bundleAuditor.Close() }()

	bundleGW, err := gateway.New(opts.TaskRootAbs, gateway.Actor{AgentID: "taskctl", Role: "system"}, gateway.Linkage{
		TaskID: opts.TaskID,
		RunID:  opts.RunID,
		Rev:    opts.Rev,
	}, gateway.Policy{
		AllowedWritePrefixes: []string{bundleRootRel},
		AppendOnlyFiles:      []string{ledgerRel},
	}, bundleAuditor)
	if err != nil {
		return "", err
	}

	corr := Correlation{
		BundleID: opts.PackID,
		PackID:   opts.PackID,
		TaskID:   opts.TaskID,
		RunID:    opts.RunID,
		Rev:      opts.Rev,
	}
	lw, err := NewLedgerWriter(bundleGW, ledgerRel, opts.Actor, corr)
	if err != nil {
		return "", err
	}

	approvalID := uuidv7.New()
	reqFor := strings.TrimSpace(opts.RequestedFor)
	if reqFor == "" {
		reqFor = "UNKNOWN"
	}
	payload := map[string]any{
		"approval_id":   approvalID,
		"requested_for": reqFor,
	}
	if s := strings.TrimSpace(opts.Reason); s != "" {
		payload["reason"] = s
	}
	if err := lw.Append("APPROVAL_REQUESTED", opts.Refs, payload); err != nil {
		return "", err
	}
	return approvalID, nil
}

func FindLatestPendingApprovalID(ledgerAbs string) (string, bool, error) {
	p, ok, err := findLatestPendingApprovalID(ledgerAbs)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	return p, true, nil
}

func shouldAutoRequestApproval(policy string, gates []string, issues []verify.Issue) bool {
	switch strings.TrimSpace(strings.ToLower(policy)) {
	case "", "always":
		return true
	case "never", "none", "off":
		return false
	case "warn":
		for _, it := range issues {
			if strings.EqualFold(strings.TrimSpace(it.Severity), "warn") {
				return true
			}
		}
		return false
	case "gate":
		if len(gates) == 0 {
			return false
		}
		set := map[string]struct{}{}
		for _, g := range gates {
			g = strings.ToLower(strings.TrimSpace(g))
			if g == "" {
				continue
			}
			set[g] = struct{}{}
		}
		for _, it := range issues {
			where := strings.ToLower(strings.TrimSpace(it.Where))
			if where == "" {
				continue
			}
			if _, ok := set[where]; ok {
				// blocker 会在外层被拒绝；这里只处理非 blocker 的情况即可。
				return true
			}
		}
		return false
	default:
		// Unknown policy → safe default: always.
		return true
	}
}

func findLatestPendingApprovalID(ledgerAbs string) (string, bool, error) {
	f, err := os.Open(ledgerAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	defer func() { _ = f.Close() }()

	type line struct {
		EventType string          `json:"event_type"`
		Payload   json.RawMessage `json:"payload"`
	}
	type payload struct {
		ApprovalID string `json:"approval_id"`
	}

	pending := map[string]struct{}{}
	order := []string{}

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var ln line
		if err := json.Unmarshal([]byte(raw), &ln); err != nil {
			return "", false, err
		}
		var pl payload
		if len(ln.Payload) > 0 {
			_ = json.Unmarshal(ln.Payload, &pl)
		}
		if pl.ApprovalID == "" {
			continue
		}

		switch ln.EventType {
		case "APPROVAL_REQUESTED":
			if _, ok := pending[pl.ApprovalID]; !ok {
				order = append(order, pl.ApprovalID)
			}
			pending[pl.ApprovalID] = struct{}{}
		case "APPROVAL_GRANTED", "APPROVAL_DENIED":
			delete(pending, pl.ApprovalID)
		}
	}
	if err := sc.Err(); err != nil {
		return "", false, err
	}
	for i := len(order) - 1; i >= 0; i-- {
		id := order[i]
		if _, ok := pending[id]; ok {
			return id, true, nil
		}
	}
	return "", false, nil
}

func isApprovalDecided(ledgerAbs string, approvalID string) (bool, string, error) {
	f, err := os.Open(ledgerAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", err
	}
	defer func() { _ = f.Close() }()

	type line struct {
		EventType string          `json:"event_type"`
		Payload   json.RawMessage `json:"payload"`
	}
	type payload struct {
		ApprovalID string `json:"approval_id"`
		Decision   string `json:"decision"`
	}

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var ln line
		if err := json.Unmarshal([]byte(raw), &ln); err != nil {
			return false, "", err
		}
		var pl payload
		if len(ln.Payload) > 0 {
			_ = json.Unmarshal(ln.Payload, &pl)
		}
		if pl.ApprovalID != approvalID {
			continue
		}
		switch ln.EventType {
		case "APPROVAL_GRANTED", "APPROVAL_DENIED":
			d := strings.TrimSpace(pl.Decision)
			if d == "" {
				d = ln.EventType
			}
			return true, d, nil
		}
	}
	if err := sc.Err(); err != nil {
		return false, "", err
	}
	return false, "", nil
}
