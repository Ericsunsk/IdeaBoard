package database

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
)

// idleConnectionLimiter preserves the configured retained-idle ceiling because pgxpool exposes no MaxIdleConns setting.
type idleConnectionLimiter struct {
	mu    sync.Mutex
	limit int
	conns map[*pgx.Conn]struct{}
}

func newIdleConnectionLimiter(limit int) *idleConnectionLimiter {
	return &idleConnectionLimiter{limit: limit, conns: make(map[*pgx.Conn]struct{})}
}

func (l *idleConnectionLimiter) prepareConn(_ context.Context, conn *pgx.Conn) (bool, error) {
	l.mu.Lock()
	delete(l.conns, conn)
	l.mu.Unlock()
	return true, nil
}

func (l *idleConnectionLimiter) afterRelease(conn *pgx.Conn) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.limit <= 0 || len(l.conns) >= l.limit {
		return false
	}
	l.conns[conn] = struct{}{}
	return true
}

func (l *idleConnectionLimiter) beforeClose(conn *pgx.Conn) {
	l.mu.Lock()
	delete(l.conns, conn)
	l.mu.Unlock()
}
