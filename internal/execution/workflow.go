package execution

import (
	"context"
	"strings"

	"github.com/yoke233/zhanggui/internal/gateway"
	"github.com/yoke233/zhanggui/internal/planning"
	"github.com/yoke233/zhanggui/internal/verify"
)

type Workflow interface {
	Name() string
	Run(ctx Context) (Result, error)
}

type Context struct {
	Ctx context.Context
	GW  *gateway.Gateway

	TaskID string
	RunID  string
	Rev    string

	DeliveryPlan *planning.DeliveryPlan
}

type Result struct {
	Issues []verify.Issue
}

func (r Result) HasBlocker() bool {
	for _, it := range r.Issues {
		if strings.EqualFold(strings.TrimSpace(it.Severity), "blocker") {
			return true
		}
	}
	return false
}
