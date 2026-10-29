// Package testscope provides shared settlement transaction fakes with rollback undos.
package testscope

import (
	"context"
	"strings"
	"sync"

	"ideaboard/api/internal/db/dbutil"

	"github.com/jackc/pgx/v5"
)

// Tx is a no-op settlement transaction: Commit/Rollback only record their
// outcome (plus deferred cleanups on rollback). All other pgx.Tx legs are
// never called by the in-memory fakes below.
type Tx struct {
	pgx.Tx
	Committed  bool
	RolledBack bool
	onRollback []func()
}

// BeginScope stages a fake settlement transaction as an owned scope.
func BeginScope(ctx context.Context) (*dbutil.Scope, *Tx) {
	tx := &Tx{}
	return dbutil.ScopeForTest(ctx, tx, true), tx
}

// Commit records the commit; the in-memory writes already applied.
func (t *Tx) Commit(context.Context) error {
	t.Committed = true
	return nil
}

// Rollback records the rollback and runs every registered undo.
func (t *Tx) Rollback(context.Context) error {
	t.RolledBack = true
	for _, fn := range t.onRollback {
		fn()
	}
	t.onRollback = nil
	return nil
}

// ShimTx wraps a real settlement transaction so the database-outside fake
// legs can register rollback undos on it. Task rows need no undo: the real
// rollback discards them.
type ShimTx struct {
	pgx.Tx
	onRollback []func()
}

// WrapShim wraps a real transaction with the shim flavor.
func WrapShim(tx pgx.Tx) *ShimTx {
	return &ShimTx{Tx: tx}
}

// Rollback runs the registered in-memory undos, then delegates to the
// real transaction.
func (t *ShimTx) Rollback(ctx context.Context) error {
	for _, fn := range t.onRollback {
		fn()
	}
	t.onRollback = nil
	return t.Tx.Rollback(ctx)
}

// OnRollback registers an in-memory cleanup on either fake transaction
// flavor. It is a no-op for foreign transactions (e.g. ambient carriers),
// which own their own commit/rollback.
func OnRollback(tx pgx.Tx, fn func()) {
	switch t := tx.(type) {
	case *Tx:
		t.onRollback = append(t.onRollback, fn)
	case *ShimTx:
		t.onRollback = append(t.onRollback, fn)
	}
}

// ClaimScopeKey fences one durable submit claim by workspace, operation,
// and idempotency key.
func ClaimScopeKey(workspaceID, operation, key string) string {
	return strings.TrimSpace(workspaceID) + "\x00" + strings.TrimSpace(operation) + "\x00" + strings.TrimSpace(key)
}

// Claims fences durable submit claims in memory with rollback undos. The
// first claim wins and replays observe the winner; the fence undo is
// registered on the transaction so a rejected submit leaves no claim
// behind to poison its key.
type Claims[T any] struct {
	mu    sync.Mutex
	items map[string]T
}

// Claim registers one durable submit claim under key.
func (c *Claims[T]) Claim(tx pgx.Tx, key string, claim T) (*T, bool, error) {
	c.mu.Lock()
	if c.items == nil {
		c.items = make(map[string]T)
	}
	if existing, ok := c.items[key]; ok {
		dup := existing
		c.mu.Unlock()
		return &dup, false, nil
	}
	c.items[key] = claim
	c.mu.Unlock()
	OnRollback(tx, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.items, key)
	})
	return nil, true, nil
}

// Count reports how many claims the fence currently holds.
func (c *Claims[T]) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}
