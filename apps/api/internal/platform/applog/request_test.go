package applog

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func scopedTestLogger(logs *bytes.Buffer) {
	slog.SetDefault(slog.New(slog.NewJSONHandler(logs, nil)))
}

func logScoped(t *testing.T, ctx context.Context, enrich func(context.Context) context.Context, msg string) map[string]any {
	t.Helper()
	var logs bytes.Buffer
	previous := slog.Default()
	scopedTestLogger(&logs)
	t.Cleanup(func() { slog.SetDefault(previous) })
	if enrich != nil {
		ctx = enrich(ctx)
	}
	FromContext(ctx).Info(msg)
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
		t.Fatalf("scoped log is not JSON: %v; logs=%q", err, logs.String())
	}
	return entry
}

func TestContextWithRequestIDSeedsScopedLogger(t *testing.T) {
	entry := logScoped(t, context.Background(), func(ctx context.Context) context.Context {
		return ContextWithRequestID(ctx, "req-1")
	}, "hello")
	if entry["request_id"] != "req-1" || entry["msg"] != "hello" {
		t.Fatalf("entry = %v, want request_id req-1", entry)
	}
}

func TestFromContextFallsBackToDefault(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{name: "background", ctx: context.Background()},
		{name: "nil", ctx: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if FromContext(tc.ctx) != slog.Default() {
				t.Fatal("context without a scoped logger should return slog.Default")
			}
		})
	}
}

func TestContextWithPrincipalAddsIdentityOnlyWhenResolved(t *testing.T) {
	ctx := ContextWithRequestID(context.Background(), "req-2")

	entry := logScoped(t, ctx, func(ctx context.Context) context.Context {
		return ContextWithPrincipal(ctx, 0, "ws_abc")
	}, "unresolved")
	if _, ok := entry["user_id"]; ok {
		t.Fatalf("zero userID must not attach identity: %v", entry)
	}
	if entry["request_id"] != "req-2" {
		t.Fatalf("scope must survive no-op enrichment: %v", entry)
	}

	entry = logScoped(t, ctx, func(ctx context.Context) context.Context {
		return ContextWithPrincipal(ctx, 42, "")
	}, "user-only")
	if entry["user_id"] != float64(42) || entry["request_id"] != "req-2" {
		t.Fatalf("entry = %v, want user_id 42 with request scope kept", entry)
	}
	if _, ok := entry["workspace_id"]; ok {
		t.Fatalf("blank workspace must stay omitted: %v", entry)
	}

	entry = logScoped(t, ctx, func(ctx context.Context) context.Context {
		return ContextWithPrincipal(ctx, 42, "ws_abc")
	}, "scoped")
	if entry["user_id"] != float64(42) || entry["workspace_id"] != "ws_abc" {
		t.Fatalf("entry = %v, want user_id and workspace_id", entry)
	}
}

func TestReseedKeepsEnrichment(t *testing.T) {
	entry := logScoped(t, context.Background(), func(ctx context.Context) context.Context {
		ctx = ContextWithRequestID(ctx, "req-3")
		ctx = ContextWithPrincipal(ctx, 7, "ws_x")
		return ContextWithRequestID(ctx, "req-3")
	}, "reseeded")
	if entry["request_id"] != "req-3" || entry["user_id"] != float64(7) || entry["workspace_id"] != "ws_x" {
		t.Fatalf("same-id re-seed must keep enrichment: %v", entry)
	}
}

func TestRequestIDAccessor(t *testing.T) {
	if RequestID(context.Background()) != "" {
		t.Fatal("bare context request id should be empty")
	}
	ctx := ContextWithRequestID(context.Background(), "req-4")
	if RequestID(ctx) != "req-4" {
		t.Fatalf("RequestID = %q, want req-4", RequestID(ctx))
	}
}
