package events

// Board-scoped event visibility (issue #940).
//
// Board envelopes carry protected content: board ids, roster membership,
// prompts, and input/output asset identifiers must never reach a principal
// who holds no current board access. Servers therefore filter at DELIVERY
// time (SSE replay + live, cross-instance fanout consumers, webhook
// dispatch), never at emit time: an envelope emitted before a revocation
// but delivered after it is dropped, which is what closes the
// send-vs-revoke race and the pre-revocation backlog.
//
// Delivery rules (single home; HTTP + webhooks + bridge all consult them):
//
//   - board.document_replaced: delivered only to
//     principals holding current board read access (resolved through the
//     Boards RoleReader port at serve time, never cached, never
//     frontend-filtered).
//   - board.access_changed: directed control signal, delivered ONLY to the
//     named member (data.member_id). It carries no content beyond the
//     recipient's already-known board identity plus the access version.
//     It never rides webhooks.
//   - task.*: delivered to the current submitter or readable Board audience.
//   - media.*: delivered only while canonical Media authorization allows read.
//   - billing.settled: Workspace membership gates delivery.
//   - Workspace webhooks carry checkout-session billing.settled only; Task-linked
//     settlements and protected principal-scoped
//     Task, Media and Board frames are rejected at enqueue and delivery.
//
// Worker/internal vs external boundary: the internal task.status_changed
// channel and River jobs may carry execution words (pending) and provider
// error text; NOTHING crosses to the v1 envelope except through
// TaskStatusV1Type + the typed data builders, which fail closed on
// unknown statuses and expose refs/codes only.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"ideaboard/api/internal/platform/contract"
)

// WorkspaceEventHandler handles a remote-origin v1 envelope.
type WorkspaceEventHandler func(ctx context.Context, envelope V1Envelope) error

// IsBoardScopedEventType reports whether typ names a board-content
// envelope subject to per-recipient board-access filtering.
func IsBoardScopedEventType(typ string) bool {
	switch strings.TrimSpace(typ) {
	case EventV1BoardDocumentReplaced, EventV1BoardAccessChanged:
		return true
	default:
		return false
	}
}

// IsDirectedAccessSignal reports whether the envelope is a directed
// access-loss control signal (deliver to the named member only, never to
// webhooks, never to ordinary replay for anyone else).
func IsDirectedAccessSignal(envelope V1Envelope) bool {
	return strings.TrimSpace(envelope.Type) == EventV1BoardAccessChanged
}

// BoardIDOf extracts the board identity from a board-scoped envelope's
// typed data. ok=false when the envelope is not board-scoped or the data
// carries no board id.
func BoardIDOf(envelope V1Envelope) (string, bool) {
	if !IsBoardScopedEventType(envelope.Type) {
		return "", false
	}
	var probe struct {
		BoardID string `json:"board_id"`
	}
	if err := json.Unmarshal(envelope.Data, &probe); err != nil {
		return "", false
	}
	if !contract.IsValidPublicID(contract.PrefixBoard, strings.TrimSpace(probe.BoardID)) {
		return "", false
	}
	return strings.TrimSpace(probe.BoardID), true
}

// AccessChangedRecipient extracts the directed recipient (data.member_id)
// of a board.access_changed envelope. ok=false for any other type.
func AccessChangedRecipient(envelope V1Envelope) (string, bool) {
	if !IsDirectedAccessSignal(envelope) {
		return "", false
	}
	var probe struct {
		MemberID string `json:"member_id"`
	}
	if err := json.Unmarshal(envelope.Data, &probe); err != nil {
		return "", false
	}
	if !contract.IsValidPublicID(contract.PrefixUser, strings.TrimSpace(probe.MemberID)) {
		return "", false
	}
	return strings.TrimSpace(probe.MemberID), true
}

// AccessChangedVersion extracts the monotonic access version carried by a
// board.access_changed envelope. Callers order access signals by it; it
// MUST NOT order document revisions or any other fact.
func AccessChangedVersion(envelope V1Envelope) (int64, bool) {
	if !IsDirectedAccessSignal(envelope) {
		return 0, false
	}
	var probe struct {
		AccessVersion int64 `json:"access_version"`
	}
	if err := json.Unmarshal(envelope.Data, &probe); err != nil {
		return 0, false
	}
	if probe.AccessVersion < 1 {
		return 0, false
	}
	return probe.AccessVersion, true
}

// StableEventID derives one deterministic evt_ id for an outbox fact. The
// parts converge: a live emit and a later outbox replay of the same fact
// mint the same id, so the hot log, NATS MsgID dedup, and webhook dedup
// collapse them to one delivery. Callers pass the fact's identity words in
// order (task: task id + status + execution token; board access: board +
// member + version); each part is trimmed before the NUL join.
func StableEventID(parts ...string) string {
	trimmed := make([]string, len(parts))
	for i, part := range parts {
		trimmed[i] = strings.TrimSpace(part)
	}
	sum := sha256.Sum256([]byte(strings.Join(trimmed, "\x00")))
	return contract.PrefixEvent + hex.EncodeToString(sum[:12])
}

// NewWorkspaceEventBridgeHandler returns the cross-instance fanout
// consumer: remote-origin envelopes validated by the NATS bridge append
// into the local hot log so this instance's SSE replay + live subscribers
// observe other instances' commits. Append is idempotent on evt_ id
// (duplicates return the stored copy WITHOUT refanout), so redeliveries
// converge and the bridge can never loop: appends never republish.
// Per-recipient permission filtering stays at SSE/webhook serve time.
func NewWorkspaceEventBridgeHandler(log *WorkspaceEventLog) WorkspaceEventHandler {
	return func(_ context.Context, envelope V1Envelope) error {
		return appendBridgedEnvelope(log, envelope)
	}
}

func appendBridgedEnvelope(log *WorkspaceEventLog, envelope V1Envelope) error {
	if log == nil {
		return nil
	}
	// Bridge payloads were validated at decode; a permanently-invalid
	// envelope is dropped here without error (surfacing it would Nak and
	// redeliver poison forever), never bridged.
	valid := ValidateV1Envelope(envelope) == nil
	if !valid {
		return nil
	}
	log.Append(envelope)
	return nil
}
