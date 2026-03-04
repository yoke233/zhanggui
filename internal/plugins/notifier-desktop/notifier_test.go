package notifierdesktop

import (
	"context"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestDesktopNotifier_NameInitClose(t *testing.T) {
	notifier := New()

	if got := notifier.Name(); got != "desktop" {
		t.Fatalf("Name() = %q, want %q", got, "desktop")
	}
	if err := notifier.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := notifier.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestDesktopNotifier_Notify_NoPanic(t *testing.T) {
	var (
		called    bool
		gotName   string
		gotArgs   []string
		notifyErr error
	)

	notifier := newWithRunner(func(_ context.Context, name string, args ...string) error {
		called = true
		gotName = name
		gotArgs = append([]string(nil), args...)
		return notifyErr
	}, "darwin", false)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Notify() panic = %v", r)
		}
	}()

	err := notifier.Notify(context.Background(), core.Notification{
		Title: "Build complete",
		Body:  "All checks passed",
	})
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if !called {
		t.Fatalf("expected runner to be called")
	}
	if gotName != "osascript" {
		t.Fatalf("runner name = %q, want %q", gotName, "osascript")
	}
	if len(gotArgs) != 2 || gotArgs[0] != "-e" {
		t.Fatalf("runner args = %v, want [-e <script>]", gotArgs)
	}
	if !strings.Contains(gotArgs[1], "display notification") {
		t.Fatalf("script = %q, want display notification", gotArgs[1])
	}
}

func TestDesktopNotifier_Notify_CISkipsSystemCall(t *testing.T) {
	called := false
	notifier := newWithRunner(func(context.Context, string, ...string) error {
		called = true
		return nil
	}, "windows", true)

	err := notifier.Notify(context.Background(), core.Notification{
		Title: "Build complete",
		Body:  "All checks passed",
	})
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if called {
		t.Fatalf("expected runner not to be called in CI mode")
	}
}

func TestDesktopNotifier_Notify_WindowsCommand(t *testing.T) {
	var (
		gotName string
		gotArgs []string
	)
	notifier := newWithRunner(func(_ context.Context, name string, args ...string) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil
	}, "windows", false)

	if err := notifier.Notify(context.Background(), core.Notification{
		Title: "Run",
		Body:  "Deploy done",
	}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	if gotName != "powershell" {
		t.Fatalf("runner name = %q, want %q", gotName, "powershell")
	}
	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "-Command") {
		t.Fatalf("runner args missing -Command: %v", gotArgs)
	}
	if !strings.Contains(joined, "Wscript.Shell") {
		t.Fatalf("runner args missing Wscript.Shell script: %v", gotArgs)
	}
}

func TestNew_RespectsCIEnvironment(t *testing.T) {
	t.Setenv("CI", "true")
	if notifier := New(); !notifier.ci {
		t.Fatalf("expected notifier.ci to be true when CI=true")
	}

	t.Setenv("CI", "false")
	if notifier := New(); notifier.ci {
		t.Fatalf("expected notifier.ci to be false when CI=false")
	}
}
