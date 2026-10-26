package workspacefacts

import (
	"context"
	"errors"
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// stubDBTX fakes the sqlc DBTX seam so the shared workspace resolver core
// is pinned without a live database: it captures the looked-up public id
// and replays one canned row outcome.
type stubDBTX struct {
	gotPublicID string
	internalID  int64
	scanErr     error
}

func (s *stubDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("workspacefacts stub: Exec not implemented")
}

func (s *stubDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("workspacefacts stub: Query not implemented")
}

func (s *stubDBTX) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	if len(args) > 0 {
		s.gotPublicID, _ = args[0].(string)
	}
	return stubRow{internalID: s.internalID, err: s.scanErr}
}

type stubRow struct {
	internalID int64
	err        error
}

func (r stubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) > 0 {
		if id, ok := dest[0].(*int64); ok {
			*id = r.internalID
		}
	}
	return nil
}

func TestResolveInternalIDTrimsAndMaps(t *testing.T) {
	stub := &stubDBTX{internalID: 42}
	q := dbsqlc.New(stub)
	got, err := ResolveInternalID(context.Background(), q, "  ws_abc123  ")
	if err != nil {
		t.Fatalf("ResolveInternalID error = %v", err)
	}
	if got != 42 {
		t.Fatalf("ResolveInternalID = %d, want 42", got)
	}
	if stub.gotPublicID != "ws_abc123" {
		t.Fatalf("looked-up public id = %q, want trimmed %q", stub.gotPublicID, "ws_abc123")
	}
}

func TestResolveInternalIDPassesQueryErrorThrough(t *testing.T) {
	stub := &stubDBTX{scanErr: pgx.ErrNoRows}
	q := dbsqlc.New(stub)
	if _, err := ResolveInternalID(context.Background(), q, "ws_missing"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("ResolveInternalID error = %v, want pgx.ErrNoRows unchanged (callers map it to their own sentinel)", err)
	}
}
