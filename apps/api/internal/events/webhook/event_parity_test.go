package webhook

// Cross-stack parity for the frozen workspace event registry.
//
// Three places must stay in sync; a new event type touches all three or the
// suites below go red:
//   - apps/api/internal/events/v1_envelope.go (V1EventTypes, the frozen table)
//   - apps/web/src/api/workspaceEventStream.ts (WORKSPACE_EVENT_TYPES)
//   - this package's webhook allowlist (IsWorkspaceWebhookEventType)
//
// This file is the machine gate: it reads the frontend type set from disk
// and compares it with the backend registry, so adding a type on either
// side without the other fails here (frontend CI pins the same literals in
// `workspaceEventStream.contract.spec.ts` for local feedback).
//
// Principal-scoped hints (board.access_changed, notification.created,
// notification.updated) are SSE/user-stream only and must never ride
// webhooks: subscription filters, dispatch enqueue, and delivery all fail
// them closed (see also TestWorkspaceWebhooksRejectProtectedSubscriptionsAndDeliveries).

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"testing"

	"ideaboard/api/internal/events"
)

func findEventParityRepoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(filename)
	for {
		if fileExists(filepath.Join(dir, "apps", "api", "internal", "events", "v1_envelope.go")) &&
			fileExists(filepath.Join(dir, "apps", "web", "src", "api", "workspaceEventStream.ts")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root not found from %s", filename)
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// extractFrontendEventTypes parses WORKSPACE_EVENT_TYPES from the frontend
// transport module. It asserts the declaration shape (array `as const`) so
// a refactor that moves the vocabulary fails loudly instead of comparing
// against an empty set.
func extractFrontendEventTypes(t *testing.T, source string) []string {
	t.Helper()
	m := regexp.MustCompile(`(?s)export\s+const\s+WORKSPACE_EVENT_TYPES\s*=\s*\[(.*?)\]\s+as\s+const`).FindStringSubmatch(source)
	if len(m) < 2 {
		t.Fatal("WORKSPACE_EVENT_TYPES declaration not found in workspaceEventStream.ts")
	}
	ms := regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(m[1], -1)
	out := make([]string, 0, len(ms))
	for _, x := range ms {
		out = append(out, x[1])
	}
	if len(out) == 0 {
		t.Fatal("WORKSPACE_EVENT_TYPES parsed to an empty set")
	}
	return out
}

func TestWorkspaceEventRegistryMatchesFrontend(t *testing.T) {
	root := findEventParityRepoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "apps", "web", "src", "api", "workspaceEventStream.ts"))
	if err != nil {
		t.Fatalf("read frontend event stream: %v", err)
	}
	frontend := extractFrontendEventTypes(t, string(raw))
	backend := events.V1EventTypes()
	if len(backend) != 12 {
		t.Fatalf("backend registry has %d types, want 12 (update v1_envelope.go, workspaceEventStream.ts, and the webhook allowlist together)", len(backend))
	}
	sortedFrontend := append([]string(nil), frontend...)
	sortedBackend := append([]string(nil), backend...)
	slices.Sort(sortedFrontend)
	slices.Sort(sortedBackend)
	if !slices.Equal(sortedFrontend, sortedBackend) {
		t.Fatalf("event registry mismatch\nbackend=%v\nfrontend=%v", sortedBackend, sortedFrontend)
	}
}

func TestWorkspaceWebhookAllowlistIsPinned(t *testing.T) {
	// The allowlist is exactly one type: checkout-scoped billing.settled.
	// Any widening must update the frontend parity pin
	// (workspaceEventStream.contract.spec.ts) and the frozen registry first.
	var allowed []string
	for _, typ := range events.V1EventTypes() {
		if IsWorkspaceWebhookEventType(typ) {
			allowed = append(allowed, typ)
		}
	}
	if !slices.Equal(allowed, []string{events.EventV1BillingSettled}) {
		t.Fatalf("webhook allowlist=%v, want [%q]", allowed, events.EventV1BillingSettled)
	}
	// Principal-scoped hints are SSE/user-stream only: they must never
	// become webhook-allowlisted, even as the frozen registry grows.
	for _, typ := range []string{
		events.EventV1BoardAccessChanged,
		events.EventV1NotificationCreated,
		events.EventV1NotificationUpdated,
	} {
		if IsWorkspaceWebhookEventType(typ) {
			t.Fatalf("principal-scoped hint %q must never be a webhook event type", typ)
		}
	}
	// The empty (default) filter resolves to the allowlist, not the full
	// registry.
	store := NewWebhookStore(&fakeWebhookSender{status: 200}, nil)
	sub, _, _, err := store.Create(t.Context(), "ws_1", "https://hooks.example.test/events", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sub.EventTypes, []string{events.EventV1BillingSettled}) {
		t.Fatalf("default subscription types=%v, want [%q]", sub.EventTypes, events.EventV1BillingSettled)
	}
	// Explicit principal-scoped subscriptions fail closed at Create.
	for _, typ := range []string{
		events.EventV1BoardAccessChanged,
		events.EventV1NotificationCreated,
		events.EventV1NotificationUpdated,
	} {
		if _, _, _, err := store.Create(t.Context(), "ws_1", "https://hooks.example.test/events", []string{typ}, nil); err == nil {
			t.Fatalf("principal-scoped subscription %q must be rejected", typ)
		}
	}
}
