package dbutil

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type connectionContextKey struct{}

// ContextWithConnection carries an owner-managed PostgreSQL session across
// repository boundaries, including operations protected by session advisory
// locks. Participants borrow it for sequential work in the callback only: they
// must not release it, use it concurrently, or retain it beyond that scope.
// Unlike an ambient transaction, this does not keep a transaction open across
// external I/O; participants may open and complete short transactions on it.
func ContextWithConnection(ctx context.Context, connection *pgxpool.Conn) context.Context {
	return context.WithValue(ctx, connectionContextKey{}, connection)
}

// ConnectionFromContext returns the borrowed session connection, if any.
func ConnectionFromContext(ctx context.Context) (*pgxpool.Conn, bool) {
	if ctx == nil {
		return nil, false
	}
	connection, ok := ctx.Value(connectionContextKey{}).(*pgxpool.Conn)
	return connection, ok && connection != nil
}
