package workspacemedia

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"
)

// openMediaConcurrentEnv mirrors openMediaTestEnv but leaves the pool
// unbounded (no WithMaxConns(1)) so two CompleteSession transactions can
// hold concurrent connections and truly contend on the session row lock.
func openMediaConcurrentEnv(t *testing.T) *mediaTestEnv {
	t.Helper()
	pool := testdb.OpenIsolatedSchema(t, "media_test", mediaTestDDL)
	ctx := context.Background()
	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (display_name, storage_uid) VALUES ('media concurrent user', 'M00002') RETURNING id`,
	).Scan(&userID); err != nil {
		t.Fatalf("seed test user: %v", err)
	}
	ws := "ws_mediatest_concurrent"
	if _, err := pool.Exec(ctx,
		`INSERT INTO workspaces (public_id, owner_user_id, name) VALUES ($1, $2, 'media-concurrent')`,
		ws, userID,
	); err != nil {
		t.Fatalf("seed workspace %s: %v", ws, err)
	}
	return &mediaTestEnv{pool: pool, svc: NewService(pool), wsA: ws, wsB: ws, owner: userID}
}

// TestConcurrentCompleteSingleWinner proves the single-tx CompleteSession
// recovery serializes real concurrent completions: the session row starts
// pending so both transactions contend on the MarkTransferring row lock;
// exactly one commits the asset + completed flip while the other observes
// the terminal row and reports ErrStateConflict. Only one durable asset
// row lands.
func TestConcurrentCompleteSingleWinner(t *testing.T) {
	env := openMediaConcurrentEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Kept pending on purpose: no GetSession warm-up, so both Completes
	// serialize on the MarkTransferring UPDATE inside the tx. Pre-warming
	// to transferring would let both reach the asset INSERT and collide
	// on the storage_key unique index instead of the state gate.
	session := mustCreateDBSession(t, ctx, env.svc, env.wsA, env.owner)

	start := make(chan struct{})
	type completeResult struct {
		completed *Session
		asset     *Asset
		err       error
	}
	results := make([]completeResult, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			completed, asset, err := env.svc.CompleteSession(ctx, env.wsA, session.ID, dbCompleteFacts(), CreateAssetParams{})
			results[idx] = completeResult{completed: completed, asset: asset, err: err}
		}(i)
	}
	close(start)
	wg.Wait()

	var winner *completeResult
	conflicts := 0
	for i := range results {
		r := &results[i]
		if r.err == nil {
			if winner != nil {
				t.Fatalf("both concurrent Completes succeeded, want single winner")
			}
			winner = r
			continue
		}
		if errors.Is(r.err, ErrStateConflict) {
			conflicts++
			continue
		}
		t.Fatalf("loser err = %v, want ErrStateConflict", r.err)
	}
	if winner == nil {
		t.Fatalf("no Complete succeeded: %+v", results)
	}
	if conflicts != 1 {
		t.Fatalf("conflicts = %d, want 1", conflicts)
	}
	if winner.completed.Status != StatusCompleted || winner.asset == nil || winner.completed.AssetID != winner.asset.ID {
		t.Fatalf("winner = %+v asset %+v, want completed with asset link", winner.completed, winner.asset)
	}

	got, err := env.svc.GetSession(ctx, env.wsA, session.ID)
	if err != nil {
		t.Fatalf("final GetSession: %v", err)
	}
	if got.Status != StatusCompleted {
		t.Fatalf("final status = %q, want completed", got.Status)
	}
	var assetCount int
	if err := env.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM media_assets WHERE upload_session_id = (SELECT internal_id FROM upload_sessions WHERE public_id = $1)`,
		session.ID,
	).Scan(&assetCount); err != nil {
		t.Fatalf("count assets: %v", err)
	}
	if assetCount != 1 {
		t.Fatalf("asset rows = %d, want exactly 1", assetCount)
	}
}
