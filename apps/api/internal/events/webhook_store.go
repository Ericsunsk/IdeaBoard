package events

// Signed webhook subscriptions (issue #807).
//
// Model: a workspace-scoped subscription carries a destination URL, an
// event-type filter (omitted = all frozen types), a signing secret, and
// delivery state. Reads never expose the secret plaintext: views carry
// has_secret only; the plaintext is returned exactly once at creation
// and only when server-minted.
//
// Delivery: POST the v1 envelope JSON with Svix-compatible signature
// headers (see webhook_crypto.go). Secrets travel in whsec_ form
// ("whsec_" + base64 of the raw key); the store keeps raw key bytes.
//
// Retry/backoff: one webhook_deliver River job per delivery with
// MaxAttempts (8). See webhook_delivery.go and webhook_river.go.
// Attempt history and dead-letter state are persisted by the injected
// WebhookPersistence adapter; the in-memory projection is only a cache
// for the HTTP surface.
//
// Provider inbound webhooks (Stripe) stay distinct integrations that
// emit internal events; they never flow through this dispatcher.

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/platform/contract"
)

const (
	webhookMaxAttempts        = 8
	webhookBackoffBase        = 30 * time.Second
	webhookBackoffCap         = time.Hour
	webhookTimestampTolerance = 5 * time.Minute
	webhookAttemptLogCap      = 20
	webhookDeadLetterCap      = 100
	webhookMaxSubscriptions   = 50
	webhookMinSecretLength    = 16
	webhookSecretPrefix       = "whsec_"
	svixIDHeader              = "svix-id"
	svixTimestampHeader       = "svix-timestamp"
	svixSignatureHeader       = "svix-signature"
	webhookSignatureVersion   = "v1"
	webhookSubscriptionPrefix = "sub_"
)

// Subscription delivery states. These words are the process-local
// projection owned by WebhookStore (see CanWebhookTransition), NOT the
// durable event_subscriptions.status column: that column stays 'active'
// (chk_event_subscriptions_status allows active/paused/disabled, but no
// Go writer emits paused/disabled and this store never persists
// degraded/dead_letter). The wire words are unchanged.
type WebhookStatus string

const (
	WebhookStatusActive     WebhookStatus = "active"
	WebhookStatusDegraded   WebhookStatus = "degraded"
	WebhookStatusDeadLetter WebhookStatus = "dead_letter"
)

// allowedWebhookTransitions mirrors the in-memory guards in
// webhook_delivery.go and postgres_store.go hydrateSubscription: a
// failed attempt degrades an active subscription; a delivered envelope
// with no remaining failures restores a degraded subscription to
// active; budget exhaustion parks any live subscription as
// dead_letter. dead_letter is terminal (Dispatch and
// executeDeliveryAttempt drop it). The type only documents the allowed
// transitions at compile time (see CanWebhookTransition); no SQL
// literals change.
var allowedWebhookTransitions = map[WebhookStatus]map[WebhookStatus]struct{}{
	WebhookStatusActive: {
		WebhookStatusDegraded:   {},
		WebhookStatusDeadLetter: {},
	},
	WebhookStatusDegraded: {
		WebhookStatusActive:     {},
		WebhookStatusDeadLetter: {},
	},
}

// CanWebhookTransition reports whether a from -> to flip is allowed in
// the process-local projection. It is documentation, not a gate.
func CanWebhookTransition(from, to WebhookStatus) bool {
	nexts, ok := allowedWebhookTransitions[from]
	if !ok {
		return false
	}
	_, ok = nexts[to]
	return ok
}

// AttemptOutcome is one per-attempt delivery fact. The wire and
// database words are unchanged ("delivered", "failed"); the type only
// documents the closed vocabulary at compile time. Attempts are
// INSERT-only facts, not a lifecycle: there is no transition table.
type AttemptOutcome string

const (
	AttemptOutcomeDelivered AttemptOutcome = "delivered"
	AttemptOutcomeFailed    AttemptOutcome = "failed"
)

// DeliveryStatus is the durable event_deliveries.status word. The wire
// and database words are unchanged
// ("pending","retrying","delivered","failed","dead_letter"); the type
// only documents the allowed transitions at compile time (see
// CanDeliveryTransition). SQL literals stay byte-identical.
//
// 'failed' is a legacy read-only word: the current RecordAttempt only
// writes retrying/delivered/dead_letter (a non-final failure stays
// retrying; the per-attempt outcome carries failed), but hydrate still
// honors a stored 'failed' delivery by degrading the subscription, so
// the word stays in the vocabulary.
type DeliveryStatus string

const (
	DeliveryStatusPending    DeliveryStatus = "pending"
	DeliveryStatusRetrying   DeliveryStatus = "retrying"
	DeliveryStatusDelivered  DeliveryStatus = "delivered"
	DeliveryStatusFailed     DeliveryStatus = "failed"
	DeliveryStatusDeadLetter DeliveryStatus = "dead_letter"
)

// allowedDeliveryTransitions mirrors the durable writes in
// postgres_store.go (EnsureDelivery inserts pending; RecordAttempt
// flips to retrying/delivered/dead_letter; MarkDelivered flips to
// delivered) and chk_event_deliveries_status: pending fans out to
// retrying/delivered/dead_letter; retrying repeats until delivered or
// dead_letter; delivered/dead_letter/failed are terminal (failed is
// legacy read-only and has no writer).
var allowedDeliveryTransitions = map[DeliveryStatus]map[DeliveryStatus]struct{}{
	DeliveryStatusPending: {
		DeliveryStatusRetrying:   {},
		DeliveryStatusDelivered:  {},
		DeliveryStatusDeadLetter: {},
	},
	DeliveryStatusRetrying: {
		DeliveryStatusRetrying:   {},
		DeliveryStatusDelivered:  {},
		DeliveryStatusDeadLetter: {},
	},
}

// CanDeliveryTransition reports whether a from -> to flip is durably
// allowed. It is documentation, not a gate: every transition it
// reports true for was already possible, and no previously possible
// transition reports false.
func CanDeliveryTransition(from, to DeliveryStatus) bool {
	nexts, ok := allowedDeliveryTransitions[from]
	if !ok {
		return false
	}
	_, ok = nexts[to]
	return ok
}

// WebhookSubscription is the stored subscription (secret never serialized).
type WebhookSubscription struct {
	ID          string
	WorkspaceID string
	URL         string
	EventTypes  []string
	Secret      []byte
	Status      WebhookStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Delivered   map[string]struct{}
	Attempts    []WebhookAttempt
	DeadLetter  []WebhookDeadLetter
}

// WebhookAttempt is one delivery attempt fact.
type WebhookAttempt struct {
	DeliveryID string         `json:"delivery_id"`
	Attempt    int            `json:"attempt"`
	StatusCode int            `json:"status_code"`
	Outcome    AttemptOutcome `json:"outcome"`
	Error      string         `json:"error,omitempty"`
	At         time.Time      `json:"at"`
}

// WebhookDeadLetter is an envelope parked after budget exhaustion.
type WebhookDeadLetter struct {
	DeliveryID string    `json:"delivery_id"`
	EventType  string    `json:"event_type"`
	Attempts   int       `json:"attempts"`
	LastError  string    `json:"last_error,omitempty"`
	At         time.Time `json:"at"`
}

// WebhookView is the masked read projection (no secret material).
type WebhookView struct {
	ID             string              `json:"id"`
	WorkspaceID    string              `json:"workspace_id"`
	URL            string              `json:"url"`
	EventTypes     []string            `json:"event_types"`
	HasSecret      bool                `json:"has_secret"`
	Status         string              `json:"status"`
	FailureCount   int                 `json:"failure_count"`
	DeadLetterSize int                 `json:"dead_letter_size"`
	RecentAttempts []WebhookAttempt    `json:"recent_attempts"`
	DeadLetter     []WebhookDeadLetter `json:"dead_letter"`
	CreatedAt      string              `json:"created_at"`
	UpdatedAt      string              `json:"updated_at"`
}

// WebhookSender posts one attempt. Production uses HTTPSender;
// tests inject fakes.
type WebhookSender interface {
	Send(ctx context.Context, destination string, headers map[string]string, body []byte) (int, error)
}

// WebhookPersistence is the durable subscription and delivery boundary.
type WebhookPersistence interface {
	CreateSubscription(context.Context, *WebhookSubscription) error
	DeleteSubscription(context.Context, string, string) error
	LoadSubscriptions(context.Context) ([]*WebhookSubscription, error)
	LoadSubscription(context.Context, string) (*WebhookSubscription, error)
	EnsureDelivery(context.Context, V1Envelope, string) error
	PendingDeliveries(context.Context) ([]WebhookDeliveryArgs, error)
	RecordAttempt(context.Context, string, V1Envelope, WebhookAttempt, bool) error
	MarkDelivered(context.Context, string, V1Envelope) error
}

// WebhookStore owns subscriptions + dispatch for one process.
// Deliveries are scheduled through the injected enqueuer (production:
// the shared River client; tests: fakes). A nil enqueuer drops
// dispatches with a warning, mirroring the nil-EventBus seam.
type WebhookStore struct {
	mu                 sync.Mutex
	subs               map[string]*WebhookSubscription
	byWorkspace        map[string][]string
	pendingByWorkspace map[string]int
	sender             WebhookSender
	enqueuer           WebhookEnqueuer
	persistence        WebhookPersistence
	now                func() time.Time
}

// NewWebhookStore builds a store dispatching via sender and enqueuer.
func NewWebhookStore(sender WebhookSender, enqueuer WebhookEnqueuer) *WebhookStore {
	return &WebhookStore{
		subs: map[string]*WebhookSubscription{}, byWorkspace: map[string][]string{},
		pendingByWorkspace: map[string]int{},
		sender:             sender, enqueuer: enqueuer, now: time.Now,
	}
}

// SetPersistence attaches the durable adapter at the composition root.
func (s *WebhookStore) SetPersistence(persistence WebhookPersistence) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.persistence = persistence
}

// LoadFromPersistence refreshes the process-local projection after startup.
func (s *WebhookStore) LoadFromPersistence(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	persistence := s.persistence
	s.mu.Unlock()
	if persistence == nil {
		return nil
	}
	subs, err := persistence.LoadSubscriptions(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sub := range subs {
		if sub == nil {
			continue
		}
		if sub.Delivered == nil {
			sub.Delivered = map[string]struct{}{}
		}
		s.subs[sub.ID] = sub
		found := false
		for _, id := range s.byWorkspace[sub.WorkspaceID] {
			if id == sub.ID {
				found = true
				break
			}
		}
		if !found {
			s.byWorkspace[sub.WorkspaceID] = append(s.byWorkspace[sub.WorkspaceID], sub.ID)
		}
	}
	return nil
}

// SetEnqueuer attaches the River-backed enqueuer after construction.
// The composition root uses it because the River client needs the
// worker at registration while the worker needs this store first.
func (s *WebhookStore) SetEnqueuer(enqueuer WebhookEnqueuer) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enqueuer = enqueuer
}

// SetClockForTests injects a deterministic clock.
func (s *WebhookStore) SetClockForTests(now func() time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

func (s *WebhookStore) clock() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

// ValidateWebhookURL accepts https everywhere and http only for
// loopback/.test (dev + tests). Anything else fails closed.
func ValidateWebhookURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fmt.Errorf("events: webhook url is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("events: webhook url is invalid")
	}
	host := strings.ToLower(parsed.Hostname())
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return nil
	case "http":
		if host == "localhost" || host == "127.0.0.1" || host == "::1" ||
			strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".test") {
			return nil
		}
		return fmt.Errorf("events: webhook url must be https (http allowed only for loopback/.test)")
	default:
		return fmt.Errorf("events: webhook url must be https")
	}
}

// IsWebhookSubscriptionID validates the sub_ shape.
func IsWebhookSubscriptionID(id string) bool {
	return contract.IsValidPublicID(webhookSubscriptionPrefix, strings.TrimSpace(id))
}

// Create registers a subscription. Empty secret mints one (returned once
// via minted=true); provided secrets must be >= 16 chars. Empty
// eventTypes means all frozen types.
//
// The durable write happens outside the mutex: the registry check is
// copied out under lock, persistence runs unlocked, then the projection
// commit re-locks.
func (s *WebhookStore) Create(ctx context.Context, workspaceID, destination string, eventTypes []string, secret []byte) (*WebhookSubscription, []byte, bool, error) {
	if s == nil {
		return nil, nil, false, fmt.Errorf("events: webhook store unavailable")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if !contract.IsValidPublicID(contract.PrefixWorkspace, workspaceID) {
		return nil, nil, false, fmt.Errorf("events: workspace_id must be ws_ opaque")
	}
	if err := ValidateWebhookURL(destination); err != nil {
		return nil, nil, false, err
	}
	normalized := make([]string, 0, len(eventTypes))
	for _, typ := range eventTypes {
		trimmed := strings.TrimSpace(typ)
		if trimmed == "" {
			continue
		}
		if !IsV1EventType(trimmed) {
			return nil, nil, false, fmt.Errorf("events: unknown event type filter %q", trimmed)
		}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		normalized = V1EventTypes()
	}
	minted := false
	if len(secret) == 0 {
		generated, err := mintWebhookSecret()
		if err != nil {
			return nil, nil, false, err
		}
		secret = generated
		minted = true
	} else if err := validateWebhookSecret(secret); err != nil {
		return nil, nil, false, err
	}
	if !minted {
		decoded, _ := ParseWhsecSecret(secret)
		secret = decoded
	}
	id, err := contract.MintPublicID(webhookSubscriptionPrefix, 12)
	if err != nil {
		return nil, nil, false, err
	}
	now := s.clock()
	storedSecret := append([]byte(nil), secret...)
	sub := &WebhookSubscription{
		ID: id, WorkspaceID: workspaceID, URL: strings.TrimSpace(destination),
		EventTypes: append([]string(nil), normalized...), Secret: storedSecret,
		Status: WebhookStatusActive, CreatedAt: now, UpdatedAt: now,
		Delivered: map[string]struct{}{},
	}
	s.mu.Lock()
	if len(s.byWorkspace[workspaceID])+s.pendingByWorkspace[workspaceID] >= webhookMaxSubscriptions {
		s.mu.Unlock()
		return nil, nil, false, fmt.Errorf("events: too many webhook subscriptions for workspace")
	}
	s.pendingByWorkspace[workspaceID]++
	persistence := s.persistence
	s.mu.Unlock()
	if persistence != nil {
		if err := persistence.CreateSubscription(ctx, sub); err != nil {
			s.mu.Lock()
			s.pendingByWorkspace[workspaceID]--
			if s.pendingByWorkspace[workspaceID] == 0 {
				delete(s.pendingByWorkspace, workspaceID)
			}
			s.mu.Unlock()
			return nil, nil, false, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingByWorkspace[workspaceID]--
	if s.pendingByWorkspace[workspaceID] == 0 {
		delete(s.pendingByWorkspace, workspaceID)
	}
	s.subs[id] = sub
	s.byWorkspace[workspaceID] = append(s.byWorkspace[workspaceID], id)
	if minted {
		return sub, append([]byte(nil), secret...), true, nil
	}
	return sub, nil, false, nil
}

// Get returns the subscription when it belongs to the workspace.
func (s *WebhookStore) Get(workspaceID, id string) (*WebhookSubscription, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[strings.TrimSpace(id)]
	if !ok || sub.WorkspaceID != strings.TrimSpace(workspaceID) {
		return nil, false
	}
	return sub, true
}

// List returns masked views for the workspace.
func (s *WebhookStore) List(workspaceID string) []WebhookView {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	views := []WebhookView{}
	for _, id := range s.byWorkspace[strings.TrimSpace(workspaceID)] {
		if sub, ok := s.subs[id]; ok {
			views = append(views, maskSubscription(sub))
		}
	}
	return views
}

// Delete removes the subscription when it belongs to the workspace.
// The durable delete happens outside the mutex: ownership is checked
// under lock, persistence runs unlocked, then the projection commit
// re-locks.
func (s *WebhookStore) Delete(ctx context.Context, workspaceID, id string) (bool, error) {
	if s == nil {
		return false, nil
	}
	workspaceID = strings.TrimSpace(workspaceID)
	s.mu.Lock()
	sub, ok := s.subs[strings.TrimSpace(id)]
	if !ok || sub.WorkspaceID != workspaceID {
		s.mu.Unlock()
		return false, nil
	}
	subscriptionID := sub.ID
	persistence := s.persistence
	s.mu.Unlock()
	if persistence != nil {
		if err := persistence.DeleteSubscription(ctx, workspaceID, subscriptionID); err != nil {
			return false, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.subs[subscriptionID]; !ok || current.WorkspaceID != workspaceID {
		return false, nil
	}
	delete(s.subs, subscriptionID)
	kept := s.byWorkspace[workspaceID][:0]
	for _, candidate := range s.byWorkspace[workspaceID] {
		if candidate != subscriptionID {
			kept = append(kept, candidate)
		}
	}
	s.byWorkspace[workspaceID] = kept
	return true, nil
}

func maskSubscription(sub *WebhookSubscription) WebhookView {
	failures := 0
	for _, attempt := range sub.Attempts {
		if attempt.Outcome != AttemptOutcomeDelivered {
			failures++
		}
	}
	return WebhookView{
		ID: sub.ID, WorkspaceID: sub.WorkspaceID, URL: sub.URL,
		EventTypes: append([]string(nil), sub.EventTypes...), HasSecret: len(sub.Secret) > 0,
		Status: string(sub.Status), FailureCount: failures, DeadLetterSize: len(sub.DeadLetter),
		RecentAttempts: append([]WebhookAttempt(nil), sub.Attempts...),
		DeadLetter:     append([]WebhookDeadLetter(nil), sub.DeadLetter...),
		CreatedAt:      sub.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      sub.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// Matches reports whether the subscription wants the envelope type.
func (sub *WebhookSubscription) Matches(typ string) bool {
	if sub == nil {
		return false
	}
	if len(sub.EventTypes) == 0 {
		return true
	}
	for _, want := range sub.EventTypes {
		if want == strings.TrimSpace(typ) {
			return true
		}
	}
	return false
}

func appendIfMissing(ids []string, id string) []string {
	for _, candidate := range ids {
		if candidate == id {
			return ids
		}
	}
	return append(ids, id)
}

func failureCount(sub *WebhookSubscription) int {
	count := 0
	for _, attempt := range sub.Attempts {
		if attempt.Outcome != AttemptOutcomeDelivered {
			count++
		}
	}
	return count
}
