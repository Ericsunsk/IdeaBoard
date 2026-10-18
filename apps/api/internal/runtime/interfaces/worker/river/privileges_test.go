package river

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	dbprivileges "ideaboard/api/internal/platform/database/privileges"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
)

func TestRiverRuntimeRoleHasOperationalPrivilegesWithoutMigrationOwnership(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	if err := MigrateRiver(ctx, pool); err != nil {
		t.Fatalf("MigrateRiver() error = %v", err)
	}
	runtimeRole := fmt.Sprintf("river_runtime_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	if _, err := pool.Exec(ctx, `CREATE ROLE `+roleSQL+` NOLOGIN`); err != nil {
		t.Fatalf("create runtime role error = %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP OWNED BY `+roleSQL)
		_, _ = pool.Exec(context.Background(), `DROP ROLE IF EXISTS `+roleSQL)
	})
	policies := []dbprivileges.Policy{dbprivileges.RuntimePrivilegePolicy(), RuntimePrivilegePolicy()}
	if err := dbprivileges.Configure(ctx, pool, runtimeRole, policies...); err != nil {
		t.Fatalf("configure runtime privilege policies error = %v", err)
	}
	if err := dbprivileges.Verify(ctx, pool, policies...); err == nil {
		t.Fatal("schema owner was accepted as the River runtime role")
	}

	runtimeConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("open runtime connection error = %v", err)
	}
	defer runtimeConn.Release()
	if _, err := runtimeConn.Exec(ctx, `SET ROLE `+roleSQL); err != nil {
		t.Fatalf("SET ROLE error = %v", err)
	}
	if err := dbprivileges.Verify(ctx, runtimeConn, policies...); err != nil {
		t.Fatalf("verify runtime privilege policies error = %v", err)
	}
}
