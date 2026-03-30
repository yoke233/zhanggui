package orchestrateapp

import (
	"context"

	"github.com/yoke233/zhanggui/internal/application/planning"
	"github.com/yoke233/zhanggui/internal/application/threadapp"
	"github.com/yoke233/zhanggui/internal/application/workitemapp"
	"github.com/yoke233/zhanggui/internal/core"
)

type WorkItemCreator interface {
	CreateWorkItem(ctx context.Context, input workitemapp.CreateWorkItemInput) (*core.WorkItem, error)
}

type Store interface {
	core.WorkItemStore
	core.ActionStore
	core.RunStore
}

type Planner interface {
	Generate(ctx context.Context, input planning.GenerateInput) (*planning.GeneratedDAG, error)
}

type ThreadCoordinator interface {
	CreateThread(ctx context.Context, input threadapp.CreateThreadInput) (*threadapp.CreateThreadResult, error)
	DeleteThread(ctx context.Context, threadID int64) error
	LinkThreadWorkItem(ctx context.Context, input threadapp.LinkThreadWorkItemInput) (*core.ThreadWorkItemLink, error)
	FindActiveThreadByWorkItem(ctx context.Context, workItemID int64) (*core.Thread, error)
	EnsureHumanParticipants(ctx context.Context, threadID int64, userIDs []string) ([]*core.ThreadMember, error)
}

type Config struct {
	Store           Store
	WorkItemCreator WorkItemCreator
	Planner         Planner
	Threads         ThreadCoordinator
}
