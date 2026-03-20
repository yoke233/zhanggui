package proposalapp

import (
	"context"

	"github.com/yoke233/zhanggui/internal/core"
)

type Store interface {
	core.ProposalStore
	core.ProjectStore
	core.ThreadStore
	core.WorkItemStore
	core.InitiativeStore
}

type Tx interface {
	InTx(ctx context.Context, fn func(ctx context.Context, store Store) error) error
}

type Config struct {
	Store Store
	Tx    Tx
	Bus   core.EventBus
}
