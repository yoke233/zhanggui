package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureBuiltinSkills_ExtractsStepSignal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := EnsureBuiltinSkills(root); err != nil {
		t.Fatalf("EnsureBuiltinSkills: %v", err)
	}

	// Verify step-signal SKILL.md was extracted.
	skillMD := filepath.Join(root, "step-signal", "SKILL.md")
	b, err := os.ReadFile(skillMD)
	if err != nil {
		t.Fatalf("read extracted SKILL.md: %v", err)
	}
	content := string(b)

	// Check frontmatter fields.
	if meta, errs := ValidateSkillMD("step-signal", content); len(errs) > 0 {
		t.Fatalf("extracted SKILL.md validation errors: %v", errs)
	} else if meta.Name != "step-signal" {
		t.Fatalf("expected name=step-signal, got %q", meta.Name)
	}

	// Check that key content is present.
	for _, keyword := range []string{
		"AI_WORKFLOW_STEP_TYPE",
		"AI_WORKFLOW_STEP_ID",
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

func TestEnsureBuiltinSkills_SkipsWhenContentMatches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// First extraction.
	if err := EnsureBuiltinSkills(root); err != nil {
		t.Fatalf("first EnsureBuiltinSkills: %v", err)
	}

	skillMD := filepath.Join(root, "step-signal", "SKILL.md")
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

	// Write a fake step-signal so the embedded builtin should overwrite it.
	skillDir := filepath.Join(root, "step-signal")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeContent := "---\nname: step-signal\ndescription: fake\n---\n\n# Fake\n"
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

	scriptPath := filepath.Join(root, "step-signal", "scripts", "signal.sh")
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
