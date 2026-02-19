package outbox

import (
	"context"
	"testing"
	"time"
)

func TestCodexRunner_ParseJSONResult(t *testing.T) {
	raw := `{"status":"pass","summary":"ok","result_code":"none","commit":"git:abc"}`

	got, err := parseCodexResult(raw)
	if err != nil {
		t.Fatalf("parseCodexResult() error = %v", err)
	}
	if got.Status != "pass" || got.Commit != "git:abc" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestWithExecutorTimeout_UsesExecutorSeconds(t *testing.T) {
	ctx, cancel := withExecutorTimeout(context.Background(), 1)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("withExecutorTimeout() deadline missing")
	}

	remaining := time.Until(deadline)
	if remaining <= 0 {
		t.Fatalf("deadline already elapsed: %s", remaining)
	}
	if remaining > 2*time.Second {
		t.Fatalf("deadline too far in future: %s", remaining)
	}
}
