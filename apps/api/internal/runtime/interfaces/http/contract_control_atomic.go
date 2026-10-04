package httpapi

// Control Plane atomic audit ports (step 2: audit joins the business
// transaction instead of dual-writing after it).
//
// The control handlers own no transaction: every business write commits
// inside a domain-owned transaction (identity commands, the credits
// ledger, task settlement, settings, tenancy), so no single handler can
// be re-threaded onto one shared transaction without touching those
// owning contexts. Step 2 therefore introduces ONE unified transaction
// scope at the guard (the single choke point every guarded mutation
// flows through) plus a participant-form audit writer that joins the
// caller-owned transaction instead of opening its own detached one.
//
// ISOLATION: these are ports only (interfaces plus a context carrier).
// This file imports no durable, audit-store, or delivery package, which
// is what TestControlNeverTouchesEventLayer guards. Composition
// provides the pgxpool-backed implementations; the guard only programs
// against these seams.
//
// SEMANTIC INVERSION (versus step 1): the step-1 durable sink is an
// observer (failures are warn-logged and swallowed so the business call
// can never fail because of its audit trail). The participant writer
// below is a co-committer: when it reports an error the guard MUST roll
// back the whole transaction and fail the request, because committing
// the business fact without its audit row would re-open the exact gap
// step 2 closes. An audit failure failing the business call is the
// definition of atomic here, not a bug.

import (
	"bytes"
	"context"
	"net/http"

	"ideaboard/api/internal/db/dbutil"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// ContextWithControlTx returns ctx carrying tx for the current guarded
// mutation. Composition's scope implementation is the only production
// caller; tests use it to stage a transaction without a pool. It
// delegates to the shared dbutil carrier so domain participants (which
// cannot import this package) observe the same transaction.
func ContextWithControlTx(ctx context.Context, tx pgx.Tx) context.Context {
	return dbutil.ContextWithAmbientTx(ctx, tx)
}

// ControlTxFromContext extracts the guard-owned transaction, if any.
// The second return is false when the call runs outside an atomic
// guard scope (legacy path, reads, fail-fast denials). It delegates to
// the shared dbutil carrier, so a transaction staged here is visible to
// every step-3 domain participant, and vice versa.
func ControlTxFromContext(ctx context.Context) (pgx.Tx, bool) {
	return dbutil.AmbientTxFromContext(ctx)
}

// ControlTxScope owns one transaction per guarded mutation call: the
// guard begins it before the handler runs and commits (or rolls back)
// after the audit participant joined it. Implementations live in
// composition over the shared pool; nil (unwired) keeps the legacy
// detached audit path.
type ControlTxScope interface {
	// Begin opens the mutation transaction and returns the request
	// context carrying it (see ContextWithControlTx).
	Begin(ctx context.Context) (context.Context, error)
	// Commit commits the mutation transaction (business effects, if
	// any participant joined, plus the audit row) as one unit.
	Commit(ctx context.Context) error
	// Rollback discards the mutation transaction, including its audit
	// row. It never reports an error: rollback is best-effort cleanup
	// after the request already failed for another reason.
	Rollback(ctx context.Context)
}

// ControlAuditTxWriter persists one guarded Control Plane call as a
// PARTICIPANT of the guard-owned transaction: it never begins or
// commits, so the audit row shares the fate of the mutation. Error
// semantics are inverted versus the step-1 observer sink: a non-nil
// error means the guard must roll back and fail the request.
//
// The bool reports whether a durable row joined the transaction.
// False with a nil error means "slog trail only" (service subjects
// without a users row, or an empty operation): the guard commits the
// transaction and the business outcome stands, exactly like the step-1
// skip. Only a non-nil error rolls the business call back.
type ControlAuditTxWriter interface {
	AppendControlAuditTx(ctx context.Context, tx pgx.Tx, record ControlAuditRecord) (durable bool, err error)
}

// msgServiceUnavailableAtomic is the operator-facing copy when the
// atomic audit join fails: the business call executed but its audit
// row could not join the transaction, so the whole mutation rolled
// back. It lives here (not in message.go) because step 2 may only
// touch contract_control*.go inside this package.
const msgServiceUnavailableAtomic = "审计写入失败，操作已回滚" // CodeServiceUnavailable

// controlAtomicCaptureWriter buffers a guarded mutation response so
// the guard can still fail the request AFTER the handler rendered:
// unlike controlCaptureWriter (which passes bytes through to the
// client immediately), nothing reaches the wire until the guard commits
// the mutation transaction and replays the buffer. That deferral is
// what makes "audit failure rolls the business call back" honest at
// the HTTP layer: with pass-through the 200 status would already be on
// the wire before the audit outcome is known.
type controlAtomicCaptureWriter struct {
	gin.ResponseWriter
	header      http.Header
	status      int
	wroteHeader bool
	body        bytes.Buffer
}

// Header exposes the isolated buffer headers to the handler. Legacy
// transport headers never leak: only the replay step copies them to
// the real writer after commit.
func (w *controlAtomicCaptureWriter) Header() http.Header {
	if w == nil {
		return http.Header{}
	}
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

// WriteHeader records the status without touching the wire. The status
// defaults to 200 on first Write, mirroring net/http.
func (w *controlAtomicCaptureWriter) WriteHeader(status int) {
	if w == nil || w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
}

// Write buffers the body without touching the wire.
func (w *controlAtomicCaptureWriter) Write(data []byte) (int, error) {
	if w == nil {
		return 0, nil
	}
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(data)
}

// WriteString buffers the string form without touching the wire (the
// contract render seam may use it).
func (w *controlAtomicCaptureWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// replayTo commits the buffered response onto dst after the mutation
// transaction committed: status, headers (multi-values preserved, e.g.
// impersonation cookies), then body.
func (w *controlAtomicCaptureWriter) replayTo(dst gin.ResponseWriter) {
	if w == nil || dst == nil {
		return
	}
	header := dst.Header()
	for key, values := range w.header {
		for _, value := range values {
			header.Add(key, value)
		}
	}
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	dst.WriteHeader(status)
	_, _ = dst.Write(w.body.Bytes())
}
