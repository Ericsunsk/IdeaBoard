package applog

import (
	"context"
	"log/slog"
)

// Request-scoped logging.
//
// Every HTTP request carries a scope in its context, seeded by the
// request-ID middleware with the base field:
//
//	request_id — minted or echoed by the request-ID middleware, always present.
//
// Where the principal is already resolved (ResolvePrincipal runs AFTER
// session auth, so never in the global middleware), the scope is enriched
// with:
//
//	user_id      — numeric operator id, only when a principal is present.
//	workspace_id — ws_ tenant scope, only when the request is workspace-scoped.
//
// Middleware order matters: request-ID seeding must precede principal
// enrichment. Re-seeding the same request_id keeps the existing scope so
// enrichment is never dropped by a second (e.g. contract) request-ID
// middleware.
//
// The scope stores attributes, not a logger: FromContext builds the logger
// from slog.Default() on every call, so late Default swaps (tests,
// reconfiguration) keep working and no handler is pinned at seed time.
//
// Level policy (HTTP edge):
//
//	4xx (caller mistakes: validation, not-found, conflict, throttle) render
//	without logging — the wire envelope is the signal.
//	Unmatched/default failures (5xx) log at Error with domain-supplied safe
//	messages and redacted attribution; never at Warn.
//	Misconfiguration (an error-table row missing its safe message) logs at
//	Error — it is a server bug, not caller noise.
//	Audit and access trails log at Info with request_id from the scope.
//
// Workers keep their task_id+correlation_id pattern untouched; this scope
// is for request-bound logs only.
type requestScope struct {
	requestID string
	attrs     []any
}

type requestScopeKey struct{}

// ContextWithRequestID returns ctx carrying a request scope for requestID.
// A blank id or nil ctx leaves ctx unchanged. Seeding an id that already
// scopes ctx returns ctx unchanged so a re-seed never drops previously
// attached identity fields.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil || requestID == "" {
		return ctx
	}
	if scope, ok := ctx.Value(requestScopeKey{}).(requestScope); ok && scope.requestID == requestID {
		return ctx
	}
	return context.WithValue(ctx, requestScopeKey{}, requestScope{requestID: requestID})
}

// ContextWithPrincipal enriches the scope with user_id and, when
// non-blank, workspace_id. Call only where the principal is already
// resolved (post-auth middleware, authenticated handlers). A non-positive
// userID leaves ctx unchanged and never clears the existing scope.
func ContextWithPrincipal(ctx context.Context, userID int64, workspaceID string) context.Context {
	if ctx == nil || userID <= 0 {
		return ctx
	}
	var scope requestScope
	if existing, ok := ctx.Value(requestScopeKey{}).(requestScope); ok {
		scope = existing
	}
	attrs := make([]any, 0, len(scope.attrs)+4)
	attrs = append(attrs, scope.attrs...)
	attrs = append(attrs, "user_id", userID)
	if workspaceID != "" {
		attrs = append(attrs, "workspace_id", workspaceID)
	}
	scope.attrs = attrs
	return context.WithValue(ctx, requestScopeKey{}, scope)
}

// FromContext returns a logger carrying the request scope, or slog.Default
// when ctx carries no scope (workers, bare tests, requests that bypassed
// the request-ID middleware).
func FromContext(ctx context.Context) *slog.Logger {
	logger := slog.Default()
	if ctx == nil {
		return logger
	}
	scope, ok := ctx.Value(requestScopeKey{}).(requestScope)
	if !ok {
		return logger
	}
	args := make([]any, 0, len(scope.attrs)+2)
	if scope.requestID != "" {
		args = append(args, "request_id", scope.requestID)
	}
	args = append(args, scope.attrs...)
	if len(args) == 0 {
		return logger
	}
	return logger.With(args...)
}

// RequestID returns the scoped request id, or "" when ctx carries no scope.
func RequestID(ctx context.Context) string {
	if ctx != nil {
		if scope, ok := ctx.Value(requestScopeKey{}).(requestScope); ok {
			return scope.requestID
		}
	}
	return ""
}
