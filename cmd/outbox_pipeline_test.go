package cmd

import "testing"

func TestOutboxPipelineRunFlags(t *testing.T) {
	t.Parallel()

	cmd := newOutboxPipelineRunCmd(nil)
	if err := cmd.ParseFlags([]string{
		"--issue", "local#1",
		"--project-dir", ".",
		"--prompt-file", "mailbox/issue.md",
		"--workflow", "workflow.toml",
		"--coding-role", "backend",
		"--max-review-round", "4",
		"--max-test-round", "5",
	}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	issueRef, _ := cmd.Flags().GetString("issue")
	if issueRef != "local#1" {
		t.Fatalf("issue = %q, want local#1", issueRef)
	}

	projectDir, _ := cmd.Flags().GetString("project-dir")
	if projectDir != "." {
		t.Fatalf("project-dir = %q, want .", projectDir)
	}

	promptFile, _ := cmd.Flags().GetString("prompt-file")
	if promptFile != "mailbox/issue.md" {
		t.Fatalf("prompt-file = %q, want mailbox/issue.md", promptFile)
	}

	workflowFile, _ := cmd.Flags().GetString("workflow")
	if workflowFile != "workflow.toml" {
		t.Fatalf("workflow = %q, want workflow.toml", workflowFile)
	}

	codingRole, _ := cmd.Flags().GetString("coding-role")
	if codingRole != "backend" {
		t.Fatalf("coding-role = %q, want backend", codingRole)
	}

	maxReviewRound, _ := cmd.Flags().GetInt("max-review-round")
	if maxReviewRound != 4 {
		t.Fatalf("max-review-round = %d, want 4", maxReviewRound)
	}

	maxTestRound, _ := cmd.Flags().GetInt("max-test-round")
	if maxTestRound != 5 {
		t.Fatalf("max-test-round = %d, want 5", maxTestRound)
	}
}
