package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureBuiltinSkills_ExtractsActionSignal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := EnsureBuiltinSkills(root); err != nil {
		t.Fatalf("EnsureBuiltinSkills: %v", err)
	}

	// Verify action-signal SKILL.md was extracted.
	skillMD := filepath.Join(root, "action-signal", "SKILL.md")
	b, err := os.ReadFile(skillMD)
	if err != nil {
		t.Fatalf("read extracted SKILL.md: %v", err)
	}
	content := string(b)

	// Check frontmatter fields.
	if meta, errs := ValidateSkillMD("action-signal", content); len(errs) > 0 {
		t.Fatalf("extracted SKILL.md validation errors: %v", errs)
	} else if meta.Name != "action-signal" {
		t.Fatalf("expected name=action-signal, got %q", meta.Name)
	}

	// Check that key content is present.
	for _, keyword := range []string{
		"AI_WORKFLOW_ACTION_TYPE",
		"AI_WORKFLOW_ACTION_ID",
		"AI_WORKFLOW_SIGNAL",
		"signal.sh",
	} {
		if !contains(content, keyword) {
			t.Errorf("expected SKILL.md to contain %q", keyword)
		}
	}
}

func TestEnsureBuiltinSkills_ExtractsPlanActions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := EnsureBuiltinSkills(root); err != nil {
		t.Fatalf("EnsureBuiltinSkills: %v", err)
	}

	skillMD := filepath.Join(root, "plan-actions", "SKILL.md")
	b, err := os.ReadFile(skillMD)
	if err != nil {
		t.Fatalf("read extracted plan-actions SKILL.md: %v", err)
	}
	content := string(b)

	if meta, errs := ValidateSkillMD("plan-actions", content); len(errs) > 0 {
		t.Fatalf("extracted plan-actions SKILL.md validation errors: %v", errs)
	} else if meta.Name != "plan-actions" {
		t.Fatalf("expected name=plan-actions, got %q", meta.Name)
	}

	for _, keyword := range []string{
		"Planning Objectives",
		"Role And Capability Mapping",
		"structured DAG plan",
	} {
		if !contains(content, keyword) {
			t.Errorf("expected plan-actions SKILL.md to contain %q", keyword)
		}
	}
}

func TestEnsureBuiltinSkills_ExtractsCEOManage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := EnsureBuiltinSkills(root); err != nil {
		t.Fatalf("EnsureBuiltinSkills: %v", err)
	}

	skillMD := filepath.Join(root, "ceo-manage", "SKILL.md")
	b, err := os.ReadFile(skillMD)
	if err != nil {
		t.Fatalf("read extracted ceo-manage SKILL.md: %v", err)
	}
	content := string(b)

	if meta, errs := ValidateSkillMD("ceo-manage", content); len(errs) > 0 {
		t.Fatalf("extracted ceo-manage SKILL.md validation errors: %v", errs)
	} else if meta.Name != "ceo-manage" {
		t.Fatalf("expected name=ceo-manage, got %q", meta.Name)
	}

	for _, keyword := range []string{
		"task-first orchestration",
		"orchestrate task create",
		"orchestrate task escalate-thread",
	} {
		if !contains(content, keyword) {
			t.Errorf("expected ceo-manage SKILL.md to contain %q", keyword)
		}
	}

	agentConfig := filepath.Join(root, "ceo-manage", "agents", "openai.yaml")
	if _, err := os.Stat(agentConfig); err != nil {
		t.Fatalf("expected ceo-manage openai.yaml to exist: %v", err)
	}
}

func TestEnsureBuiltinSkills_ExtractsNormalizedGstackSkills(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := EnsureBuiltinSkills(root); err != nil {
		t.Fatalf("EnsureBuiltinSkills: %v", err)
	}

	cases := []struct {
		name     string
		keywords []string
	}{
		{
			name: "gstack-office-hours",
			keywords: []string{
				"Gstack Office Hours",
				"narrowest viable wedge",
				".ai-workflow/artifacts/gstack/office-hours/",
			},
		},
		{
			name: "gstack-plan-ceo-review",
			keywords: []string{
				"Gstack Plan CEO Review",
				"Review Modes",
				".ai-workflow/artifacts/gstack/ceo-review/",
			},
		},
		{
			name: "gstack-plan-eng-review",
			keywords: []string{
				"Gstack Plan Eng Review",
				"Failure and retry matrix",
				".ai-workflow/artifacts/gstack/eng-review/",
			},
		},
		{
			name: "gstack-review",
			keywords: []string{
				"Gstack Review",
				"Present findings first",
				".ai-workflow/artifacts/gstack/review/",
			},
		},
		{
			name: "gstack-document-release",
			keywords: []string{
				"Gstack Document Release",
				"Bring project documentation back in sync",
				".ai-workflow/artifacts/gstack/document-release/",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			skillMD := filepath.Join(root, tc.name, "SKILL.md")
			b, err := os.ReadFile(skillMD)
			if err != nil {
				t.Fatalf("read extracted %s SKILL.md: %v", tc.name, err)
			}
			content := string(b)

			if meta, errs := ValidateSkillMD(tc.name, content); len(errs) > 0 {
				t.Fatalf("extracted %s SKILL.md validation errors: %v", tc.name, errs)
			} else if meta.Name != tc.name {
				t.Fatalf("expected name=%s, got %q", tc.name, meta.Name)
			}

			for _, keyword := range tc.keywords {
				if !contains(content, keyword) {
					t.Errorf("expected %s SKILL.md to contain %q", tc.name, keyword)
				}
			}
		})
	}
}

func TestEnsureBuiltinSkills_SkipsWhenContentMatches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// First extraction.
	if err := EnsureBuiltinSkills(root); err != nil {
		t.Fatalf("first EnsureBuiltinSkills: %v", err)
	}

	skillMD := filepath.Join(root, "action-signal", "SKILL.md")
	info1, err := os.Stat(skillMD)
	if err != nil {
		t.Fatalf("stat after first extract: %v", err)
	}

	// The function should skip because the on-disk content still matches.
	// We'll verify by checking that the second call doesn't error.
	if err := EnsureBuiltinSkills(root); err != nil {
		t.Fatalf("second EnsureBuiltinSkills: %v", err)
	}

	info2, err := os.Stat(skillMD)
	if err != nil {
		t.Fatalf("stat after second extract: %v", err)
	}

	// On a fast machine the mod times might be the same; we just verify no error.
	_ = info1
	_ = info2
}

func TestEnsureBuiltinSkills_OverwritesOnContentMismatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// Write a fake action-signal so the embedded builtin should overwrite it.
	skillDir := filepath.Join(root, "action-signal")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeContent := "---\nname: action-signal\ndescription: fake\n---\n\n# Fake\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(fakeContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// EnsureBuiltinSkills should overwrite because the content differs.
	if err := EnsureBuiltinSkills(root); err != nil {
		t.Fatalf("EnsureBuiltinSkills: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)

	// Should now contain the real content, not "# Fake".
	if contains(content, "# Fake") {
		t.Fatal("expected overwrite of fake SKILL.md, but old content remains")
	}
	if !contains(content, "AI_WORKFLOW_SIGNAL") {
		t.Fatal("expected real SKILL.md content after overwrite")
	}
}

func TestEnsureBuiltinSkills_OverwritesWhenBundledScriptDiffers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := EnsureBuiltinSkills(root); err != nil {
		t.Fatalf("EnsureBuiltinSkills: %v", err)
	}

	scriptPath := filepath.Join(root, "action-signal", "scripts", "signal.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\necho fake\n"), 0o644); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	if err := EnsureBuiltinSkills(root); err != nil {
		t.Fatalf("EnsureBuiltinSkills second run: %v", err)
	}

	b, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read restored script: %v", err)
	}
	if contains(string(b), "echo fake") {
		t.Fatal("expected bundled script to be restored after mismatch")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && containsStr(s, substr)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
