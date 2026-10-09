package httpapi

// Control Plane HTTP kernel (issue #808, backend slice).
//
// Additive only: this file and its control_* siblings mount a SEPARATE
// middleware chain and route group under /api/control/v1. Legacy
// /api/admin/* routes were removed in #809 (hard cut); their handler
// methods are reused under the control guard (dedicated control handlers
// plus controlProxy adapters).
//
// Separation guarantees (Baseline 2.2):
//   - Auth: same session transport as Public, but authorization is
//     admin-only (admin business role on a human session or an internal
//     worker; service accounts, API keys, and OAuth apps never pass).
//     Production sessions mint no control capabilities at login, so the
//     family capability strings below are audit labels only.
//   - Rate limits: a dedicated fixed-window bucket per operator, separate
//     from Public limits, rendered via contract.SetRateLimit.
//   - Audit: EVERY guarded route emits a ControlAuditRecord (operator,
//     target, capability, request_id, idempotency key, reason, outcome,
//     timestamp) to structured slog plus an optional in-memory sink for
//     tests. Control audit records NEVER enter the public event registry
//     (no event-bus import anywhere under these files; guarded by
//     TestV1RegistryExcludesControl and by construction).
//   - Idempotency: header-only Idempotency-Key on ALL mutating control
//     routes (no body-key fallback). The durable scope is
//     operator/operation/target/key with a canonical request hash
//     (see contract_control_idempotency.go): same key + same request
//     replays the first 2xx result (cross-process, restart-safe), same
//     key + different request is a 409 before any side effect runs.
//     The shared contract IdempotencyStore is the same-process fast
//     path only.
//   - Envelope: dedicated handlers render the contract success/error
//     envelope with Request-Id. The small set of platform-config
//     families (ai-gateway, storage, search providers, feature prompts,
//     monitor, brand-logo overrides, rewards) reuses the legacy admin
//     handler methods under the control guard so validation and provider
//     side effects stay single-sourced; the control-facing render layer
//     translates every proxied response into the contract envelope at
//     the boundary (parent #809; binary brand-logo and the SSE monitor
//     stream stay raw protocol exceptions). Auth, audit, idempotency,
//     and reason guarantees hold uniformly via the guard either way.
//
// Persistence: every guarded call is emitted as structured slog and is
// persisted to the append-only control_audit_records sink. High-risk
// mutations also leave their durable domain fact carrying actor + reason
// (credits ledger command metadata, identity admin command actor, tenancy
// membership actor, settings outbound-policy actor, and gateway/catalog
// actor ids). The audit sink is an observation record; domain tables remain
// the source of truth for business state.

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/applog"
	"ideaboard/api/internal/platform/audit"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Control family capabilities (frozen audit labels; mirrored in
// openapi.control.yaml x-capabilities).
const (
	ControlCapabilityUsers      = principal.ControlCapabilityUsers
	ControlCapabilityWorkspaces = principal.ControlCapabilityWorkspaces
	ControlCapabilityBilling    = principal.ControlCapabilityBilling
	ControlCapabilityCredits    = principal.ControlCapabilityCredits
	ControlCapabilityPayments   = principal.ControlCapabilityPayments
	ControlCapabilityPlatform   = principal.ControlCapabilityPlatform
	ControlCapabilityTasks      = principal.ControlCapabilityTasks
)

// ControlAuditOutcome classifies a guarded control call for the audit
// record (mirrored in openapi.control.yaml AuditRecord.outcome). These
// are aliases for the single platform/audit Outcome vocabulary: there is
// exactly one failure spelling (failure), shared with the durable store
// and the impersonation audit path.
const (
	ControlAuditRead   = audit.OutcomeRead
	ControlAuditOK     = audit.OutcomeSuccess
	ControlAuditDenied = audit.OutcomeDenied
	ControlAuditFailed = audit.OutcomeFailure
)

// OperatorReasonHeader carries the operator note for mutating control
// routes that reuse legacy handlers whose bodies have no reason field.
// Dedicated control handlers also accept reason/operator_note JSON body
// fields; the header is the uniform fallback either way.
const OperatorReasonHeader = "X-Operator-Reason"

const controlReasonContextKey = "control.operator_reason"

func setControlReason(c *gin.Context, reason string) {
	if c != nil {
		c.Set(controlReasonContextKey, strings.TrimSpace(reason))
	}
}

func controlReasonFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	raw, _ := c.Get(controlReasonContextKey)
	reason, _ := raw.(string)
	return strings.TrimSpace(reason)
}

// maxControlBodyBytes bounds control request bodies buffered by the guard
// for reason extraction and idempotency hashing. Control payloads are
// small operator JSON documents; larger uploads never belong here.
const maxControlBodyBytes = 64 << 10

// ControlAuditRecord is one guarded Control Plane call. Operator and
// target identifiers are public ids only (usr_*, ws_*, task_*, order
// numbers); internal numeric ids never appear here.
type ControlAuditRecord struct {
	OperatorType   string        `json:"operator_type"`
	OperatorID     string        `json:"operator_id"`
	TargetType     string        `json:"target_type,omitempty"`
	TargetID       string        `json:"target_id,omitempty"`
	Capability     string        `json:"capability"`
	Operation      string        `json:"operation"`
	Route          string        `json:"route"`
	RequestID      string        `json:"request_id"`
	IdempotencyKey string        `json:"idempotency_key,omitempty"`
	Reason         string        `json:"reason,omitempty"`
	Outcome        audit.Outcome `json:"outcome"`
	Replayed       bool          `json:"replayed,omitempty"`
	Status         int           `json:"status"`
	OccurredAt     time.Time     `json:"occurred_at"`
}

// ControlAuditSink receives audit records in tests. Production wires nil
// (structured slog is the sink).
type ControlAuditSink interface {
	AppendControlAudit(ControlAuditRecord)
}

// ControlAuditMemorySink is the test sink: an append-only in-memory log.
type ControlAuditMemorySink struct {
	mu      sync.Mutex
	records []ControlAuditRecord
}

// AppendControlAudit implements ControlAuditSink.
func (s *ControlAuditMemorySink) AppendControlAudit(record ControlAuditRecord) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
}

// Records returns a copy of the captured audit records.
func (s *ControlAuditMemorySink) Records() []ControlAuditRecord {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ControlAuditRecord, len(s.records))
	copy(out, s.records)
	return out
}

// emitControlAudit writes the record to structured slog and to the sink.
// It never touches the public event layer. request_id (plus user_id /
// workspace_id once ResolvePrincipal enriched the scope) rides the applog
// request scope from ctx, so it is not plumbed manually here.
func emitControlAudit(ctx context.Context, sink ControlAuditSink, record ControlAuditRecord) {
	attrs := []any{
		"operator_type", record.OperatorType,
		"operator_id", record.OperatorID,
		"capability", record.Capability,
		"operation", record.Operation,
		"route", record.Route,
		"outcome", record.Outcome,
		"status", record.Status,
	}
	if record.TargetType != "" {
		attrs = append(attrs, "target_type", record.TargetType, "target_id", record.TargetID)
	}
	if record.IdempotencyKey != "" {
		attrs = append(attrs, "idempotency_key", record.IdempotencyKey)
	}
	if record.Reason != "" {
		attrs = append(attrs, "reason", record.Reason)
	}
	if record.Replayed {
		attrs = append(attrs, "replayed", true)
	}
	applog.FromContext(ctx).InfoContext(ctx, "control audit", attrs...)
	if sink != nil {
		sink.AppendControlAudit(record)
	}
}

// controlOperatorIdentity projects the operator principal to public-only
// audit identity. Human operators use the usr_ public id; service
// subjects use their stable subject string. Numeric ids never leak.
func controlOperatorIdentity(p principal.Principal) (subjectType, subjectID string) {
	subjectType = string(p.SubjectType())
	switch p.SubjectType() {
	case principal.SubjectHumanSession:
		subjectID = "usr_" + strings.ReplaceAll(p.PublicID().String(), "-", "")
	default:
		subjectID = strings.TrimSpace(p.Subject())
	}
	return subjectType, subjectID
}

// extractControlReason returns the operator note for a mutating control
// call: the X-Operator-Reason header wins, otherwise the reason or
// operator_note JSON body field. Empty means missing.
func extractControlReason(c *gin.Context, body []byte) string {
	if c == nil {
		return ""
	}
	if reason := strings.TrimSpace(c.GetHeader(OperatorReasonHeader)); reason != "" {
		return reason
	}
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := contract.DecodeStrict(body, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"reason", "operator_note"} {
		if raw, ok := payload[key].(string); ok && strings.TrimSpace(raw) != "" {
			return strings.TrimSpace(raw)
		}
	}
	return ""
}

// bufferControlBody reads and restores the request body (mutations only)
// so the guard can hash it and extract the reason while handlers still
// see the full body. Oversized bodies fail closed with 400.
func bufferControlBody(c *gin.Context) ([]byte, bool) {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return nil, true
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, maxControlBodyBytes+1))
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest)
		return nil, false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if int64(len(raw)) > maxControlBodyBytes {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest2)
		return nil, false
	}
	return raw, true
}

// controlCaptureWriter buffers a handler response so the idempotency
// seam can replay first results on key reuse.
type controlCaptureWriter struct {
	gin.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *controlCaptureWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *controlCaptureWriter) Write(data []byte) (int, error) {
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

// controlRoute describes one guarded route for auth/audit purposes.
type controlRoute struct {
	// Operation is the stable idempotency + audit operation name
	// (e.g. "credits.grant").
	Operation string
	// Route is the method + path template for the audit record
	// (e.g. "POST /users/{user_id}/role").
	Route string
	// Mutation marks a guarded write: the header Idempotency-Key is always
	// required and the outcome is success/failed. The operator reason is
	// required unless ReasonOptional exempts the route (routine config
	// mutations only; fund/safety-affecting writes keep the default).
	Mutation bool
	// ReasonOptional exempts a Mutation from the operator-reason
	// requirement (secure by default: false means reason is enforced).
	// The header Idempotency-Key stays required, the audit record is
	// still emitted (with an empty reason when none was supplied), and
	// a supplied reason must still be well-formed. Set it only for
	// routine config mutations; fund/safety-affecting writes keep the
	// default and fail closed with 400 when the reason is missing.
	ReasonOptional bool
	// Detached keeps a Mutation on the detached guard path (reason +
	// header Idempotency-Key + L1 fast-path + durable winner election
	// through short independent transactions + observer audit) instead
	// of the atomic guard transaction. Set it for external side effects
	// that cannot join a database transaction by construction (Auth mail
	// calls, Stripe prepare/publish): the audit row stays an observer so
	// an audit failure cannot imply the external send was revoked, and
	// no long database transaction ever wraps the external call. The
	// durable claim commits before the external call and completes (2xx)
	// or abandons (known failure) after it, so retries replay
	// cross-process without re-sending. Failed settlement keeps uncertainty
	// instead of repeating an external effect.
	Detached bool
	// Target extracts the audit target (public ids only) from the
	// request and buffered body. Nil means no target.
	Target func(c *gin.Context, body []byte) (targetType, targetID string)
}

// controlTargetParam builds a target extractor from a path param.
func controlTargetParam(param, targetType string) func(*gin.Context, []byte) (string, string) {
	return func(c *gin.Context, _ []byte) (string, string) {
		raw := strings.TrimSpace(c.Param(param))
		if raw == "" {
			return "", ""
		}
		return targetType, raw
	}
}

// parseControlUserPublicID resolves a usr_ external id to the UUID the
// identity/credit/tenancy seams consume. Malformed shapes fail with 400;
// unknown users surface as 404 from the seams.
func parseControlUserPublicID(c *gin.Context, raw string) (uuid.UUID, bool) {
	parsed, err := tenancy.ParseUserPublicID(strings.TrimSpace(raw))
	if err != nil || parsed == uuid.Nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest9)
		return uuid.Nil, false
	}
	return parsed, true
}

// parseControlSKU validates a {sku} path param's presence. Empty shapes
// fail with 400; unknown SKUs surface as 404 from the catalog seams.
func parseControlSKU(c *gin.Context) (string, bool) {
	sku := strings.TrimSpace(c.Param("sku"))
	if sku == "" {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest5)
		return "", false
	}
	return sku, true
}

// parseControlPageCursor decodes the opaque cursor query param into T.
// An absent cursor yields nil (the caller applies its default); a
// malformed or predicate-rejected cursor fails with 400
// pagination_cursor_invalid. Callers provide cursor-specific validation.
func parseControlPageCursor[T any](c *gin.Context, valid func(*T) bool) (*T, bool) {
	raw := strings.TrimSpace(c.Query("cursor"))
	if raw == "" {
		return nil, true
	}
	decoded, err := contract.DecodeCursor[T](raw)
	if err != nil || decoded == nil || (valid != nil && !valid(decoded)) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, msgPaginationCursorInvalid)
		return nil, false
	}
	return decoded, true
}

// parseControlPageSize parses the page_size query param with the shared
// page-size dialect. Absent yields def; malformed fails with 400.
func parseControlPageSize(c *gin.Context, def int) (int, bool) {
	raw := strings.TrimSpace(c.Query("page_size"))
	if raw == "" {
		return def, true
	}
	parsed, err := contract.ParsePageSize(raw)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest13)
		return 0, false
	}
	return parsed, true
}

// controlReasonOr prefers the body-carried operator note, falling back
// to the guard-extracted reason (X-Operator-Reason header wins there).
func controlReasonOr(ctx controlContext, reason string) string {
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		return trimmed
	}
	return ctx.Reason
}

// nonEmptyControlMessage returns the trimmed message or the fallback
// when blank. Shared by the Auth-seam error mappers (mail, OAuth)
// whose errors.As dispatch stays at the call sites.
func nonEmptyControlMessage(message, fallback string) string {
	if trimmed := strings.TrimSpace(message); trimmed != "" {
		return trimmed
	}
	return fallback
}

// parseControlProviderSlug validates a {provider} path param's shape
// (lowercase slug, allowlisted charset, max length). Unknown or malformed
// slugs fail closed with 404 (invisible, never 403). Callers apply their
// own allowlist (static set or service capabilities) after this check.
func parseControlProviderSlug(c *gin.Context) (string, bool) {
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	if provider == "" || len(provider) > 64 {
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound6)
		return "", false
	}
	for _, character := range provider {
		if (character < 'a' || character > 'z') && character != '-' && character != '_' && (character < '0' || character > '9') {
			contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound6)
			return "", false
		}
	}
	return provider, true
}
