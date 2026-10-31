package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"
)

// TaskSubmitOperation is the durable idempotency operation name for
// POST /workspaces/{ws}/tasks. It is part of the claim's unique scope, so a
// future submit-shaped operation (e.g. task:batch_create) can reuse the same
// client key namespace without colliding with single-task submit.
const TaskSubmitOperation = "task:create"

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

// NormalizeSubmitIdempotencyKey trims and validates the caller-chosen key.
// An empty key is rejected rather than silently treated as "no key": the
// submit path is fail-closed, because without a key the settlement cannot be
// fenced at all.
func NormalizeSubmitIdempotencyKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", newValidationError(FieldError{Field: "idempotency_key", Message: "required"})
	}
	if len(key) > idempotencyMaxKeyLength {
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

// TaskSubmitHoldKey derives the Credit Hold idempotency key from the
// durable claim scope (workspace + idempotency key). It is deterministic:
// two calls with the same client key produce the SAME hold key, so the
// ledger's unique constraint fences duplicates even if the claim check
// were bypassed.
func TaskSubmitHoldKey(workspaceID, idempotencyKey string) string {
	h := sha256.New()
	h.Write([]byte("task_hold\n"))
	h.Write([]byte(strings.TrimSpace(workspaceID)))
	h.Write([]byte("\n"))
	h.Write([]byte(strings.TrimSpace(idempotencyKey)))
	return "thold_" + hex.EncodeToString(h.Sum(nil))[:24]
}
