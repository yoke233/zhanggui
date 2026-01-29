package taskbundle_test

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke233/zhanggui/internal/gateway"
	"github.com/yoke233/zhanggui/internal/taskbundle"
	"github.com/yoke233/zhanggui/internal/taskdir"
	"github.com/yoke233/zhanggui/internal/uuidv7"
	"github.com/yoke233/zhanggui/internal/verify"
)

func TestCreatePackBundle_CreatesBundleAndLatestPointers(t *testing.T) {
	base := t.TempDir()
	taskID := uuidv7.NewAt(time.Now())
	runID := uuidv7.NewAt(time.Now())

	td, err := taskdir.CreateNew(base, taskID)
	if err != nil {
		t.Fatalf("CreateNew: %v", err)
	}

	rev := "r1"
	revDir := td.RevDir(rev)
	if err := os.MkdirAll(revDir, 0o755); err != nil {
		t.Fatalf("mkdir rev: %v", err)
	}
	if err := os.WriteFile(filepath.Join(revDir, "summary.md"), []byte("# Summary\n"), 0o644); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}
	{
		deliverDir := filepath.Join(revDir, "deliver")
		if err := os.MkdirAll(deliverDir, 0o755); err != nil {
			t.Fatalf("mkdir deliver: %v", err)
		}
		if err := os.WriteFile(filepath.Join(deliverDir, "report.md"), []byte("# Report\n"), 0o644); err != nil {
			t.Fatalf("write deliver/report.md: %v", err)
		}
		if err := os.WriteFile(filepath.Join(deliverDir, "slides.html"), []byte("<h1>Slides</h1>\n"), 0o644); err != nil {
			t.Fatalf("write deliver/slides.html: %v", err)
		}
	}
	{
		out := verify.IssuesFile{
			SchemaVersion: 1,
			TaskID:        taskID,
			Rev:           rev,
			Issues:        []verify.Issue{},
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			t.Fatalf("marshal issues: %v", err)
		}
		b = append(b, '\n')
		if err := os.WriteFile(filepath.Join(revDir, "issues.json"), b, 0o644); err != nil {
			t.Fatalf("write issues.json: %v", err)
		}
	}

	aud, err := gateway.NewAuditor(filepath.Join(td.LogsDir(), "tool_audit.jsonl"))
	if err != nil {
		t.Fatalf("NewAuditor: %v", err)
	}
	t.Cleanup(func() { _ = aud.Close() })

	gw, err := gateway.New(td.Root(), gateway.Actor{AgentID: "taskctl", Role: "system"}, gateway.Linkage{
		TaskID: taskID,
		RunID:  runID,
		Rev:    rev,
	}, gateway.Policy{
		AllowedWritePrefixes: []string{"task.json", "state.json", "logs/", "revs/", "packs/", "pack/", "verify/"},
		SingleWriterPrefixes: []string{"state.json", "pack/", "verify/"},
		SingleWriterRoles:    []string{"system"},
		LockFile:             "logs/locks/taskctl.lock",
	}, aud)
	if err != nil {
		t.Fatalf("New gateway: %v", err)
	}
	if err := gw.AcquireLock(); err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	t.Cleanup(func() { _ = gw.ReleaseLock() })

	verifyRes, err := verify.VerifyTaskRev(gw, taskID, rev)
	if err != nil {
		t.Fatalf("VerifyTaskRev: %v", err)
	}
	if verifyRes.HasBlocker {
		t.Fatalf("expected no blocker issues")
	}

	res, err := taskbundle.CreatePackBundle(taskbundle.CreatePackBundleOptions{
		TaskGW:              gw,
		TaskID:              taskID,
		RunID:               runID,
		Rev:                 rev,
		ToolVersion:         "0.1.0-test",
		IncludeLatestCopies: true,
	})
	if err != nil {
		t.Fatalf("CreatePackBundle: %v", err)
	}

	if _, err := os.Stat(res.LedgerAbs); err != nil {
		t.Fatalf("ledger missing: %v", err)
	}
	if _, err := os.Stat(res.ArtifactsZipAbs); err != nil {
		t.Fatalf("artifacts.zip missing: %v", err)
	}
	if _, err := os.Stat(res.EvidenceZipAbs); err != nil {
		t.Fatalf("evidence.zip missing: %v", err)
	}

	ledgerBytes, err := os.ReadFile(res.LedgerAbs)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if !strings.Contains(string(ledgerBytes), `"event_type":"BUNDLE_CREATED"`) {
		t.Fatalf("ledger should contain BUNDLE_CREATED")
	}

	// latest pointer
	latestBytes, err := os.ReadFile(filepath.Join(td.Root(), "pack", "latest.json"))
	if err != nil {
		t.Fatalf("read pack/latest.json: %v", err)
	}
	var latest taskbundle.LatestPointer
	if err := json.Unmarshal(latestBytes, &latest); err != nil {
		t.Fatalf("unmarshal pack/latest.json: %v", err)
	}
	if latest.PackID != res.PackID {
		t.Fatalf("latest.pack_id mismatch: got=%s want=%s", latest.PackID, res.PackID)
	}

	// "latest copies" exist
	if _, err := os.Stat(filepath.Join(td.Root(), "pack", "artifacts.zip")); err != nil {
		t.Fatalf("latest pack/artifacts.zip missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(td.Root(), "pack", "evidence.zip")); err != nil {
		t.Fatalf("latest pack/evidence.zip missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(td.Root(), "pack", "manifest.json")); err != nil {
		t.Fatalf("latest pack/manifest.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(td.Root(), "verify", "report.json")); err != nil {
		t.Fatalf("latest verify/report.json missing: %v", err)
	}
	{
		reportBytes, err := os.ReadFile(filepath.Join(td.Root(), "verify", "report.json"))
		if err != nil {
			t.Fatalf("read latest verify/report.json: %v", err)
		}
		var report taskbundle.VerifyReport
		if err := json.Unmarshal(reportBytes, &report); err != nil {
			t.Fatalf("unmarshal latest verify/report.json: %v", err)
		}

		find := func(id string) (string, bool) {
			for _, r := range report.Results {
				if r.CriteriaID == id {
					return r.Status, true
				}
			}
			return "", false
		}
		for _, id := range []string{"AC-006", "AC-007"} {
			status, ok := find(id)
			if !ok {
				t.Fatalf("verify/report.json missing criteria: %s", id)
			}
			if status != "PASS" {
				t.Fatalf("criteria %s status=%s, want PASS", id, status)
			}
		}
	}

	// evidence.zip contains required entries
	zr, err := zip.OpenReader(res.EvidenceZipAbs)
	if err != nil {
		t.Fatalf("open evidence.zip: %v", err)
	}
	defer func() { _ = zr.Close() }()

	names := map[string]struct{}{}
	for _, f := range zr.File {
		names[f.Name] = struct{}{}
	}
	for _, want := range []string{
		"ledger/events.jsonl",
		"logs/tool_audit.jsonl",
		"verify/report.json",
		"artifacts/manifest.json",
		"pack/artifacts.zip",
	} {
		if _, ok := names[want]; !ok {
			t.Fatalf("evidence.zip missing entry: %s", want)
		}
	}
}
