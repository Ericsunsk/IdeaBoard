package idempotency

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/platform/apperror"
)

func TestParseRequiresKey(t *testing.T) {
	if _, err := Parse(""); err == nil || apperror.CodeOf(err) != CodeRequired {
		t.Fatalf("Parse empty = %v, want missing", err)
	}
	if _, err := Parse("   "); err == nil {
		t.Fatal("Parse blank should fail")
	}
	if key, err := Parse("  key-1  "); err != nil || key != "key-1" {
		t.Fatalf("Parse trim = %q, %v", key, err)
	}
}

func TestParseRejectsOversizedKey(t *testing.T) {
	oversized := make([]byte, MaxKeyLength+1)
	for i := range oversized {
		oversized[i] = 'k'
	}
	if _, err := Parse(string(oversized)); err == nil {
		t.Fatal("Parse oversized should fail")
	}
}

func TestResolvePrefersHeader(t *testing.T) {
	key, err := Resolve("header-key", "")
	if err != nil || key != "header-key" {
		t.Fatalf("Resolve header-only = %q, %v", key, err)
	}
	key, err = Resolve("", "body-key")
	if err != nil || key != "body-key" {
		t.Fatalf("Resolve body-only = %q, %v", key, err)
	}
	key, err = Resolve("same", "same")
	if err != nil || key != "same" {
		t.Fatalf("Resolve matching = %q, %v", key, err)
	}
	if _, err = Resolve("header-key", "other-key"); err == nil || !IsConflict(err) {
		t.Fatalf("Resolve mismatch = %v, want conflict", err)
	}
	if _, err = Resolve("", ""); err == nil || IsConflict(err) {
		t.Fatalf("Resolve empty = %v, want missing", err)
	}
}

func TestStoreReplaysFirstResult(t *testing.T) {
	store := NewStore(StoreOptions{TTL: time.Hour, MaxEntries: 8})
	store.StoreResult("scope", 7, "key-1", HashRequest("a"), map[string]string{"v": "first"})
	data, replayed, err := store.Lookup("scope", 7, "key-1", HashRequest("a"))
	if err != nil || !replayed {
		t.Fatalf("Lookup replay = %+v, %v, %v", data, replayed, err)
	}
	if got, _ := data.(map[string]string); got["v"] != "first" {
		t.Fatalf("replayed data = %+v", data)
	}
	// First-write-wins: a later capture never overwrites.
	store.StoreResult("scope", 7, "key-1", HashRequest("a"), map[string]string{"v": "second"})
	data, _, _ = store.Lookup("scope", 7, "key-1", HashRequest("a"))
	if got, _ := data.(map[string]string); got["v"] != "first" {
		t.Fatalf("overwrite data = %+v", data)
	}
}

func TestStoreConflictsOnHashMismatch(t *testing.T) {
	store := NewStore(StoreOptions{TTL: time.Hour, MaxEntries: 8})
	store.StoreResult("scope", 7, "key-1", HashRequest("a"), "first")
	if _, _, err := store.Lookup("scope", 7, "key-1", HashRequest("b")); err == nil || !IsConflict(err) {
		t.Fatalf("Lookup mismatch = %v, want conflict", err)
	}
}

func TestStoreExpiresEntries(t *testing.T) {
	now := time.Now()
	current := now
	store := NewStore(StoreOptions{TTL: time.Hour, MaxEntries: 8, Now: func() time.Time { return current }})
	store.StoreResult("scope", 7, "key-1", HashRequest("a"), "first")
	current = now.Add(25 * time.Hour)
	if _, replayed, err := store.Lookup("scope", 7, "key-1", HashRequest("a")); err != nil || replayed {
		t.Fatalf("expired Lookup replayed=%v err=%v", replayed, err)
	}
	// Expired keys execute again and capture fresh results.
	store.StoreResult("scope", 7, "key-1", HashRequest("b"), "second")
	data, replayed, err := store.Lookup("scope", 7, "key-1", HashRequest("b"))
	if err != nil || !replayed || data != "second" {
		t.Fatalf("recapture = %+v, %v, %v", data, replayed, err)
	}
}

func TestStoreIsolatesScopeAndUser(t *testing.T) {
	store := NewStore(StoreOptions{TTL: time.Hour, MaxEntries: 8})
	store.StoreResult("scope-a", 7, "key-1", HashRequest("a"), "first")
	if _, replayed, _ := store.Lookup("scope-b", 7, "key-1", HashRequest("a")); replayed {
		t.Fatal("scope must isolate replay")
	}
	if _, replayed, _ := store.Lookup("scope-a", 8, "key-1", HashRequest("a")); replayed {
		t.Fatal("user must isolate replay")
	}
}

func TestStoreNilSafe(t *testing.T) {
	var store *Store
	if _, replayed, err := store.Lookup("scope", 7, "key", "hash"); err != nil || replayed {
		t.Fatalf("nil Lookup = %v, %v", replayed, err)
	}
	store.StoreResult("scope", 7, "key", "hash", "data")
}

func TestHashRequestIsStable(t *testing.T) {
	first, second := HashRequest("a", "b"), HashRequest("a", "b")
	if first != second {
		t.Fatal("hash must be deterministic")
	}
	if HashRequest("a", "b") == HashRequest("a", "c") {
		t.Fatal("hash must distinguish inputs")
	}
}

func TestHashRequestSeparatesNULContainingParts(t *testing.T) {
	if HashRequest("a\x00b", "c") == HashRequest("a", "b\x00c") {
		t.Fatal("NUL-containing tuples must not collide")
	}
}

func TestStoreZeroValueIsSafe(t *testing.T) {
	var store Store
	store.StoreResult("scope", 1, "key", HashRequest("a"), "value")
	data, replayed, err := store.Lookup("scope", 1, "key", HashRequest("a"))
	if err != nil || !replayed || data != "value" {
		t.Fatalf("zero-value store lookup = %#v, %v, %v", data, replayed, err)
	}
}

func TestStoreDoCoordinatesConcurrentCallers(t *testing.T) {
	store := NewStore(DefaultStoreOptions())
	started := make(chan struct{})
	release := make(chan struct{})
	first := make(chan struct {
		data     any
		replayed bool
		err      error
	}, 1)
	go func() {
		data, replayed, err := store.Do(context.Background(), "scope", 1, "key", HashRequest("a"), func() (any, error) {
			close(started)
			<-release
			return "first", nil
		})
		first <- struct {
			data     any
			replayed bool
			err      error
		}{data, replayed, err}
	}()
	<-started
	second := make(chan struct {
		data     any
		replayed bool
		err      error
	}, 1)
	go func() {
		data, replayed, err := store.Do(context.Background(), "scope", 1, "key", HashRequest("a"), func() (any, error) {
			return "wrong", errors.New("must not execute")
		})
		second <- struct {
			data     any
			replayed bool
			err      error
		}{data, replayed, err}
	}()
	close(release)
	gotFirst, gotSecond := <-first, <-second
	if gotFirst.err != nil || gotFirst.replayed || gotFirst.data != "first" {
		t.Fatalf("leader result = %+v", gotFirst)
	}
	if gotSecond.err != nil || !gotSecond.replayed || gotSecond.data != "first" {
		t.Fatalf("waiter result = %+v", gotSecond)
	}
}
