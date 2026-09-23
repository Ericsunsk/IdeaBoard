package database

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestIdleConnectionLimiterEnforcesCapAcrossConcurrentReleases(t *testing.T) {
	const limit = 5
	limiter := newIdleConnectionLimiter(limit)
	connections := make([]*pgx.Conn, 100)
	for i := range connections {
		connections[i] = &pgx.Conn{}
	}

	var accepted int
	var acceptedMu sync.Mutex
	var wait sync.WaitGroup
	for _, conn := range connections {
		wait.Add(1)
		go func(conn *pgx.Conn) {
			defer wait.Done()
			if limiter.afterRelease(conn) {
				acceptedMu.Lock()
				accepted++
				acceptedMu.Unlock()
			}
		}(conn)
	}
	wait.Wait()

	if accepted != limit {
		t.Fatalf("accepted idle connections = %d, want %d", accepted, limit)
	}
	if got := len(limiter.conns); got != limit {
		t.Fatalf("tracked idle connections = %d, want %d", got, limit)
	}

	var conn *pgx.Conn
	for idleConn := range limiter.conns {
		conn = idleConn
		break
	}
	if ok, err := limiter.prepareConn(context.Background(), conn); !ok || err != nil {
		t.Fatalf("prepare acquired connection = %v, %v, want true, nil", ok, err)
	}
	replacement := &pgx.Conn{}
	if !limiter.afterRelease(replacement) {
		t.Fatal("released connection was rejected after an idle connection was acquired")
	}
	limiter.beforeClose(replacement)
	if got := len(limiter.conns); got != limit-1 {
		t.Fatalf("tracked idle connections after close = %d, want %d", got, limit-1)
	}
}

func TestIdleConnectionLimiterRejectsAllConnectionsAtZero(t *testing.T) {
	limiter := newIdleConnectionLimiter(0)
	if limiter.afterRelease(&pgx.Conn{}) {
		t.Fatal("zero idle limit retained a connection")
	}
}
