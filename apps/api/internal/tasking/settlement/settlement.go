package settlement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/db/dbutil"

	"github.com/jackc/pgx/v5"
)

// Settlement state vocabulary. The wire and database words are unchanged
// ("free", "held", "captured", "released"); the constants only kill the
// raw literals at the validation sites.
const (
	StateFree     = "free"
	StateHeld     = "held"
	StateCaptured = "captured"
	StateReleased = "released"
)

// IsInitialState reports whether state is a legal durable initial row
// (free: nothing reserved; held: hold outstanding).
func IsInitialState(state string) bool {
	return state == StateFree || state == StateHeld
}

// IsTerminalState reports whether state is a legal durable terminal row.
func IsTerminalState(state string) bool {
	return state == StateCaptured || state == StateReleased
}

// CaptureSplit derives the per-output unit billing capped at the held
// total, with the remainder released. Both legs run in the same tx.
// Moved verbatim from workspacetasks: total is the held total, unit the
// quoted per-output price, outputCount the persisted output refs.
func CaptureSplit(total, unit int64, outputCount int) (capture, release int64) {
	capture = int64(outputCount) * unit
	if capture > total {
		capture = total
	}
	if capture < 0 {
		capture = 0
	}
	release = total - capture
	if release < 0 {
		release = 0
	}
	return capture, release
}

// taskSubmitOperation scopes the durable task hold key. It must equal
// workspacetasks.TaskSubmitOperation ("task:create"): the ledger key is a
// function of client intent (workspace + operation + key), never of the
// per-call task id, so a retried submit replays instead of double-holding.
// Pinned by TestTaskHoldKeyPinsDurableIdentity; change both together.
const taskSubmitOperation = "task:create"

// TaskHoldKey derives the Credit Hold ledger key from the durable submit
// identity (workspace + operation + client key), never the per-attempt task ID.
func TaskHoldKey(workspaceID, idempotencyKey string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		"task-hold",
		taskSubmitOperation,
		strings.TrimSpace(workspaceID),
		strings.TrimSpace(idempotencyKey),
	}, "\x00")))
	return "task-hold:" + taskSubmitOperation + ":" + hex.EncodeToString(sum[:])
}

// TaskCaptureKey is the capture idempotency key for one task.
func TaskCaptureKey(taskID string) string {
	return "task-capture:" + strings.TrimSpace(taskID)
}

// TaskReleaseKey is the release idempotency key for one task.
func TaskReleaseKey(taskID string) string {
	return "task-release:" + strings.TrimSpace(taskID)
}

// VisionHoldKey is the hold idempotency key for one vision operation.
func VisionHoldKey(operationID string) string {
	return "ai-search:" + strings.TrimSpace(operationID) + ":hold"
}

// VisionCaptureKey is the capture idempotency key for one vision operation.
func VisionCaptureKey(operationID string) string {
	return "ai-search:" + strings.TrimSpace(operationID) + ":capture"
}

// VisionReleaseKey is the release idempotency key for one vision operation.
func VisionReleaseKey(operationID string) string {
	return "ai-search:" + strings.TrimSpace(operationID) + ":release"
}

// VisionReconcileReleaseKey is the reconciliation-release idempotency key
// for one vision operation (distinct from the deterministic-failure
// release so a reconciled hold never replays a failed release).
func VisionReconcileReleaseKey(operationID string) string {
	return "ai-search:" + strings.TrimSpace(operationID) + ":reconcile-release"
}

// Transactor demarcates the settlement transaction. It returns the
// caller-owned Scope (never a bare handle): the postgres Store opens it
// with BeginOwned, the in-memory fakes wrap their shim transaction with
// dbutil.ScopeForTest, and BorrowOrBeginScope joins the ambient carrier
// when one is staged. Ownership travels with the Scope, so callers end
// it with scope.Commit/scope.Rollback and never track a separate bool.
type Transactor interface {
	BeginSettlementScope(ctx context.Context) (*dbutil.Scope, error)
}

// BorrowOrBeginScope joins the ambient guard transaction when one is staged
// in ctx, or opens the store's own settlement transaction otherwise. The
// returned Scope carries ownership: Owned() true means the caller opened
// it and must commit or roll it back; false means it is BORROWED from the
// guard, which alone commits or rolls it back.
//
// The borrowed leg wraps the carrier in a borrowed-safe handle (via
// dbutil.Begin with a nil pool): statements delegate to the carrier while
// direct Commit/Rollback refuse with dbutil.ErrBorrowedTxLifecycle, so a
// borrowed participant can never end the owner's unit even when it
// bypasses Scope.Commit/Scope.Rollback. The owned leg returns the
// transactor's Scope unchanged.
func BorrowOrBeginScope(ctx context.Context, transactor Transactor) (*dbutil.Scope, error) {
	if _, ok := dbutil.AmbientTxFromContext(ctx); ok {
		return dbutil.Begin(ctx, nil, pgx.TxOptions{})
	}
	if transactor == nil {
		return nil, errors.New("settlement: settlement transactor is nil")
	}
	scope, err := transactor.BeginSettlementScope(ctx)
	if err != nil {
		return nil, fmt.Errorf("settlement: begin settlement scope: %w", err)
	}
	if scope == nil || scope.Tx() == nil {
		return nil, errors.New("settlement: begin settlement scope returned nil")
	}
	return scope, nil
}

// StageOrDefer stages one settlement event after its unit is durable. An
// owned transaction commits at the caller, so the event stages
// synchronously for the caller to flush. A borrowed guard transaction
// commits later at the guard, so the stage registers as a post-commit
// hook instead: it runs only after the guard's commit, never on rollback.
// Without a registry (raw carriers outside the guard) the legacy
// synchronous stage is kept so the caller can flush after the commit it
// owns.
func StageOrDefer(ctx context.Context, owned bool, stage func(context.Context)) {
	if stage == nil {
		return
	}
	if owned {
		stage(ctx)
		return
	}
	if !dbutil.DeferAmbientPostCommit(ctx, stage) {
		stage(ctx)
	}
}
