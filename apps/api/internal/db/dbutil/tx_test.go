package dbutil

// Deep Tx module unit tests: no database required. The borrowed-handle
// refusal and the borrow-or-begin decision are pinned against a stub
// pgx.Tx so they hold without a DSN.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// stubTx is a minimal pgx.Tx. Statement methods are never called in
// these tests; lifecycle calls are counted so the tests prove who would
// have ended the transaction.
type stubTx struct {
	commitCalls   int
	rollbackCalls int
	commitErr     error
	rollbackErr   error
}

func (s *stubTx) Begin(context.Context) (pgx.Tx, error) { return s, nil }
func (s *stubTx) Commit(context.Context) error {
	s.commitCalls++
	return s.commitErr
}
func (s *stubTx) Rollback(context.Context) error {
	s.rollbackCalls++
	return s.rollbackErr
}
func (s *stubTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("stubTx: no CopyFrom")
}
func (s *stubTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (s *stubTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (s *stubTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("stubTx: no Prepare")
}
func (s *stubTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("stubTx: no Exec")
}
func (s *stubTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("stubTx: no Query")
}
func (s *stubTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (s *stubTx) Conn() *pgx.Conn                                  { return nil }

var _ pgx.Tx = (*stubTx)(nil)

func TestBorrowedHandleRefusesLifecycle(t *testing.T) {
	// Even with no inner transaction, refusal must come from the
	// module, never by touching (and panicking on) the carrier.
	handle := &borrowedTx{}
	if err := handle.Commit(context.Background()); !errors.Is(err, ErrBorrowedTxLifecycle) {
		t.Fatalf("borrowed Commit = %v, want ErrBorrowedTxLifecycle", err)
	}
	if err := handle.Rollback(context.Background()); !errors.Is(err, ErrBorrowedTxLifecycle) {
		t.Fatalf("borrowed Rollback = %v, want ErrBorrowedTxLifecycle", err)
	}
	if !BorrowedTx(handle) {
		t.Fatal("BorrowedTx(borrowed) = false, want true")
	}
	if BorrowedTx(nil) {
		t.Fatal("BorrowedTx(nil) = true, want false")
	}
	raw := &stubTx{}
	if BorrowedTx(raw) {
		t.Fatal("BorrowedTx(raw) = true, want false")
	}
	// No double-wrap: borrowing an already-borrowed handle is identity.
	if borrow(handle) != pgx.Tx(handle) {
		t.Fatal("borrow double-wrapped an already-borrowed handle")
	}
}

func TestBeginBorrowsAmbientWithoutPool(t *testing.T) {
	raw := &stubTx{}
	ctx := ContextWithAmbientTx(context.Background(), raw)
	scope, err := Begin(ctx, nil, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("Begin on carrier with nil pool = %v, want nil", err)
	}
	if scope.Owned() {
		t.Fatal("borrowed scope Owned() = true, want false")
	}
	if scope.Queries() == nil {
		t.Fatal("borrowed scope Queries() is nil")
	}
	if !BorrowedTx(scope.Tx()) {
		t.Fatal("borrowed scope Tx() does not refuse lifecycle")
	}
	// Scope-level lifecycle is a safe no-op: the stager decides.
	if err := scope.Commit(ctx); err != nil {
		t.Fatalf("borrowed Scope.Commit = %v, want nil", err)
	}
	if err := scope.Rollback(ctx); err != nil {
		t.Fatalf("borrowed Scope.Rollback = %v, want nil", err)
	}
	if raw.commitCalls != 0 || raw.rollbackCalls != 0 {
		t.Fatalf("carrier touched: commits=%d rollbacks=%d, want 0/0", raw.commitCalls, raw.rollbackCalls)
	}
	// The handle itself refuses: bypassing the Scope cannot end the unit.
	if err := scope.Tx().Commit(ctx); !errors.Is(err, ErrBorrowedTxLifecycle) {
		t.Fatalf("borrowed handle Commit = %v, want refusal", err)
	}
	if err := scope.Tx().Rollback(ctx); !errors.Is(err, ErrBorrowedTxLifecycle) {
		t.Fatalf("borrowed handle Rollback = %v, want refusal", err)
	}
	if raw.commitCalls != 0 || raw.rollbackCalls != 0 {
		t.Fatalf("refused lifecycle touched carrier: commits=%d rollbacks=%d", raw.commitCalls, raw.rollbackCalls)
	}
}

func TestBeginOwnedNeverBorrows(t *testing.T) {
	raw := &stubTx{}
	ctx := ContextWithAmbientTx(context.Background(), raw)
	if _, err := BeginOwned(ctx, nil, pgx.TxOptions{}); err == nil {
		t.Fatal("BeginOwned on carrier with nil pool succeeded, want database error (no borrow)")
	}
}

func TestDoRejectsNilFn(t *testing.T) {
	if err := Do(context.Background(), nil, pgx.TxOptions{}, nil); err == nil {
		t.Fatal("Do with nil fn succeeded, want error")
	}
}

func TestDoRequiresPoolWithoutCarrier(t *testing.T) {
	called := false
	err := Do(context.Background(), nil, pgx.TxOptions{}, func(*Scope) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("Do without carrier or pool succeeded, want error")
	}
	if called {
		t.Fatal("Do ran fn without a transaction, want no run")
	}
}

func TestBorrowedQueriesBindsCarrier(t *testing.T) {
	raw := &stubTx{}
	ctx := ContextWithAmbientTx(context.Background(), raw)
	if got := BorrowedQueries(ctx, nil); got == nil {
		t.Fatal("BorrowedQueries on carrier with nil base is nil, want carrier-bound queries")
	}
	// Without a carrier the base passes through untouched (identity).
	sentinel := BorrowedQueries(context.Background(), nil)
	if sentinel != nil {
		t.Fatal("BorrowedQueries without carrier or base is non-nil, want nil passthrough")
	}
}
