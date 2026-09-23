package testdb

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var roleCounter uint64

func CreateRestrictedRole(t testing.TB, db *pgxpool.Pool, prefix string) string {
	t.Helper()

	roleName := nextRoleName(prefix)
	roleSQL := pgx.Identifier{roleName}.Sanitize()
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- roleSQL is pg identifier sanitized above
	if _, err := db.Exec(t.Context(), `CREATE ROLE `+roleSQL+` NOLOGIN`); err != nil {
		t.Fatalf("create restricted PostgreSQL test role: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- roleSQL is pg identifier sanitized above
		if _, err := db.Exec(ctx, `DROP OWNED BY `+roleSQL); err != nil {
			t.Errorf("drop restricted PostgreSQL test role ownership: %v", err)
		}
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- roleSQL is pg identifier sanitized above
		if _, err := db.Exec(ctx, `DROP ROLE IF EXISTS `+roleSQL); err != nil {
			t.Errorf("drop restricted PostgreSQL test role: %v", err)
		}
	})
	return roleName
}

func OpenPoolAsRole(t testing.TB, base *pgxpool.Pool, roleName string) *pgxpool.Pool {
	t.Helper()

	roleSQL := pgx.Identifier{roleName}.Sanitize()
	cfg := base.Config().Copy()
	baseAfterConnect := cfg.AfterConnect
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		if baseAfterConnect != nil {
			if err := baseAfterConnect(ctx, conn); err != nil {
				return err
			}
		}
		_, err := conn.Exec(ctx, `SET ROLE `+roleSQL)
		return err
	}
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("open PostgreSQL test pool as restricted role: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(t.Context()); err != nil {
		t.Fatalf("ping PostgreSQL test pool as restricted role: %v", err)
	}
	return pool
}

func nextRoleName(prefix string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "_")
	if prefix == "" {
		prefix = "restricted"
	}
	if len(prefix) > 24 {
		prefix = prefix[:24]
	}
	counter := atomic.AddUint64(&roleCounter, 1)
	return fmt.Sprintf("%s_%x_%x_%x", prefix, os.Getpid(), time.Now().UnixNano(), counter)
}
