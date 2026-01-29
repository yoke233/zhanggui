package execution

import (
	"testing"

	"github.com/yoke233/zhanggui/internal/verify"
)

func TestResult_HasBlocker(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if (Result{}).HasBlocker() {
			t.Fatalf("expected HasBlocker=false")
		}
	})

	t.Run("warn_only", func(t *testing.T) {
		if (Result{Issues: []verify.Issue{{Severity: "warn"}}}).HasBlocker() {
			t.Fatalf("expected HasBlocker=false")
		}
	})

	t.Run("blocker_case_insensitive", func(t *testing.T) {
		if !(Result{Issues: []verify.Issue{{Severity: " Blocker "}}}).HasBlocker() {
			t.Fatalf("expected HasBlocker=true")
		}
	})
}
