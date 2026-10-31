package webhook

// Cross-stack parity for the frozen workspace event registry.
//
// Three places must stay in sync; a new event type touches all three or the
// suites below go red:
//   - apps/api/internal/events/envelope.go (V1EventTypes, the frozen table)
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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
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
		if fileExists(filepath.Join(dir, "apps", "api", "internal", "events", "envelope.go")) &&
			(fileExists(filepath.Join(dir, "apps", "web", "src", "api", "realtime", "workspaceEventStream.ts")) ||
				fileExists(filepath.Join(dir, "apps", "web", "src", "api", "workspaceEventStream.ts"))) {
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

// parseFrontendEventTypes expands literal arrays and named spreads in the
// frontend registry. Unsupported shapes fail closed rather than silently
// omitting part of the vocabulary.
func parseFrontendEventTypes(source string) ([]string, error) {
	visiting := make(map[string]bool)
	literal := regexp.MustCompile(`^'([^']+)'$`)
	spread := regexp.MustCompile(`^\.\.\.([A-Z_][A-Z_0-9]*)$`)
	var expand func(string) ([]string, error)
	expand = func(name string) ([]string, error) {
		if visiting[name] {
			return nil, fmt.Errorf("cyclic event array spread: %s", name)
		}
		declaration := regexp.MustCompile(`(?s)export\s+const\s+` + regexp.QuoteMeta(name) + `\s*=\s*\[(.*?)\]\s+as\s+const`)
		m := declaration.FindStringSubmatch(source)
		if len(m) < 2 {
			return nil, fmt.Errorf("%s literal array declaration not found", name)
		}
		visiting[name] = true
		defer delete(visiting, name)
		var out []string
		items := strings.Split(strings.TrimSpace(m[1]), ",")
		for i, item := range items {
			item = strings.TrimSpace(item)
			if item == "" && i == len(items)-1 {
				continue // Empty array or trailing comma.
			}
			if value := literal.FindStringSubmatch(item); value != nil {
				out = append(out, value[1])
			} else if reference := spread.FindStringSubmatch(item); reference != nil {
				values, err := expand(reference[1])
				if err != nil {
					return nil, err
				}
				out = append(out, values...)
			} else {
				return nil, fmt.Errorf("unsupported %s event array element: %q", name, item)
			}
		}
		return out, nil
	}
	out, err := expand("WORKSPACE_EVENT_TYPES")
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("WORKSPACE_EVENT_TYPES parsed to an empty set")
	}
	return out, nil
}

func TestParseFrontendEventTypes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		want   []string
	}{
		{"named spread", `export const HINTS = ['notification.created', 'notification.updated'] as const
export const WORKSPACE_EVENT_TYPES = ['task.created', ...HINTS,] as const`, []string{"task.created", "notification.created", "notification.updated"}},
		{"missing spread", `export const WORKSPACE_EVENT_TYPES = ['task.created', ...MISSING] as const`, nil},
		{"cyclic spread", `export const WORKSPACE_EVENT_TYPES = [...WORKSPACE_EVENT_TYPES] as const`, nil},
		{"unsupported expression", `export const WORKSPACE_EVENT_TYPES = ['task.created', makeTypes()] as const`, nil},
		{"empty registry", `export const WORKSPACE_EVENT_TYPES = [] as const`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseFrontendEventTypes(tc.source)
			if tc.want == nil {
				if err == nil {
					t.Fatalf("parseFrontendEventTypes() = %v, want error", got)
				}
				return
			}
			if err != nil || !slices.Equal(got, tc.want) {
				t.Fatalf("parseFrontendEventTypes() = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestWorkspaceEventRegistryMatchesFrontend(t *testing.T) {
	root := findEventParityRepoRoot(t)
	streamPath := filepath.Join(root, "apps", "web", "src", "api", "realtime", "workspaceEventStream.ts")
	if !fileExists(streamPath) {
		streamPath = filepath.Join(root, "apps", "web", "src", "api", "workspaceEventStream.ts")
	}
	raw, err := os.ReadFile(streamPath)
	if err != nil {
		t.Fatalf("read frontend event stream: %v", err)
	}
	frontend, err := parseFrontendEventTypes(string(raw))
	if err != nil {
		t.Fatalf("parse frontend event registry: %v", err)
	}
	backend := events.V1EventTypes()
	if len(backend) != 12 {
		t.Fatalf("backend registry has %d types, want 12 (update envelope.go, workspaceEventStream.ts, and the webhook allowlist together)", len(backend))
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
