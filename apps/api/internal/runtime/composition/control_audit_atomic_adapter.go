package composition

// Control Plane atomic audit adapter (step 2: participant, not observer).
//
// This is the composition-side implementation of
// httpapi.ControlAuditTxWriter: the same guarded ControlAuditRecord the
// step-1 observer sink persists AFTER the business commit is instead
// persisted INSIDE the guard-owned transaction, so the business fact
// and its audit row commit or roll back together.
//
// SEMANTIC INVERSION (versus control_audit_adapter.go): the step-1
// observer swallows persistence failures (slog.Warn only) so the
// business call can never fail because of its audit trail. This adapter
// does the opposite on purpose: a persistence failure is RETURNED, and
// the guard rolls the whole mutation transaction back with it. An audit
// failure failing the business call is the definition of atomic commit
// here, not a bug: committing the business fact without its audit row
// would re-open the exact gap step 2 closes (business commit followed
// by an audit crash leaves the fact without its trail).
//
// The step-1 placeholder conventions are reused unchanged (same
// controlAuditCapability / controlAuditTarget helpers, same package):
// the durable CHECK constraints (non-empty target, control:% capability)
// must hold inside the transaction exactly as they do outside it, and
// inventing a second normalization would risk CHECK violations that
// roll back unrelated business work.
//
// The operator skip is preserved too: service subjects without a users
// row report durable=false with a nil error, so the guard commits the
// (audit-less) transaction and keeps the slog trail only, matching the
// step-1 skip instead of failing calls that never had a durable
// operator identity.

import (
	"context"
	"log/slog"
	"strings"

	"ideaboard/api/internal/platform/audit"
	httpapi "ideaboard/api/internal/runtime/interfaces/http"

	"github.com/jackc/pgx/v5"
)

// controlAuditTxStore is the narrow participant seam the atomic adapter
// needs. It matches audit.Store.AppendRecordTx so production passes
// *audit.Store directly while tests inject fakes.
type controlAuditTxStore interface {
	AppendRecordTx(ctx context.Context, tx pgx.Tx, params audit.RecordParams) error
}

// controlAuditAtomicSink implements httpapi.ControlAuditTxWriter with a
// participant write into control_audit_records inside the guard-owned
// transaction.
type controlAuditAtomicSink struct {
	store controlAuditTxStore
	users httpapi.V1UserDirectory
}

var _ httpapi.ControlAuditTxWriter = (*controlAuditAtomicSink)(nil)

// newControlAuditAtomicSink builds the atomic control audit sink over
// store with operator resolution via users. Either dependency may be
// nil: a nil store degrades to the slog trail only (durable=false),
// never to a panic or a failed business call.
func newControlAuditAtomicSink(store controlAuditTxStore, users httpapi.V1UserDirectory) *controlAuditAtomicSink {
	return &controlAuditAtomicSink{store: store, users: users}
}

// AppendControlAuditTx resolves the operator and joins the audit row
// into tx. It returns durable=false with a nil error for the two
// step-1 skip shapes (nil store, unresolvable operator, empty
// operation): the guard then commits the transaction with the slog
// trail only. Any persistence failure is returned so the guard rolls
// the mutation back (semantic inversion versus step 1, see the package
// comment). A nil tx is a wiring error and fails the call.
func (a *controlAuditAtomicSink) AppendControlAuditTx(ctx context.Context, tx pgx.Tx, record httpapi.ControlAuditRecord) (bool, error) {
	if a == nil || a.store == nil {
		return false, nil
	}
	if tx == nil {
		return false, errControlAuditMissingTx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	operatorID, ok := resolveControlAuditOperatorUserID(ctx, a.users, record)
	if !ok {
		slog.Warn("control atomic audit skipped without operator row",
			"operator_id", record.OperatorID,
			"operation", record.Operation,
			"route", record.Route)
		return false, nil
	}
	operation := strings.TrimSpace(record.Operation)
	if operation == "" {
		slog.Warn("control atomic audit skipped without operation", "route", record.Route)
		return false, nil
	}
	params := controlAuditRecordParams(record, operatorID)
	if err := a.store.AppendRecordTx(ctx, tx, params); err != nil {
		return false, err
	}
	return true, nil
}
