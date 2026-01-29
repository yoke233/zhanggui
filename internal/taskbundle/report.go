package taskbundle

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yoke233/zhanggui/internal/manifest"
	"github.com/yoke233/zhanggui/internal/sha256sum"
	"github.com/yoke233/zhanggui/internal/verify"
)

func buildVerifyReport(
	cf criteriaFile,
	corr Correlation,
	criteriaShaHex string,
	criteriaRel string,
	manifestShaHex string,
	man manifest.Manifest,
	artifactsZipShaHex string,
	issuesFile verify.IssuesFile,
) (VerifyReport, []byte, string, error) {
	if corr.BundleID == "" {
		return VerifyReport{}, nil, "", fmt.Errorf("correlation.bundle_id 不能为空")
	}
	if criteriaShaHex == "" || criteriaRel == "" {
		return VerifyReport{}, nil, "", fmt.Errorf("criteria 快照信息缺失")
	}
	if manifestShaHex == "" {
		return VerifyReport{}, nil, "", fmt.Errorf("manifestShaHex 不能为空")
	}
	if artifactsZipShaHex == "" {
		return VerifyReport{}, nil, "", fmt.Errorf("artifactsZipShaHex 不能为空")
	}

	files := make(map[string]manifest.FileEntry, len(man.Files))
	for _, f := range man.Files {
		files[f.Path] = f
	}

	artifactsZipRef := Ref{
		Kind:   "artifact",
		ID:     "sha256:" + artifactsZipShaHex,
		Path:   filepath.ToSlash(filepath.Join("pack", "artifacts.zip")),
		Sha256: "sha256:" + artifactsZipShaHex,
	}
	manifestRef := Ref{
		Kind:   "artifact",
		ID:     "sha256:" + manifestShaHex,
		Path:   filepath.ToSlash(filepath.Join("artifacts", "manifest.json")),
		Sha256: "sha256:" + manifestShaHex,
	}

	memberRef := func(entryPath string) (Ref, bool) {
		ent, ok := files[entryPath]
		if !ok {
			return Ref{}, false
		}
		size := ent.Size
		return Ref{
			Kind:   "artifact",
			ID:     "sha256:" + ent.Sha256,
			Path:   filepath.ToSlash(filepath.Join("pack", "artifacts.zip")) + "#" + entryPath,
			Sha256: "sha256:" + ent.Sha256,
			Size:   &size,
		}, true
	}

	hasBlocker := false
	for _, it := range issuesFile.Issues {
		if strings.EqualFold(strings.TrimSpace(it.Severity), "blocker") {
			hasBlocker = true
			break
		}
	}

	var results []ReportResult
	for _, c := range cf.Criteria {
		status := "SKIP"
		notes := "未实现该条验收检查"
		refs := []Ref{}

		switch strings.TrimSpace(c.ID) {
		case "AC-001":
			notes = ""
			status = "FAIL"
			if ref, ok := memberRef(filepath.ToSlash(filepath.Join("revs", corr.Rev, "summary.md"))); ok {
				status = "PASS"
				refs = append(refs, ref, artifactsZipRef)
			} else {
				notes = "未在产物清单中找到 summary.md"
				refs = append(refs, manifestRef)
			}
		case "AC-002":
			notes = ""
			status = "FAIL"
			if ref, ok := memberRef(filepath.ToSlash(filepath.Join("revs", corr.Rev, "issues.json"))); ok {
				status = "PASS"
				refs = append(refs, ref, artifactsZipRef)
			} else {
				notes = "未在产物清单中找到 issues.json"
				refs = append(refs, manifestRef)
			}
		case "AC-003":
			notes = ""
			status = "PASS"
			if hasBlocker {
				status = "FAIL"
				notes = "issues.json 存在 blocker"
			}
			if ref, ok := memberRef(filepath.ToSlash(filepath.Join("revs", corr.Rev, "issues.json"))); ok {
				refs = append(refs, ref, artifactsZipRef)
			} else {
				refs = append(refs, manifestRef)
			}
		case "AC-004":
			notes = ""
			status = "PASS"
			refs = append(refs, manifestRef, artifactsZipRef)
		case "AC-005":
			// 注意：evidence.zip 自包含 verify/report.json，因此 report 内不记录 evidence.zip 的 sha256（避免自指循环）。
			notes = "evidence.zip 生成后可离线复核（layout=nested）；sha256 由 ledger/EVIDENCE_PACK_CREATED 提供"
			status = "PASS"
		case "AC-006":
			notes = ""
			status = "FAIL"
			if ref, ok := memberRef(filepath.ToSlash(filepath.Join("revs", corr.Rev, "deliver", "report.md"))); ok {
				status = "PASS"
				refs = append(refs, ref, artifactsZipRef)
			} else {
				notes = "未在产物清单中找到 deliver/report.md"
				refs = append(refs, manifestRef)
			}
		case "AC-007":
			notes = ""
			status = "FAIL"
			if ref, ok := memberRef(filepath.ToSlash(filepath.Join("revs", corr.Rev, "deliver", "slides.html"))); ok {
				status = "PASS"
				refs = append(refs, ref, artifactsZipRef)
			} else {
				notes = "未在产物清单中找到 deliver/slides.html"
				refs = append(refs, manifestRef)
			}
		default:
			// keep SKIP
		}

		results = append(results, ReportResult{
			CriteriaID:   c.ID,
			Status:       status,
			Severity:     strings.TrimSpace(c.Severity),
			EvidenceRefs: refs,
			Notes:        notes,
		})
	}

	summary := ReportSummary{}
	for _, r := range results {
		switch r.Status {
		case "PASS":
			summary.Passed++
		case "FAIL":
			summary.Failed++
			if strings.EqualFold(strings.TrimSpace(r.Severity), "blocker") {
				summary.Blocker++
			}
		}
	}

	report := VerifyReport{
		SchemaVersion: 1,
		Correlation:   corr,
		Criteria: CriteriaRef{
			ID:      cf.CriteriaSet.ID,
			Version: cf.CriteriaSet.Version,
			Sha256:  "sha256:" + criteriaShaHex,
			Path:    criteriaRel,
		},
		Results: results,
		Summary: summary,
	}

	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return VerifyReport{}, nil, "", err
	}
	b = append(b, '\n')
	shaHex := sha256sum.BytesHex(b)
	return report, b, shaHex, nil
}
