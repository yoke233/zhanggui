package execution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoke233/zhanggui/internal/gateway"
	"github.com/yoke233/zhanggui/internal/planning"
)

func TestDemo04_Run_RespectsMaxParallel_Acceptance(t *testing.T) {
	cases := []struct {
		name     string
		planYAML string
		wantMax  int
	}{
		{
			name: "max_parallel_1",
			planYAML: `
teams:
  - team_id: team_a
roles:
  - role: writer
    count: 1
  - role: designer
    count: 1
budgets:
  max_parallel: 1
  per_team_parallel_cap:
    team_a: 1
  per_role_parallel_cap:
    writer: 1
    designer: 1
`,
			wantMax: 1,
		},
		{
			name: "max_parallel_2",
			planYAML: `
teams:
  - team_id: team_a
roles:
  - role: writer
    count: 1
  - role: designer
    count: 1
budgets:
  max_parallel: 2
  per_team_parallel_cap:
    team_a: 2
  per_role_parallel_cap:
    writer: 2
    designer: 1
`,
			wantMax: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			_ = os.MkdirAll(filepath.Join(root, "logs"), 0o755)
			_ = os.MkdirAll(filepath.Join(root, "revs", "r1"), 0o755)

			aud, err := gateway.NewAuditor(filepath.Join(root, "logs", "tool_audit.jsonl"))
			if err != nil {
				t.Fatalf("NewAuditor: %v", err)
			}
			t.Cleanup(func() { _ = aud.Close() })

			gw, err := gateway.New(root, gateway.Actor{AgentID: "taskctl", Role: "system"}, gateway.Linkage{TaskID: "t1", RunID: "r1", Rev: "r1"}, gateway.Policy{
				AllowedWritePrefixes: []string{"revs/", "logs/"},
			}, aud)
			if err != nil {
				t.Fatalf("gateway.New: %v", err)
			}

			p, err := planning.ParseDeliveryPlanYAML([]byte(tc.planYAML))
			if err != nil {
				t.Fatalf("ParseDeliveryPlanYAML: %v", err)
			}
			if err := p.Validate(); err != nil {
				t.Fatalf("plan.Validate: %v", err)
			}

			w := NewDemo04Workflow()
			_, err = w.Run(Context{Ctx: context.Background(), GW: gw, TaskID: "t1", RunID: "r1", Rev: "r1", DeliveryPlan: &p})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}

			statsPath := filepath.Join(root, "revs", "r1", "run_stats.json")
			b, err := os.ReadFile(statsPath)
			if err != nil {
				t.Fatalf("read run_stats.json: %v", err)
			}
			var stats RunStats
			if err := json.Unmarshal(b, &stats); err != nil {
				t.Fatalf("unmarshal run_stats.json: %v", err)
			}
			if stats.MaxInFlight != tc.wantMax {
				t.Fatalf("max_in_flight=%d want=%d (caps=%+v)", stats.MaxInFlight, tc.wantMax, stats.Caps)
			}
		})
	}
}
