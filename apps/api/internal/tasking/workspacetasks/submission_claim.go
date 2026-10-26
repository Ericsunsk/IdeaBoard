package workspacetasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strconv"
	"strings"

	"ideaboard/api/internal/platform/contract"

	"github.com/jackc/pgx/v5"
)

// TaskSubmitOperation is the durable idempotency operation name for
// POST /workspaces/{ws}/tasks. It is part of the claim's unique scope, so a
// future submit-shaped operation (e.g. task:batch_create) can reuse the same
// client key namespace without colliding with single-task submit.
const TaskSubmitOperation = "task:create"

// ErrIdempotencyConflict reports the same (workspace, operation, key) reused
// with a different request body. Handlers render it as 409
// task_idempotency_conflict. It is distinct from ErrStateConflict: the task
// state machine is irrelevant here, the client reused a key for a different
// request.
var ErrIdempotencyConflict = errors.New("workspacetasks: idempotency key reused with a different request")

// SubmissionClaim is the durable submit idempotency fact. It is written in
// the SAME transaction as the settlement it fences, so it commits with the
// Hold + task + River job or not at all.
type SubmissionClaim struct {
	// WorkspaceID is the workspace public id the claim is scoped to.
	WorkspaceID string
	// Operation is the stable contract operation name (TaskSubmitOperation).
	Operation string
	// IdempotencyKey is the caller-chosen key from the Idempotency-Key header.
	IdempotencyKey string
	// RequestHash fingerprints the request body. Same key + same hash replays;
	// same key + different hash is a conflict.
	RequestHash string
	// TaskPublicID is the task this claim settles. It is minted before the
	// transaction opens and written with the claim, so a replay can return
	// the original task instead of minting a second one.
	TaskPublicID string
}

// TxSubmissionClaimStore persists the durable submit idempotency fact inside
// the settlement transaction.
//
// Why the key derivation MUST be durable (the bug this seam exists to fix):
// the Credit Hold's ledger idempotency key used to be derived from a task id
// that Create minted fresh on every call. Two submissions of the same client
// key therefore produced two DIFFERENT ledger keys, so the ledger's
// pg_advisory_xact_lock and unique constraint could not fence the second one.
// A retried submit after a lost response minted a second task, took a second
// hold, and enqueued a second River job: two charges, two images, one 202.
// Deriving the hold key from the durable claim scope makes the ledger key a
// function of client intent, so the ledger's own replay path becomes a
// second line of defense behind this claim.
type TxSubmissionClaimStore interface {
	// ClaimSubmissionInTx records the claim, or reports the existing one.
	//
	// claimed=true means this transaction won the key and must go on to
	// settle. claimed=false means a claim already exists; existing carries
	// it so the caller can replay (same RequestHash) or fail closed
	// (different RequestHash).
	//
	// Implementations must be race-safe: concurrent submits of one key must
	// serialize on the unique constraint so exactly one settles and the
	// other observes the winner.
	ClaimSubmissionInTx(ctx context.Context, tx pgx.Tx, claim SubmissionClaim) (existing *SubmissionClaim, claimed bool, err error)
}

// NormalizeSubmitIdempotencyKey trims and validates the caller-chosen key.
// An empty key is rejected rather than silently treated as "no key": the
// submit path is fail-closed, because without a key the settlement cannot be
// fenced at all.
func NormalizeSubmitIdempotencyKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", newValidationError(FieldError{Field: "idempotency_key", Message: "required"})
	}
	if len(key) > contract.IdempotencyMaxKeyLength {
		return "", newValidationError(FieldError{Field: "idempotency_key", Message: "too_long"})
	}
	return key, nil
}

// TaskSubmitRequestHash fingerprints the request body for the durable claim.
// It must cover every input that changes what gets settled or rendered, so
// that reusing a key with a different body is a conflict rather than a
// silent replay of the wrong task.
//
// Only the digest is persisted; the raw prompt and params never reach the
// claim table.
//
// The Board binding is part of the fingerprint (#937): the same key with a
// different Board ID is a conflict, never a silent replay of a task bound
// to the wrong Board.
func TaskSubmitRequestHash(workspaceID, kind string, mediaAssetIDs []string, prompt string, paramsJSON string, quantity int64, resolution string, boardID string) string {
	assets := append([]string(nil), mediaAssetIDs...)
	slices.Sort(assets)
	sum := sha256.Sum256([]byte(strings.Join([]string{
		TaskSubmitOperation,
		strings.TrimSpace(workspaceID),
		strings.TrimSpace(kind),
		strings.Join(assets, ","),
		prompt,
		paramsJSON,
		strconv.FormatInt(quantity, 10),
		strings.TrimSpace(resolution),
		strings.TrimSpace(boardID),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}
