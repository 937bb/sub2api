package ent

import "context"

// WithoutTx returns a copy of ctx with any attached transaction removed.
func WithoutTx(ctx context.Context) context.Context {
	if TxFromContext(ctx) == nil {
		return ctx
	}
	return context.WithValue(ctx, txCtxKey{}, (*Tx)(nil))
}
