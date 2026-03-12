package acpclient

import (
	"os"
	"testing"
)

const runACPClientIntegrationEnv = "AI_WORKFLOW_RUN_ACPCLIENT_INTEGRATION"

func requireACPClientIntegration(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping acpclient integration test in short mode")
	}
	if os.Getenv(runACPClientIntegrationEnv) != "1" {
		t.Skip("skipping acpclient integration test; set AI_WORKFLOW_RUN_ACPCLIENT_INTEGRATION=1 to enable")
	}
}
