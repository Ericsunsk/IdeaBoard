package composition

// Control audit adapters resolve public operator IDs through the request-scoped
// user directory and persist guarded calls through audit.Store. Records without
// a domain target use the route template; unknown capabilities use control:unknown.
// Best-effort writes log failures; transaction-bound writes propagate errors.

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
