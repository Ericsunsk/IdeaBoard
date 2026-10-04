package composition

// Control Plane durable audit adapter (step 1: durable but non-atomic).
//
// This is the ONLY place where the Control Plane's neutral
// httpapi.ControlAuditSink meets the durable control_audit_records store.
// The Control Plane declares WHAT happened (one guarded call in its own
// public-id vocabulary); this adapter decides HOW it is persisted, and
// only this file may know that persistence means "resolve the operator's
// public usr_ id to its internal numeric id, then INSERT into
// control_audit_records".
//
// WHY THE ADAPTER LIVES IN COMPOSITION
// Same reason as control_task_event_adapter.go: the control handlers must
// stay free of durable infrastructure entirely, which is what
// TestControlNeverTouchesEventLayer guards (the Control Plane must never
// reach for the event layer directly; by the same token it must never
// reach for the audit store directly). The sink keeps the dependency
// arrow pointing inward (composition -> control handlers) instead of
// outward (control handlers -> audit store).
//
// STEP 1 SCOPE (durable but NON-atomic)
// This adapter dual-writes AFTER the guarded business operation commits,
// in its own detached transaction owned by audit.Store. It is durable
// (every emitted record eventually lands a row) but NOT atomic: a crash
// between the business commit and this write leaves the business fact
// without its audit row, and a business rollback after emission leaves
// an audit row without its business fact. Closing that gap (passing the
// business transaction through every control handler) is explicitly
// step 2 and is NOT attempted here.
//
// PLACEHOLDER CONVENTIONS (architecture decision)
// The durable table enforces two CHECK constraints the in-memory record
// does not:
//   - chk_control_audit_records_target requires non-empty target_type
//     AND target_id, but most control routes carry no domain target
//     (their controlRoute.Target is nil), and every fail-fast path
//     (missing reason/idempotency key, oversized body, operator-grant
//     denial in the middleware) emits BEFORE target extraction, so the
//     target is unconditionally empty there.
//   - chk_control_audit_records_capability requires capability LIKE
//     'control:%', but the middleware operator-grant denial emits with
//     an empty capability because the family label is only known inside
//     the per-route guard, which the request never reaches.
//
// When the record carries no target, this adapter persists
// target_type='route' and target_id=<route template> (falling back to
// the operation name, then to 'unknown' when even the route is empty).
// A route placeholder is deliberately conservative: it is the only
// locator available at EVERY emission point, it keeps the row searchable
// by the operator-facing route template, and it never fabricates a
// domain identity (user/task/order/...) the call never named. Audit data
// must stay conservative rather than invent semantics.
//
// When the record carries no usable capability, this adapter persists
// capability='control:unknown'. The empty capability only occurs for
// the pre-family grant denial, where the family genuinely is unknown;
// claiming 'control:platform' (or any real family) would misattribute
// the denial, while 'control:unknown' preserves the unknown-family fact
// and still satisfies the CHECK constraint.
//
// OPERATOR MAPPING
// ControlAuditRecord.OperatorID is public-only (usr_... for human
// sessions, a service subject string otherwise) while RecordParams
// needs the internal numeric FK. Each audit performs exactly one
// public->internal lookup through the SAME request-scoped
// V1UserDirectory the control handlers use (no cache, no new global
// state). Service subjects without a users row resolve to nothing and
// degrade to the slog trail only, matching audit.Store's skip for
// non-positive operator ids.
//
// FAILURE MODE
// The audit write is an observer, never a participant: persistence
// failures are slog.Warn-logged and swallowed so the guarded business
// operation can never fail because its audit trail did.

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/platform/audit"
	httpapi "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
)

const (
	// controlAuditWriteTimeout bounds one durable audit write (operator
	// lookup + INSERT). The seam is synchronous fire-and-forget, so a
	// slow database must never hold the operator's request open.
	controlAuditWriteTimeout = 2 * time.Second

	// controlAuditPlaceholderTargetType marks rows whose guarded call
	// named no domain target. See the package comment for why 'route'
	// is the conservative placeholder.
	controlAuditPlaceholderTargetType = "route"

	// controlAuditUnknownCapability marks denials emitted before the
	// family label was known (middleware operator-grant denial). It
	// satisfies chk_control_audit_records_capability without claiming
	// a family the request never reached.
	controlAuditUnknownCapability = "control:unknown"
)

// controlAuditStore is the narrow durable seam the adapter needs. It
// matches audit.Store.AppendRecord so production passes *audit.Store
// directly while tests inject fakes.
type controlAuditStore interface {
	AppendRecord(ctx context.Context, params audit.RecordParams) error
}

func resolveControlAuditOperatorUserID(ctx context.Context, users httpapi.V1UserDirectory, record httpapi.ControlAuditRecord) (int64, bool) {
	if users == nil {
		return 0, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	publicID, err := tenancy.ParseUserPublicID(strings.TrimSpace(record.OperatorID))
	if err != nil {
		return 0, false
	}
	numeric, err := users.ResolveUserID(ctx, publicID)
	if err != nil || numeric <= 0 {
		return 0, false
	}
	return numeric, true
}

func controlAuditRecordParams(record httpapi.ControlAuditRecord, operatorID int64) audit.RecordParams {
	targetType, targetID := controlAuditTarget(record)
	return audit.RecordParams{
		OperatorUserID: operatorID,
		OperatorType:   record.OperatorType,
		OperatorID:     record.OperatorID,
		Operation:      strings.TrimSpace(record.Operation),
		Capability:     controlAuditCapability(record),
		TargetType:     targetType,
		TargetID:       targetID,
		Reason:         record.Reason,
		Outcome:        record.Outcome,
		RequestID:      record.RequestID,
		IdempotencyKey: record.IdempotencyKey,
		Route:          record.Route,
		Status:         record.Status,
		Replayed:       record.Replayed,
	}
}

// controlAuditDurableSink implements httpapi.ControlAuditSink with a
// best-effort durable write into control_audit_records.
type controlAuditDurableSink struct {
	store controlAuditStore
	users httpapi.V1UserDirectory
}

var _ httpapi.ControlAuditSink = (*controlAuditDurableSink)(nil)

// newControlAuditDurableSink builds the durable control audit sink over
// store with operator resolution via users. Either dependency may be
// nil: a nil store or an unresolvable operator degrades to the slog
// trail the guard already emitted, never to a panic or a failed
// business call.
func newControlAuditDurableSink(store controlAuditStore, users httpapi.V1UserDirectory) *controlAuditDurableSink {
	return &controlAuditDurableSink{store: store, users: users}
}

// AppendControlAudit persists one guarded Control Plane call
// best-effort. The ControlAuditSink seam carries no context.Context, so
// the adapter mints its own detached background context with a tight
// timeout: the write must survive the request's cancellation (it runs
// after the handler returned) and must never block the operator on a
// slow database.
func (a *controlAuditDurableSink) AppendControlAudit(record httpapi.ControlAuditRecord) {
	if a == nil || a.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), controlAuditWriteTimeout)
	defer cancel()

	operatorID, ok := resolveControlAuditOperatorUserID(ctx, a.users, record)
	if !ok {
		slog.Warn("control audit skipped without operator row",
			"operator_id", record.OperatorID,
			"operation", record.Operation,
			"route", record.Route)
		return
	}
	operation := strings.TrimSpace(record.Operation)
	if operation == "" {
		slog.Warn("control audit skipped without operation", "route", record.Route)
		return
	}
	params := controlAuditRecordParams(record, operatorID)
	if err := a.store.AppendRecord(ctx, params); err != nil {
		slog.Warn("control audit durable write failed",
			"operation", operation,
			"route", record.Route,
			"request_id", record.RequestID,
			"err", err)
	}
}

// controlAuditCapability normalizes the record capability for the
// chk_control_audit_records_capability CHECK (capability LIKE
// 'control:%'). Records that already carry a family label pass through
// untouched; the pre-family middleware denial (empty capability) plus
// any future non-family value converge to 'control:unknown' rather than
// inventing a family.
func controlAuditCapability(record httpapi.ControlAuditRecord) string {
	capability := strings.TrimSpace(record.Capability)
	if strings.HasPrefix(capability, "control:") && len(strings.TrimSpace(strings.TrimPrefix(capability, "control:"))) > 0 {
		return capability
	}
	return controlAuditUnknownCapability
}

// controlAuditTarget normalizes the record target for the
// chk_control_audit_records_target CHECK (both columns non-empty).
// Records that name a domain target pass through untouched; everything
// else degrades to the conservative route placeholder (never a
// fabricated domain identity).
func controlAuditTarget(record httpapi.ControlAuditRecord) (string, string) {
	targetType := strings.TrimSpace(record.TargetType)
	targetID := strings.TrimSpace(record.TargetID)
	if targetType != "" && targetID != "" {
		return targetType, targetID
	}
	route := strings.TrimSpace(record.Route)
	if route == "" {
		route = strings.TrimSpace(record.Operation)
	}
	if route == "" {
		route = "unknown"
	}
	return controlAuditPlaceholderTargetType, route
}
