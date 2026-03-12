package acpclient

import (
	"context"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
)

func TestSetConfigOptionCallsTransport(t *testing.T) {
	requireACPClientIntegration(t)

	h := &recordingHandler{}
	client, err := New(testLaunchConfig(t), h)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer client.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Initialize(ctx, ClientCapabilities{FSRead: true}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	_, err = client.SetConfigOption(ctx, acpproto.SetSessionConfigOptionRequest{
		SessionId: "fake-session-1",
		ConfigId:  acpproto.SessionConfigId("model"),
		Value:     acpproto.SessionConfigValueId("model-2"),
	})
	if err == nil {
		t.Fatal("expected error from fake agent for unsupported method")
	}
}
