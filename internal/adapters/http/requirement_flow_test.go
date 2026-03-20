package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	agentapp "github.com/yoke233/zhanggui/internal/application/agent"
	requirementapp "github.com/yoke233/zhanggui/internal/application/requirementapp"
	"github.com/yoke233/zhanggui/internal/core"
)

func TestAPI_RequirementToProposalToInitiativeFlow(t *testing.T) {
	h, ts := setupAPI(t)
	ctx := context.Background()

	projectBackend, err := h.store.CreateProject(ctx, &core.Project{
		Name:        "backend-api",
		Kind:        core.ProjectDev,
		Description: "认证后端",
		Metadata: map[string]string{
			core.ProjectMetaScope:      "负责登录、鉴权与 OTP 校验",
			core.ProjectMetaKeywords:   "auth, otp, login",
			core.ProjectMetaAgentHints: "backend-dev, arch-reviewer",
		},
	})
	if err != nil {
		t.Fatalf("CreateProject(backend): %v", err)
	}
	projectFrontend, err := h.store.CreateProject(ctx, &core.Project{
		Name:        "frontend-web",
		Kind:        core.ProjectDev,
		Description: "登录前端",
		Metadata: map[string]string{
			core.ProjectMetaScope:      "负责登录页面与交互",
			core.ProjectMetaKeywords:   "frontend, login, web",
			core.ProjectMetaAgentHints: "frontend-dev",
		},
	})
	if err != nil {
		t.Fatalf("CreateProject(frontend): %v", err)
	}
	for _, item := range []struct {
		projectID int64
		rootURI   string
	}{
		{projectID: projectBackend, rootURI: "D:/workspace/backend-api"},
		{projectID: projectFrontend, rootURI: "D:/workspace/frontend-web"},
	} {
		if _, err := h.store.CreateResourceSpace(ctx, &core.ResourceSpace{
			ProjectID: item.projectID,
			Kind:      "local_fs",
			RootURI:   item.rootURI,
		}); err != nil {
			t.Fatalf("CreateResourceSpace(%d): %v", item.projectID, err)
		}
	}

	registry := agentapp.NewConfigRegistry()
	registry.LoadProfiles([]*core.AgentProfile{
		{ID: "arch-reviewer", Role: core.RoleLead, Capabilities: []string{"architecture", "review"}},
		{ID: "backend-dev", Role: core.RoleWorker, Capabilities: []string{"backend", "auth"}},
		{ID: "frontend-dev", Role: core.RoleWorker, Capabilities: []string{"frontend", "ui"}},
	})
	h.registry = registry
	threadPool := &stubThreadAgentRuntime{
		promptReplies: map[string]string{
			"arch-reviewer": "先统一 OTP 方案边界与审批点。",
			"backend-dev":   "后端先补 OTP 校验接口与兼容逻辑。",
			"frontend-dev":  "[FINAL] 前端随后接入 OTP 输入与状态反馈。",
		},
	}
	h.threadPool = threadPool
	h.requirementLLM = stubRequirementCompleter{raw: mustMarshalJSONRaw(t, map[string]any{
		"summary": "给登录系统增加 OTP 两步验证",
		"type":    "cross_project",
		"matched_projects": []map[string]any{
			{"project_id": projectBackend, "reason": "需要后端 OTP 校验与接口变更", "relevance": "high"},
			{"project_id": projectFrontend, "reason": "需要登录页补充 OTP 输入与状态反馈", "relevance": "high"},
		},
		"suggested_agents": []map[string]any{
			{"profile_id": "arch-reviewer", "reason": "负责收敛方案"},
			{"profile_id": "backend-dev", "reason": "负责接口与鉴权逻辑"},
			{"profile_id": "frontend-dev", "reason": "负责前端交互"},
		},
		"complexity":             "high",
		"suggested_meeting_mode": "group_chat",
		"risks":                  []string{"需要兼容旧登录流程"},
		"suggested_thread": map[string]any{
			"title": "讨论：登录 OTP 两步验证",
			"context_refs": []map[string]any{
				{"project_id": projectBackend, "access": "read"},
				{"project_id": projectFrontend, "access": "read"},
			},
			"agents":             []string{"arch-reviewer", "backend-dev", "frontend-dev"},
			"meeting_mode":       "group_chat",
			"meeting_max_rounds": 6,
		},
	})}

	resp, err := post(ts, "/requirements/analyze", map[string]any{
		"description": "给用户登录系统增加 OTP 两步验证，后端要校验，前端要新增输入流程。",
	})
	if err != nil {
		t.Fatalf("analyze requirement: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("analyze status = %d, want 200", resp.StatusCode)
	}
	var analysis requirementapp.AnalyzeResult
	if err := decodeJSON(resp, &analysis); err != nil {
		t.Fatalf("decode analyze: %v", err)
	}
	if analysis.Analysis.Type != "cross_project" {
		t.Fatalf("analysis.type = %q", analysis.Analysis.Type)
	}

	resp, err = post(ts, "/requirements/create-thread", map[string]any{
		"description":   "给用户登录系统增加 OTP 两步验证，后端要校验，前端要新增输入流程。",
		"context":       "先做虚拟链路验证，不依赖真实 ACP。",
		"owner_id":      "alice",
		"analysis":      analysis.Analysis,
		"thread_config": analysis.SuggestedThread,
	})
	if err != nil {
		t.Fatalf("create requirement thread: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create thread status = %d, want 201", resp.StatusCode)
	}
	var created struct {
		Thread  core.Thread        `json:"thread"`
		Agents  []string           `json:"agents"`
		Message core.ThreadMessage `json:"message"`
	}
	if err := decodeJSON(resp, &created); err != nil {
		t.Fatalf("decode created thread: %v", err)
	}
	if len(created.Agents) != 3 {
		t.Fatalf("agents = %+v, want 3", created.Agents)
	}
	if created.Message.ThreadID != created.Thread.ID || created.Message.Role != "human" {
		t.Fatalf("created message = %+v", created.Message)
	}
	if got := created.Thread.Metadata["meeting_mode"]; got != "group_chat" {
		t.Fatalf("thread meeting_mode = %v, want group_chat", got)
	}
	if got := created.Thread.Metadata["meeting_max_rounds"]; got != int64(6) && got != 6 && got != float64(6) {
		t.Fatalf("thread meeting_max_rounds = %v, want 6", got)
	}
	if _, exists := created.Thread.Metadata["skip_default_context_refs"]; exists {
		t.Fatalf("internal metadata leaked: %+v", created.Thread.Metadata)
	}

	contextRefs, err := h.store.ListThreadContextRefs(ctx, created.Thread.ID)
	if err != nil {
		t.Fatalf("ListThreadContextRefs: %v", err)
	}
	if len(contextRefs) != 2 {
		t.Fatalf("context refs = %d, want 2", len(contextRefs))
	}

	inviteCalls := threadPool.snapshotInviteCalls()
	if len(inviteCalls) != 3 {
		t.Fatalf("invite calls = %d, want 3", len(inviteCalls))
	}
	gotInviteProfiles := map[string]struct{}{}
	for _, call := range inviteCalls {
		gotInviteProfiles[call.profileID] = struct{}{}
	}
	for _, profileID := range []string{"arch-reviewer", "backend-dev", "frontend-dev"} {
		if _, ok := gotInviteProfiles[profileID]; !ok {
			t.Fatalf("invite calls missing %q: %+v", profileID, inviteCalls)
		}
	}

	waitForThreadCondition(t, 2*time.Second, func() error {
		promptCalls := threadPool.snapshotPromptCalls()
		if len(promptCalls) != 3 {
			return fmt.Errorf("prompt calls = %d, want 3", len(promptCalls))
		}
		msgs, err := h.store.ListThreadMessages(ctx, created.Thread.ID, 20, 0)
		if err != nil {
			return err
		}
		if len(msgs) < 5 {
			return fmt.Errorf("messages = %d, want at least 5 after group_chat dispatch", len(msgs))
		}
		return nil
	})

	sendCalls := threadPool.snapshotSendCalls()
	if len(sendCalls) != 0 {
		t.Fatalf("send calls = %d, want 0 in group_chat meeting mode", len(sendCalls))
	}
	promptCalls := threadPool.snapshotPromptCalls()
	gotPromptProfiles := map[string]struct{}{}
	for _, call := range promptCalls {
		gotPromptProfiles[call.profileID] = struct{}{}
		if call.threadID != created.Thread.ID {
			t.Fatalf("prompt call thread_id = %d, want %d", call.threadID, created.Thread.ID)
		}
		if call.message == "" || call.message == created.Message.Content {
			t.Fatalf("prompt call message = %q, want meeting prompt", call.message)
		}
		if !containsAny(call.message, "会议模式：group_chat", "你正在参加 thread 内的主持人会议") {
			t.Fatalf("prompt call message = %q, want group chat prompt", call.message)
		}
	}
	for _, profileID := range []string{"arch-reviewer", "backend-dev", "frontend-dev"} {
		if _, ok := gotPromptProfiles[profileID]; !ok {
			t.Fatalf("prompt calls missing %q: %+v", profileID, promptCalls)
		}
	}

	msgs, err := h.store.ListThreadMessages(ctx, created.Thread.ID, 20, 0)
	if err != nil {
		t.Fatalf("ListThreadMessages: %v", err)
	}
	if len(msgs) < 5 {
		t.Fatalf("messages = %d, want at least 5", len(msgs))
	}
	if msgs[len(msgs)-1].Role != "system" {
		t.Fatalf("last message role = %q, want system", msgs[len(msgs)-1].Role)
	}
	if got := msgs[len(msgs)-1].Metadata["meeting_mode"]; got != "group_chat" {
		t.Fatalf("summary meeting_mode = %v, want group_chat", got)
	}

	resp, err = post(ts, "/threads/"+itoa64(created.Thread.ID)+"/proposals", map[string]any{
		"title":             "登录 OTP 两步验证方案",
		"summary":           "拆成后端接口与前端交互两个 work item",
		"content":           "先后端后前端",
		"proposed_by":       "arch-reviewer",
		"source_message_id": created.Message.ID,
		"work_item_drafts": []map[string]any{
			{"temp_id": "backend", "project_id": projectBackend, "title": "实现 OTP 校验接口", "priority": "high"},
			{"temp_id": "frontend", "project_id": projectFrontend, "title": "接入 OTP 登录交互", "depends_on": []string{"backend"}, "priority": "high"},
		},
	})
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("proposal status = %d, want 201", resp.StatusCode)
	}
	var proposal core.ThreadProposal
	if err := decodeJSON(resp, &proposal); err != nil {
		t.Fatalf("decode proposal: %v", err)
	}

	resp, err = post(ts, "/proposals/"+itoa64(proposal.ID)+"/submit", map[string]any{})
	if err != nil {
		t.Fatalf("submit proposal: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("submit status = %d, want 200", resp.StatusCode)
	}

	resp, err = post(ts, "/proposals/"+itoa64(proposal.ID)+"/approve", map[string]any{
		"reviewed_by": "alice",
		"review_note": "流程正确，可以进入 initiative",
	})
	if err != nil {
		t.Fatalf("approve proposal: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve status = %d, want 200", resp.StatusCode)
	}
	if err := decodeJSON(resp, &proposal); err != nil {
		t.Fatalf("decode approved proposal: %v", err)
	}
	if proposal.Status != core.ProposalMerged || proposal.InitiativeID == nil {
		t.Fatalf("proposal = %+v", proposal)
	}

	initiative, err := h.store.GetInitiative(ctx, *proposal.InitiativeID)
	if err != nil {
		t.Fatalf("GetInitiative: %v", err)
	}
	if initiative.Status != core.InitiativeDraft {
		t.Fatalf("initiative status = %s, want draft", initiative.Status)
	}
	items, err := h.store.ListInitiativeItems(ctx, initiative.ID)
	if err != nil {
		t.Fatalf("ListInitiativeItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("initiative items = %d, want 2", len(items))
	}
}

func containsAny(value string, subs ...string) bool {
	for _, sub := range subs {
		if sub != "" && strings.Contains(value, sub) {
			return true
		}
	}
	return false
}
