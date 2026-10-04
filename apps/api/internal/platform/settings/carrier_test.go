package settings

// Control audit atomicity, step 3: the settings writes join the ambient
// guard transaction instead of committing in their own.
//
// What is proven here, against the isolated fixture database:
//   - SetMany on a carrier context lands inside the carrier: the config
//     row is readable from the still-open carrier and invisible on the
//     pool, then durable after the carrier commits.
//   - A carrier rollback leaves no config row behind (the step-2 gap:
//     the settings commit used to survive on its own).
//   - With no carrier the owned-transaction path runs unchanged (the
//     existing settings suite is that proof; it is not duplicated
//     here).

import (
	"context"
	"testing"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/testdb"
)

func TestSettingsSetManyJoinsAmbientCarrierTx(t *testing.T) {
	db := testdb.Open(t)
	repo := NewSystemConfigRepository(db)
	ctx := context.Background()
	const key = "carrier test key"

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	if err := repo.SetMany(carrier, []SystemConfigValue{{Key: key, Value: "carrier-value"}}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("SetMany() on carrier error = %v", err)
	}
	// Same-transaction visibility ...
	var inside int
	if err := tx.QueryRow(carrier, `SELECT count(*) FROM system_configs WHERE config_key = $1`, key).Scan(&inside); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("count configs inside carrier: %v", err)
	}
	if inside != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("config rows visible inside carrier = %d, want 1", inside)
	}
	// ... invisible outside before the carrier commits.
	if _, err := repo.Get(ctx, key); err == nil {
		_ = tx.Rollback(ctx)
		t.Fatal("Get() saw the config before the carrier committed, want not found")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	value, err := repo.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() after carrier commit error = %v", err)
	}
	if value != "carrier-value" {
		t.Fatalf("config value after carrier commit = %q, want %q", value, "carrier-value")
	}
}

func TestSettingsSetManyRollsBackWithAmbientCarrier(t *testing.T) {
	db := testdb.Open(t)
	repo := NewSystemConfigRepository(db)
	ctx := context.Background()
	const key = "carrier rollback key"

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	if err := repo.SetMany(carrier, []SystemConfigValue{{Key: key, Value: "doomed-value"}}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("SetMany() on carrier error = %v", err)
	}
	// The business fails after the settings write: the guard rolls the
	// whole carrier back, settings write included.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	if _, err := repo.Get(ctx, key); err == nil {
		t.Fatal("Get() saw the config after carrier rollback, want not found")
	}
}
