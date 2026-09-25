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
// headers: svix-id (the evt_ delivery id, stable across retries),
// svix-timestamp (unix seconds), and svix-signature ("v1,<base64>"
// HMAC-SHA256 over "<id>.<timestamp>.<raw-body>", space-delimited to
// allow rotation entries). Secrets travel in whsec_ form
// ("whsec_" + base64 of the raw key); the store keeps raw key bytes.
// Receivers accept timestamps within webhookTimestampTolerance
// (5 minutes).
//
// Retry/backoff: one webhook_deliver River job per delivery with
// MaxAttempts (8). The worker-level NextRetry keeps the 30s-base
// doubling curve capped at 1h and adds +/-10% jitter. Attempt history and
// dead-letter state are persisted by the injected WebhookPersistence
// adapter; the in-memory projection is only a cache for the HTTP surface.
//
// Provider inbound webhooks (Stripe) stay distinct integrations that
// emit internal events; they never flow through this dispatcher.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
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

// Subscription delivery states.
const (
	WebhookStatusActive     = "active"
	WebhookStatusDegraded   = "degraded"
	WebhookStatusDeadLetter = "dead_letter"
)

// WebhookSubscription is the stored subscription (secret never serialized).
type WebhookSubscription struct {
	ID          string
	WorkspaceID string
	URL         string
	EventTypes  []string
	Secret      []byte
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Delivered   map[string]struct{}
	Attempts    []WebhookAttempt
	DeadLetter  []WebhookDeadLetter
}

// WebhookAttempt is one delivery attempt fact.
type WebhookAttempt struct {
	DeliveryID string    `json:"delivery_id"`
	Attempt    int       `json:"attempt"`
	StatusCode int       `json:"status_code"`
	Outcome    string    `json:"outcome"`
	Error      string    `json:"error,omitempty"`
	At         time.Time `json:"at"`
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
	mu          sync.Mutex
	subs        map[string]*WebhookSubscription
	byWorkspace map[string][]string
	sender      WebhookSender
	enqueuer    WebhookEnqueuer
	persistence WebhookPersistence
	now         func() time.Time
}

// NewWebhookStore builds a store dispatching via sender and enqueuer.
func NewWebhookStore(sender WebhookSender, enqueuer WebhookEnqueuer) *WebhookStore {
	return &WebhookStore{
		subs: map[string]*WebhookSubscription{}, byWorkspace: map[string][]string{},
		sender: sender, enqueuer: enqueuer, now: time.Now,
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

// ReconcilePending re-enqueues durable deliveries left behind by an enqueue
// failure or a process crash between commit and River insertion.
func (s *WebhookStore) ReconcilePending(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	persistence, enqueuer := s.persistence, s.enqueuer
	s.mu.Unlock()
	if persistence == nil || enqueuer == nil {
		return nil
	}
	deliveries, err := persistence.PendingDeliveries(ctx)
	if err != nil {
		return err
	}
	for _, args := range deliveries {
		if _, err := enqueuer.Insert(ctx, args, WebhookDeliveryInsertOpts(args)); err != nil {
			slog.Warn("webhook pending reconciliation enqueue failed", "subscription", args.SubscriptionID, "delivery_id", args.DeliveryID, "err", err)
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

func mintWebhookSecret() ([]byte, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("events: mint webhook secret: %w", err)
	}
	return buf, nil
}

// FormatWhsecSecret renders raw key bytes for exactly-once transport:
// "whsec_" + base64. Minted secrets are stored raw and only rendered
// in this form at creation; reads never carry them.
func FormatWhsecSecret(raw []byte) string {
	return webhookSecretPrefix + base64.StdEncoding.EncodeToString(raw)
}

// ParseWhsecSecret decodes a whsec_-form secret to raw key bytes.
// ok=false when the input is not whsec_-formed or is mis-encoded.
func ParseWhsecSecret(input []byte) ([]byte, bool) {
	trimmed := strings.TrimSpace(string(input))
	if !strings.HasPrefix(trimmed, webhookSecretPrefix) {
		return nil, false
	}
	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, webhookSecretPrefix))
	if payload == "" {
		return nil, false
	}
	if decoded, err := base64.StdEncoding.DecodeString(payload); err == nil {
		return decoded, true
	}
	decoded, err := base64.RawStdEncoding.DecodeString(payload)
	if err != nil {
		return nil, false
	}
	return decoded, true
}

// validateWebhookSecret accepts only the Svix whsec_ transport form.
func validateWebhookSecret(secret []byte) error {
	if decoded, ok := ParseWhsecSecret(secret); ok {
		if len(decoded) < webhookMinSecretLength {
			return fmt.Errorf("events: webhook secret must decode to at least %d bytes", webhookMinSecretLength)
		}
		return nil
	}
	return fmt.Errorf("events: webhook secret must use whsec_ form")
}

// webhookSigningKey resolves the HMAC key from a whsec_ secret.
func webhookSigningKey(secret []byte) []byte {
	if decoded, ok := ParseWhsecSecret(secret); ok {
		return decoded
	}
	// The store keeps decoded key bytes in memory; only public inputs are
	// required to use the whsec_ representation.
	return secret
}

// Create registers a subscription. Empty secret mints one (returned once
// via minted=true); provided secrets must be >= 16 chars. Empty
// eventTypes means all frozen types.
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
	defer s.mu.Unlock()
	if len(s.byWorkspace[workspaceID]) >= webhookMaxSubscriptions {
		return nil, nil, false, fmt.Errorf("events: too many webhook subscriptions for workspace")
	}
	persistence := s.persistence
	if persistence != nil {
		if err := persistence.CreateSubscription(ctx, sub); err != nil {
			return nil, nil, false, err
		}
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
func (s *WebhookStore) Delete(ctx context.Context, workspaceID, id string) (bool, error) {
	if s == nil {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	workspaceID = strings.TrimSpace(workspaceID)
	sub, ok := s.subs[strings.TrimSpace(id)]
	if !ok || sub.WorkspaceID != workspaceID {
		return false, nil
	}
	persistence := s.persistence
	if persistence != nil {
		if err := persistence.DeleteSubscription(ctx, workspaceID, sub.ID); err != nil {
			return false, err
		}
	}
	delete(s.subs, sub.ID)
	kept := s.byWorkspace[workspaceID][:0]
	for _, candidate := range s.byWorkspace[workspaceID] {
		if candidate != sub.ID {
			kept = append(kept, candidate)
		}
	}
	s.byWorkspace[workspaceID] = kept
	return true, nil
}

func maskSubscription(sub *WebhookSubscription) WebhookView {
	failures := 0
	for _, attempt := range sub.Attempts {
		if attempt.Outcome != "delivered" {
			failures++
		}
	}
	return WebhookView{
		ID: sub.ID, WorkspaceID: sub.WorkspaceID, URL: sub.URL,
		EventTypes: append([]string(nil), sub.EventTypes...), HasSecret: len(sub.Secret) > 0,
		Status: sub.Status, FailureCount: failures, DeadLetterSize: len(sub.DeadLetter),
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

// SignWebhook builds the Svix-compatible signature headers for a body:
// svix-id carries the delivery id (evt_, stable across retries),
// svix-timestamp the unix-second signing time, and svix-signature
// "v1,<base64>" HMAC-SHA256 over "<id>.<timestamp>.<raw-body>" keyed
// by the whsec_-decoded secret.
func SignWebhook(secret, body []byte, deliveryID string, at time.Time) map[string]string {
	stamp := strconv.FormatInt(at.UTC().Unix(), 10)
	mac := hmac.New(sha256.New, webhookSigningKey(secret))
	_, _ = mac.Write([]byte(deliveryID + "."))
	_, _ = mac.Write([]byte(stamp + "."))
	_, _ = mac.Write(body)
	return map[string]string{
		svixIDHeader:        deliveryID,
		svixTimestampHeader: stamp,
		svixSignatureHeader: webhookSignatureVersion + "," + base64.StdEncoding.EncodeToString(mac.Sum(nil)),
	}
}

// VerifyWebhookSignature verifies the Svix-compatible headers (svix-id,
// svix-timestamp, svix-signature) with timestamp tolerance. At least
// one space-delimited v1,<base64> entry must match HMAC-SHA256 over
// "<id>.<timestamp>.<raw-body>". Comparison is constant time and empty
// secrets are rejected.
func VerifyWebhookSignature(secret, body []byte, headers map[string]string, now time.Time) error {
	decoded, ok := ParseWhsecSecret(secret)
	if !ok || len(decoded) < webhookMinSecretLength {
		return fmt.Errorf("events: webhook secret must use whsec_ form")
	}
	deliveryID := strings.TrimSpace(headerLookup(headers, svixIDHeader))
	stamp := strings.TrimSpace(headerLookup(headers, svixTimestampHeader))
	signature := strings.TrimSpace(headerLookup(headers, svixSignatureHeader))
	if deliveryID == "" || stamp == "" || signature == "" {
		return fmt.Errorf("events: webhook signature headers are required")
	}
	unix, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return fmt.Errorf("events: webhook timestamp is malformed")
	}
	sent := time.Unix(unix, 0).UTC()
	drift := now.UTC().Sub(sent)
	if drift < 0 {
		drift = -drift
	}
	if drift > webhookTimestampTolerance {
		return fmt.Errorf("events: webhook timestamp outside tolerance")
	}
	mac := hmac.New(sha256.New, decoded)
	_, _ = mac.Write([]byte(deliveryID + "."))
	_, _ = mac.Write([]byte(stamp + "."))
	_, _ = mac.Write(body)
	expected := mac.Sum(nil)
	for _, candidate := range strings.Fields(signature) {
		tag, payload, found := strings.Cut(candidate, ",")
		if !found || strings.TrimSpace(tag) != webhookSignatureVersion {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare(decoded, expected) == 1 {
			return nil
		}
	}
	return fmt.Errorf("events: webhook signature mismatch")
}

func headerLookup(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

// WebhookRetryDelay returns the backoff before attempt (1-based): 30s *
// 2^(attempt-1) capped at 1h. Attempt 1 needs no delay.
func WebhookRetryDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return 0
	}
	delay := webhookBackoffBase
	for i := 2; i < attempt; i++ {
		delay *= 2
		if delay >= webhookBackoffCap {
			return webhookBackoffCap
		}
	}
	if delay > webhookBackoffCap {
		return webhookBackoffCap
	}
	return delay
}

// Dispatch enqueues one webhook_deliver job per matching subscription
// of the workspace. Per-subscription type filtering and duplicate
// envelope collapse (dedup) happen here; River owns every attempt
// (including the first) and its retries. Enqueue never fails the
// caller: failures are logged and dropped, mirroring the NATS publish
// seam (emission follows the domain commit).
func (s *WebhookStore) Dispatch(ctx context.Context, envelope V1Envelope) {
	if s == nil {
		return
	}
	if err := ValidateV1Envelope(envelope); err != nil {
		return
	}
	s.mu.Lock()
	targets := []string{}
	for _, id := range s.byWorkspace[envelope.WorkspaceID] {
		sub, ok := s.subs[id]
		if !ok || sub.Status == WebhookStatusDeadLetter {
			continue
		}
		if !sub.Matches(envelope.Type) {
			continue
		}
		if _, dup := sub.Delivered[envelope.ID]; dup {
			continue
		}
		targets = append(targets, sub.ID)
	}
	enqueuer := s.enqueuer
	persistence := s.persistence
	s.mu.Unlock()
	for _, subscriptionID := range targets {
		if persistence != nil {
			if err := persistence.EnsureDelivery(ctx, envelope, subscriptionID); err != nil {
				slog.Warn("webhook durable delivery creation failed", "subscription", subscriptionID, "delivery_id", envelope.ID, "err", err)
				continue
			}
		}
		if enqueuer == nil {
			slog.Warn("webhook dispatch has no enqueuer; durable delivery remains pending", "delivery_id", envelope.ID, "subscription", subscriptionID)
			continue
		}
		args := WebhookDeliveryArgs{
			SubscriptionID: subscriptionID, DeliveryID: envelope.ID, Envelope: envelope,
		}
		if _, err := enqueuer.Insert(ctx, args, WebhookDeliveryInsertOpts(args)); err != nil {
			slog.Warn("webhook enqueue failed",
				"subscription", subscriptionID, "delivery_id", envelope.ID, "err", err)
		}
	}
}

// executeDeliveryAttempt performs ONE signed delivery attempt for the
// River worker. Nil means delivered or dropped without retry
// (unknown/deleted subscription, dead-lettered subscription, or a
// delivery already marked Delivered). A returned error asks River to
// retry; on budget exhaustion the live DLQ entry is parked first so
// the discarded job row and the read surface agree.
func (s *WebhookStore) executeDeliveryAttempt(ctx context.Context, subscriptionID string, envelope V1Envelope, attempt, maxAttempts int) error {
	if maxAttempts <= 0 {
		maxAttempts = webhookMaxAttempts
	}
	if attempt < 1 {
		attempt = 1
	}
	s.mu.Lock()
	sub, ok := s.subs[subscriptionID]
	persistence := s.persistence
	s.mu.Unlock()
	if !ok && persistence != nil {
		loaded, loadErr := persistence.LoadSubscription(ctx, subscriptionID)
		if loadErr != nil {
			return fmt.Errorf("events: load webhook subscription %s: %w", subscriptionID, loadErr)
		}
		if loaded != nil {
			s.mu.Lock()
			s.subs[subscriptionID] = loaded
			s.byWorkspace[loaded.WorkspaceID] = appendIfMissing(s.byWorkspace[loaded.WorkspaceID], subscriptionID)
			sub, ok = loaded, true
			s.mu.Unlock()
		}
	}
	s.mu.Lock()
	if !ok || sub.Status == WebhookStatusDeadLetter || sub.WorkspaceID != envelope.WorkspaceID {
		s.mu.Unlock()
		return nil
	}
	if _, dup := sub.Delivered[envelope.ID]; dup {
		s.mu.Unlock()
		return nil
	}
	body, err := json.Marshal(envelope)
	s.mu.Unlock()
	if err != nil {
		s.recordAttempt(ctx, subscriptionID, envelope, attempt, maxAttempts, 0, "failed", err.Error())
		return s.deliveryFailure(subscriptionID, envelope, attempt, maxAttempts, err.Error())
	}
	s.mu.Lock()
	secret := append([]byte(nil), sub.Secret...)
	destination := sub.URL
	s.mu.Unlock()
	headers := SignWebhook(secret, body, envelope.ID, s.clock())
	sender := s.sender
	if sender == nil {
		s.recordAttempt(ctx, subscriptionID, envelope, attempt, maxAttempts, 0, "failed", "no sender configured")
		return s.deliveryFailure(subscriptionID, envelope, attempt, maxAttempts, "no sender configured")
	}
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	status, sendErr := sender.Send(sendCtx, destination, headers, body)
	cancel()
	if sendErr == nil && status >= 200 && status < 300 {
		s.recordAttempt(ctx, subscriptionID, envelope, attempt, maxAttempts, status, "delivered", "")
		if persistence != nil {
			if err := persistence.MarkDelivered(ctx, subscriptionID, envelope); err != nil {
				return fmt.Errorf("events: persist delivered webhook: %w", err)
			}
		}
		s.mu.Lock()
		if current, ok := s.subs[subscriptionID]; ok {
			current.Delivered[envelope.ID] = struct{}{}
			if current.Status == WebhookStatusDegraded && failureCount(current) == 0 {
				current.Status = WebhookStatusActive
			}
			current.UpdatedAt = s.clock()
		}
		s.mu.Unlock()
		return nil
	}
	detail := ""
	if sendErr != nil {
		detail = sendErr.Error()
	} else {
		detail = fmt.Sprintf("unexpected status %d", status)
	}
	s.recordAttempt(ctx, subscriptionID, envelope, attempt, maxAttempts, status, "failed", detail)
	return s.deliveryFailure(subscriptionID, envelope, attempt, maxAttempts, detail)
}

// deliveryFailure parks the live DLQ entry once the budget is
// exhausted and always returns the failure for River (retry, then
// discard on the final attempt).
func (s *WebhookStore) deliveryFailure(subscriptionID string, envelope V1Envelope, attempt, maxAttempts int, detail string) error {
	if attempt >= maxAttempts {
		s.parkDeadLetterOnce(subscriptionID, envelope, attempt, detail)
	}
	return fmt.Errorf("events: webhook delivery %s attempt %d/%d failed: %s",
		envelope.ID, attempt, maxAttempts, detail)
}

// parkDeadLetterOnce moves the subscription to dead_letter and parks
// the envelope exactly once per delivery id, so worker redelivery
// after a restart never double-parks.
func (s *WebhookStore) parkDeadLetterOnce(subscriptionID string, envelope V1Envelope, attempts int, lastError string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[subscriptionID]
	if !ok {
		return
	}
	for _, parked := range sub.DeadLetter {
		if parked.DeliveryID == envelope.ID {
			sub.Status = WebhookStatusDeadLetter
			sub.UpdatedAt = s.clock()
			return
		}
	}
	sub.Status = WebhookStatusDeadLetter
	sub.DeadLetter = append(sub.DeadLetter, WebhookDeadLetter{
		DeliveryID: envelope.ID, EventType: envelope.Type,
		Attempts: attempts, LastError: lastError, At: s.clock(),
	})
	for len(sub.DeadLetter) > webhookDeadLetterCap {
		sub.DeadLetter = append([]WebhookDeadLetter(nil), sub.DeadLetter[1:]...)
	}
	sub.UpdatedAt = s.clock()
}

func (s *WebhookStore) recordAttempt(ctx context.Context, subscriptionID string, envelope V1Envelope, attempt, maxAttempts, status int, outcome, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[subscriptionID]
	if !ok {
		return
	}
	sub.Attempts = append(sub.Attempts, WebhookAttempt{
		DeliveryID: envelope.ID, Attempt: attempt, StatusCode: status,
		Outcome: outcome, Error: detail, At: s.clock(),
	})
	for len(sub.Attempts) > webhookAttemptLogCap {
		sub.Attempts = append([]WebhookAttempt(nil), sub.Attempts[1:]...)
	}
	if outcome != "delivered" && sub.Status == WebhookStatusActive {
		sub.Status = WebhookStatusDegraded
	}
	sub.UpdatedAt = s.clock()
	if s.persistence != nil {
		fact := WebhookAttempt{DeliveryID: envelope.ID, Attempt: attempt, StatusCode: status, Outcome: outcome, Error: detail, At: s.clock()}
		if err := s.persistence.RecordAttempt(ctx, subscriptionID, envelope, fact, attempt >= maxAttempts); err != nil {
			slog.Warn("webhook durable attempt record failed", "subscription", subscriptionID, "delivery_id", envelope.ID, "err", err)
		}
	}
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
		if attempt.Outcome != "delivered" {
			count++
		}
	}
	return count
}

// HTTPSender is the production WebhookSender (5s per-attempt timeout is
// enforced by the dispatcher context; this client keeps its own ceiling).
type HTTPSender struct {
	client *http.Client
}

// NewHTTPSender builds the production sender.
func NewHTTPSender() *HTTPSender {
	return &HTTPSender{client: &http.Client{Timeout: 10 * time.Second}}
}

// Send posts the envelope body with signature headers.
func (s *HTTPSender) Send(ctx context.Context, destination string, headers map[string]string, body []byte) (int, error) {
	client := http.DefaultClient
	if s != nil && s.client != nil {
		client = s.client
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, destination, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
	_ = response.Body.Close()
	return response.StatusCode, nil
}
