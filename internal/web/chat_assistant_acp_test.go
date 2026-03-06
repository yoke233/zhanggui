package web

import (
	"context"
	"testing"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/teamleader"
)

func TestShouldLoadPersistedChatSession(t *testing.T) {
	tests := []struct {
		name              string
		policy            acpclient.SessionPolicy
		persistedSession  string
		wantLoadPersisted bool
	}{
		{
			name:              "empty session id",
			policy:            acpclient.SessionPolicy{Reuse: true, PreferLoadSession: true},
			persistedSession:  " ",
			wantLoadPersisted: false,
		},
		{
			name:              "reuse disabled",
			policy:            acpclient.SessionPolicy{Reuse: false, PreferLoadSession: true},
			persistedSession:  "sid-old",
			wantLoadPersisted: false,
		},
		{
			name:              "prefer load disabled",
			policy:            acpclient.SessionPolicy{Reuse: true, PreferLoadSession: false},
			persistedSession:  "sid-old",
			wantLoadPersisted: false,
		},
		{
			name:              "reuse and prefer load enabled",
			policy:            acpclient.SessionPolicy{Reuse: true, PreferLoadSession: true},
			persistedSession:  "sid-old",
			wantLoadPersisted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldLoadPersistedChatSession(tt.policy, tt.persistedSession)
			if got != tt.wantLoadPersisted {
				t.Fatalf("shouldLoadPersistedChatSession() = %v, want %v", got, tt.wantLoadPersisted)
			}
		})
	}
}

func TestStartWebChatSessionSkipsLoadWhenReuseDisabled(t *testing.T) {
	client := &stubACPClient{
		loadResp: acpproto.SessionId("sid-loaded"),
		newResp:  acpproto.SessionId("sid-new"),
	}
	role := acpclient.RoleProfile{
		SessionPolicy: acpclient.SessionPolicy{
			Reuse:             false,
			PreferLoadSession: true,
		},
	}

	session, err := startWebChatSession(
		context.Background(),
		client,
		"team_leader",
		role,
		"sid-old",
		"D:/repo/demo",
		teamleader.MCPEnvConfig{},
		nil,
	)
	if err != nil {
		t.Fatalf("startWebChatSession() error = %v", err)
	}
	if string(session) != "sid-new" {
		t.Fatalf("session id = %q, want %q", string(session), "sid-new")
	}
	if len(client.loadReqs) != 0 {
		t.Fatalf("LoadSession calls = %d, want 0", len(client.loadReqs))
	}
	if len(client.newReqs) != 1 {
		t.Fatalf("NewSession calls = %d, want 1", len(client.newReqs))
	}
}

func TestStartWebChatSessionSkipsLoadWhenPreferLoadDisabled(t *testing.T) {
	client := &stubACPClient{
		loadResp: acpproto.SessionId("sid-loaded"),
		newResp:  acpproto.SessionId("sid-new"),
	}
	role := acpclient.RoleProfile{
		SessionPolicy: acpclient.SessionPolicy{
			Reuse:             true,
			PreferLoadSession: false,
		},
	}

	session, err := startWebChatSession(
		context.Background(),
		client,
		"team_leader",
		role,
		"sid-old",
		"D:/repo/demo",
		teamleader.MCPEnvConfig{},
		nil,
	)
	if err != nil {
		t.Fatalf("startWebChatSession() error = %v", err)
	}
	if string(session) != "sid-new" {
		t.Fatalf("session id = %q, want %q", string(session), "sid-new")
	}
	if len(client.loadReqs) != 0 {
		t.Fatalf("LoadSession calls = %d, want 0", len(client.loadReqs))
	}
	if len(client.newReqs) != 1 {
		t.Fatalf("NewSession calls = %d, want 1", len(client.newReqs))
	}
}
