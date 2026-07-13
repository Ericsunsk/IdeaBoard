package worker

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
)

func TestRiverRuntimeRoleHasOperationalPrivilegesWithoutMigrationOwnership(t *testing.T) {
	ctx := context.Background()
	db, pool := testdb.OpenWithPool(t)
	if err := MigrateRiver(ctx, pool); err != nil {
		t.Fatalf("MigrateRiver() error = %v", err)
	}
	runtimeRole := fmt.Sprintf("river_runtime_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	if _, err := db.ExecContext(ctx, `CREATE ROLE `+roleSQL+` NOLOGIN`); err != nil {
		t.Fatalf("create runtime role error = %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DROP OWNED BY `+roleSQL)
		_, _ = db.ExecContext(context.Background(), `DROP ROLE IF EXISTS `+roleSQL)
	})
	if err := database.ConfigureRuntimePrivileges(ctx, db, runtimeRole); err != nil {
		t.Fatalf("database ConfigureRuntimePrivileges() error = %v", err)
	}
	if err := ConfigureRiverRuntimePrivileges(ctx, db, runtimeRole); err != nil {
		t.Fatalf("ConfigureRiverRuntimePrivileges() error = %v", err)
	}
	if err := VerifyRiverRuntimePrivileges(ctx, db); err == nil {
		t.Fatal("schema owner was accepted as the River runtime role")
	}

	runtimeConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open runtime connection error = %v", err)
	}
	defer runtimeConn.Close()
	if _, err := runtimeConn.ExecContext(ctx, `SET ROLE `+roleSQL); err != nil {
		t.Fatalf("SET ROLE error = %v", err)
	}
	if err := VerifyRiverRuntimePrivileges(ctx, runtimeConn); err != nil {
		t.Fatalf("VerifyRiverRuntimePrivileges() error = %v", err)
	}
}
