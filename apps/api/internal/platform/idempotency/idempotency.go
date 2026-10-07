// Package idempotency is the shared HTTP-edge helper for POST write
// idempotency (backend-hardening #771).
//
// Contract:
//   - POST writes in task, reward and board paths require an
//     `Idempotency-Key` header (header-only: no body fallback —
//     contract.IdempotencyKeyFromHeader is the single reader).
//   - Missing/invalid/conflicting keys are classified through
//     `platform/apperror` (wire codes `idempotency_key_required` and
//     `idempotency_conflict`). Rewards maps the same outcomes onto its
//     existing `reward_*` namespace instead of minting new codes.
//   - Replay within ReplayTTL (24h) returns the first result.
//     Reward-create replays through its durable uniqueness facts
//     (reward_codes.created_by+idempotency_key, reward_redemptions
//     natural user+code dedup); board POSTs replay through the
//     process-local Store below.
//   - Task submit does NOT depend on this store. It has its own durable,
//     transactional idempotency fact (task_submission_idempotency, written
//     in the same transaction as the Credit Hold + task insert + River
//     enqueue), so a retried submit replays the original task even after a
//     crash or a cache eviction. The Store is still consulted by the v1
//     task edge as a cheap in-process fast path / same-request coalescer,
//     but it is not the authority: correctness no longer depends on it.
package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/platform/apperror"
)

// Header is the wire header carrying the caller-chosen idempotency key.
// CORS and the gateway contract already forward it.
const Header = "Idempotency-Key"

// MaxKeyLength bounds the opaque key (task client_action_id and the reward
// create body key both cap at 128).
const MaxKeyLength = 128

// ReplayTTL bounds how long a first result is replayed.
const ReplayTTL = 24 * time.Hour

const (
	// CodeRequired renders when the key is missing or malformed (400).
	CodeRequired apperror.Code = "idempotency_key_required"
	// CodeConflict renders when a key is reused with a different request (409).
	CodeConflict apperror.Code = "idempotency_conflict"
)

// MissingError classifies a missing key: the caller can retry with a key.
func MissingError() *apperror.Error {
	return apperror.BadRequest(CodeRequired, msgRequired)
}

// InvalidError classifies a malformed key (over length).
func InvalidError() *apperror.Error {
	return apperror.BadRequest(CodeRequired, msgRequired2)
}

// ConflictError classifies a key reused with a different request payload.
func ConflictError() *apperror.Error {
	return apperror.Conflict(CodeConflict, msgConflict)
}

// IsConflict reports whether err is a key-reuse conflict (vs missing/invalid).
func IsConflict(err error) bool {
	return apperror.CodeOf(err) == CodeConflict
}

// Parse validates a raw header value: non-empty after trim, within length.
func Parse(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", MissingError()
	}
	if len(key) > MaxKeyLength {
		return "", InvalidError()
	}
	return key, nil
}

// RenderStatusOf returns the HTTP status a caller must render for err,
// defaulting to 400 for unclassified failures.
func RenderStatusOf(err error) int {
	if status := apperror.HTTPStatusOf(err); status != 0 {
		return status
	}
	return 400
}

// RenderCodeOf returns the wire code for err, defaulting to CodeRequired.
func RenderCodeOf(err error) string {
	if code := apperror.CodeOf(err); code != "" {
		return string(code)
	}
	return string(CodeRequired)
}

// HashRequest derives the request fingerprint compared on key reuse. Only
// the digest is persisted, never the raw fields (join tokens stay secret).
func HashRequest(parts ...string) string {
	containsNUL := false
	for _, part := range parts {
		if strings.ContainsRune(part, '\x00') {
			containsNUL = true
			break
		}
	}
	var payload []byte
	if !containsNUL {
		// Keep the legacy digest stable for ordinary inputs and durable rows.
		payload = []byte(strings.Join(parts, "\x00"))
	} else {
		// NUL-delimited framing is ambiguous for caller-controlled fields.
		// Length-prefix exceptional inputs so distinct tuples stay distinct.
		payload = []byte{'\x01'}
		var length [8]byte
		for _, part := range parts {
			binary.BigEndian.PutUint64(length[:], uint64(len(part)))
			payload = append(payload, length[:]...)
			payload = append(payload, part...)
		}
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// Store is a process-local first-result replay cache for POST writes whose
// service layer has no durable idempotency fact (board share/member paths).
// Reward create/redeem replay through their durable facts and never use
// this store. Task submit has its own durable, transactional claim
// (task_submission_idempotency); the v1 task edge may still pass through
// here as an in-process fast path, but a Store miss or eviction no longer
// permits a duplicate settlement. Entries expire after TTL; methods are
// nil-safe so handlers constructed without a store still enforce keys and
// skip replay.
//
// This store is the process-local fast path only: Control mutations
// additionally write their durable fact into
// control_idempotency_records through ControlStore (durable.go), which
// is the cross-process, restart-safe authority. The money path (task
// submit) no longer relies on it.
type Store struct {
	mu         sync.Mutex
	items      map[string]storedResult
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
	inFlight   map[string]*inFlightResult
}

type inFlightResult struct {
	requestHash string
	done        chan struct{}
	data        any
	err         error
}

type storedResult struct {
	data        any
	requestHash string
	storedAt    time.Time
	expiresAt   time.Time
}

// StoreOptions configures a Store; zero values take production defaults.
type StoreOptions struct {
	TTL        time.Duration
	MaxEntries int
	Now        func() time.Time
}

// DefaultStoreOptions returns the production replay policy: 24h, capped.
func DefaultStoreOptions() StoreOptions {
	return StoreOptions{TTL: ReplayTTL, MaxEntries: 4096}
}

// NewStore builds a replay Store from options.
func NewStore(opts StoreOptions) *Store {
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = ReplayTTL
	}
	maxEntries := opts.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 4096
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Store{items: make(map[string]storedResult), ttl: ttl, maxEntries: maxEntries, now: now, inFlight: make(map[string]*inFlightResult)}
}

func (s *Store) initLocked() {
	if s.items == nil {
		s.items = make(map[string]storedResult)
	}
	if s.inFlight == nil {
		s.inFlight = make(map[string]*inFlightResult)
	}
	if s.ttl <= 0 {
		s.ttl = ReplayTTL
	}
	if s.maxEntries <= 0 {
		s.maxEntries = 4096
	}
	if s.now == nil {
		s.now = time.Now
	}
}

func storeKey(scope string, userID int64, key string) string {
	return scope + "\x00" + strconv.FormatInt(userID, 10) + "\x00" + key
}

// Lookup returns the first result when key+scope+user replay the same
// request hash. A different hash is a conflict; unknown or expired keys
// miss so the caller executes and captures.
func (s *Store) Lookup(scope string, userID int64, key, requestHash string) (data any, replayed bool, err error) {
	if s == nil {
		return nil, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initLocked()
	now := s.now().UTC()
	entry, ok := s.items[storeKey(scope, userID, key)]
	if !ok || !now.Before(entry.expiresAt) {
		if ok {
			delete(s.items, storeKey(scope, userID, key))
		}
		return nil, false, nil
	}
	if entry.requestHash != requestHash {
		return nil, false, ConflictError()
	}
	return entry.data, true, nil
}

// StoreResult caches the first success for key+scope+user. Later captures
// for the same key never overwrite the first result.
func (s *Store) StoreResult(scope string, userID int64, key, requestHash string, data any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initLocked()
	now := s.now().UTC()
	k := storeKey(scope, userID, key)
	if entry, ok := s.items[k]; ok && now.Before(entry.expiresAt) {
		return
	}
	s.purgeExpiredLocked(now)
	if len(s.items) >= s.maxEntries {
		s.evictOldestLocked()
	}
	s.items[k] = storedResult{data: data, requestHash: requestHash, storedAt: now, expiresAt: now.Add(s.ttl)}
}

// Do atomically coordinates same-key callers around one side effect. A
// waiter receives the leader's exact result (or error); a different request
// hash is rejected before any side effect runs.
func (s *Store) Do(ctx context.Context, scope string, userID int64, key, requestHash string, fn func() (any, error)) (any, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil {
		data, err := fn()
		return data, false, err
	}
	k := storeKey(scope, userID, key)
	s.mu.Lock()
	s.initLocked()
	now := s.now().UTC()
	if entry, ok := s.items[k]; ok && now.Before(entry.expiresAt) {
		if entry.requestHash != requestHash {
			s.mu.Unlock()
			return nil, false, ConflictError()
		}
		data := entry.data
		s.mu.Unlock()
		return data, true, nil
	}
	if flight, ok := s.inFlight[k]; ok {
		if flight.requestHash != requestHash {
			s.mu.Unlock()
			return nil, false, ConflictError()
		}
		done := flight.done
		s.mu.Unlock()
		select {
		case <-done:
			return flight.data, true, flight.err
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
	flight := &inFlightResult{requestHash: requestHash, done: make(chan struct{})}
	s.inFlight[k] = flight
	s.mu.Unlock()

	data, err := fn()
	s.mu.Lock()
	flight.data, flight.err = data, err
	delete(s.inFlight, k)
	if err == nil {
		now = s.now().UTC()
		s.purgeExpiredLocked(now)
		if len(s.items) >= s.maxEntries {
			s.evictOldestLocked()
		}
		s.items[k] = storedResult{data: data, requestHash: requestHash, storedAt: now, expiresAt: now.Add(s.ttl)}
	}
	close(flight.done)
	s.mu.Unlock()
	return data, false, err
}

func (s *Store) purgeExpiredLocked(now time.Time) {
	for k, entry := range s.items {
		if !now.Before(entry.expiresAt) {
			delete(s.items, k)
		}
	}
}

func (s *Store) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	first := true
	for k, entry := range s.items {
		if first || entry.storedAt.Before(oldest) {
			oldestKey, oldest, first = k, entry.storedAt, false
		}
	}
	if !first {
		delete(s.items, oldestKey)
	}
}
