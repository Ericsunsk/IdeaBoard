package settings

// Rate limit cache atomicity: the in-memory entry must never hold data
// the database rolled back. UpdateRateLimit joins the ambient
// (guard-owned) transaction when one is present, so the cache refresh
// must be post-commit semantics: invalidate on a borrowed transaction,
// write through only after an owned commit.

import (
	"context"
	"sync"
	"testing"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/testdb"
)

func TestUpdateRateLimitRollbackLeavesNoDirtyCache(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))

	old := RateLimitSetting{WindowSeconds: 120, MaxRequests: 60}
	if _, err := service.UpdateRateLimit(ctx, old); err != nil {
		t.Fatalf("seed rate limit failed: %v", err)
	}
	if _, err := service.GetRateLimit(ctx); err != nil {
		t.Fatalf("prime rate limit cache failed: %v", err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	doomed := RateLimitSetting{WindowSeconds: 300, MaxRequests: 5}
	if _, err := service.UpdateRateLimit(carrier, doomed); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("UpdateRateLimit() on carrier error = %v", err)
	}
	// The guard fails after the settings write and rolls everything back.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}

	got, err := service.GetRateLimit(ctx)
	if err != nil {
		t.Fatalf("GetRateLimit() after rollback error = %v", err)
	}
	if got == doomed {
		t.Fatalf("GetRateLimit() = %+v, serves rolled-back data", got)
	}
	if got != old {
		t.Fatalf("GetRateLimit() = %+v, want pre-rollback %+v", got, old)
	}
}

func TestUpdateRateLimitCommitIsVisibleAfterCarrierCommits(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))

	old := RateLimitSetting{WindowSeconds: 120, MaxRequests: 60}
	if _, err := service.UpdateRateLimit(ctx, old); err != nil {
		t.Fatalf("seed rate limit failed: %v", err)
	}
	if _, err := service.GetRateLimit(ctx); err != nil {
		t.Fatalf("prime rate limit cache failed: %v", err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	next := RateLimitSetting{WindowSeconds: 300, MaxRequests: 5}
	if _, err := service.UpdateRateLimit(carrier, next); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("UpdateRateLimit() on carrier error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}

	got, err := service.GetRateLimit(ctx)
	if err != nil {
		t.Fatalf("GetRateLimit() after commit error = %v", err)
	}
	if got != next {
		t.Fatalf("GetRateLimit() = %+v, want committed %+v", got, next)
	}
}

func TestRateLimitCacheConcurrentReadWrite(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))

	seed := RateLimitSetting{WindowSeconds: 60, MaxRequests: 60}
	if _, err := service.UpdateRateLimit(ctx, seed); err != nil {
		t.Fatalf("seed rate limit failed: %v", err)
	}

	var wg sync.WaitGroup
	for reader := 0; reader < 8; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				got, err := service.GetRateLimit(ctx)
				if err != nil {
					t.Errorf("GetRateLimit() error = %v", err)
					return
				}
				if got.WindowSeconds < 1 || got.WindowSeconds > maxRateLimitWindow ||
					got.MaxRequests < 1 || got.MaxRequests > maxRateLimitRequests {
					t.Errorf("GetRateLimit() = %+v, out of bounds", got)
					return
				}
			}
		}()
	}
	for writer := 0; writer < 2; writer++ {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				setting := RateLimitSetting{WindowSeconds: 60 + writer*10 + i%5, MaxRequests: 60 + i}
				if _, err := service.UpdateRateLimit(ctx, setting); err != nil {
					t.Errorf("UpdateRateLimit(%+v) error = %v", setting, err)
					return
				}
			}
		}(writer)
	}
	wg.Wait()
}
