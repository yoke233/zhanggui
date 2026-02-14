package ports

import "context"

// Tx is an opaque transaction handle for repositories/adapters.
// Infrastructure controls the concrete type (for example, *gorm.DB).
type Tx interface{}

// UnitOfWork defines a transaction boundary.
//
// This is intentionally callback-style: returning an error causes rollback,
// returning nil causes commit.
type UnitOfWork interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type txKey struct{}

// WithTxContext stores a transaction handle in context.
func WithTxContext(ctx context.Context, tx Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// TxFromContext reads a transaction handle from context.
func TxFromContext(ctx context.Context) Tx {
	return ctx.Value(txKey{})
}
