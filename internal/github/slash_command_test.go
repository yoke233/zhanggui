package github

import "testing"

func TestParseSlashCommand_Approve(t *testing.T) {
	cmd, ok, err := ParseSlashCommand("/approve")
	if err != nil {
		t.Fatalf("ParseSlashCommand() error = %v", err)
	}
	if !ok {
		t.Fatal("expected slash command to be parsed")
	}
	if cmd.Type != SlashCommandApprove {
		t.Fatalf("expected approve command, got %s", cmd.Type)
	}
}

func TestParseSlashCommand_Reject_WithStageAndReason(t *testing.T) {
	cmd, ok, err := ParseSlashCommand("/reject implement 数据模型需要重做")
	if err != nil {
		t.Fatalf("ParseSlashCommand() error = %v", err)
	}
	if !ok {
		t.Fatal("expected slash command to be parsed")
	}
	if cmd.Type != SlashCommandReject {
		t.Fatalf("expected reject command, got %s", cmd.Type)
	}
	if cmd.Stage != "implement" {
		t.Fatalf("expected stage implement, got %s", cmd.Stage)
	}
	if cmd.Reason != "数据模型需要重做" {
		t.Fatalf("expected reason preserved, got %q", cmd.Reason)
	}
}

func TestParseSlashCommand_Reject_CodeReview(t *testing.T) {
	cmd, ok, err := ParseSlashCommand("/reject review 测试覆盖不足")
	if err != nil {
		t.Fatalf("ParseSlashCommand() error = %v", err)
	}
	if !ok {
		t.Fatal("expected slash command to be parsed")
	}
	if cmd.Type != SlashCommandReject {
		t.Fatalf("expected reject command, got %s", cmd.Type)
	}
	if cmd.Stage != "review" {
		t.Fatalf("expected stage review, got %s", cmd.Stage)
	}
}

func TestSlashACL_UnauthorizedUser_Denied(t *testing.T) {
	allowed := IsSlashCommandAllowed("guest-user", "NONE", SlashCommandApprove, SlashACLConfig{})
	if allowed {
		t.Fatal("expected NONE association approve command to be denied")
	}
}

func TestSlashACL_AuthorAssociationMatrix_AppliesDefaultPermissions(t *testing.T) {
	if !IsSlashCommandAllowed("contributor-a", "CONTRIBUTOR", SlashCommandStatus, SlashACLConfig{}) {
		t.Fatal("expected contributor to be allowed for /status")
	}
	if IsSlashCommandAllowed("contributor-a", "CONTRIBUTOR", SlashCommandApprove, SlashACLConfig{}) {
		t.Fatal("expected contributor to be denied for /approve")
	}
}

func TestSlashACL_WhitelistOverridesAuthorAssociation(t *testing.T) {
	allowed := IsSlashCommandAllowed("trusted-user", "NONE", SlashCommandApprove, SlashACLConfig{
		AuthorizedUsernames: []string{"trusted-user"},
	})
	if !allowed {
		t.Fatal("expected authorized whitelist user to be allowed")
	}
}
