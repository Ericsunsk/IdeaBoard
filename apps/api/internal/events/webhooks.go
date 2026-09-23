package events

// Signed webhook subscriptions (issue #807).
//
// Model: a workspace-scoped subscription carries a destination URL, an
// event-type filter (empty = all frozen types), a signing secret, and
// delivery state. Reads never expose the secret plaintext: views carry
// has_secret only; the plaintext is returned exactly once at creation
// and only when server-minted.
//
// Delivery: POST the v1 envelope JSON with X-Webhook-Signature
// (hex HMAC-SHA256 over "<timestamp>.<raw-body>"), X-Webhook-Timestamp
// (unix seconds), X-Webhook-Delivery-Id (evt_ id), and
// X-Webhook-Attempt (1-based). Receivers accept timestamps within
// webhookTimestampTolerance (5 minutes).
//
// Retry/backoff: maxAttempts (8) with base 30s doubling capped at 1h.
// Attempt history is an in-memory ring (20 per subscription); after the
// budget is exhausted the subscription moves to dead_letter and the
// envelope lands in the dead-letter list (cap 100, oldest evicted).
// Retry scheduling is driven by the injected scheduler (production:
// time.AfterFunc; tests: manual) so retry-then-DLQ is deterministic.
// Durable subscription + outbox rows land with #809; until then this
// store is process-local (documented in the OpenAPI events delta).
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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	webhookSignatureHeader    = "X-Webhook-Signature"
	webhookTimestampHeader    = "X-Webhook-Timestamp"
	webhookDeliveryIDHeader   = "X-Webhook-Delivery-Id"
	webhookAttemptHeader      = "X-Webhook-Attempt"
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

// WebhookScheduler defers retries. Production uses time.AfterFunc;
// tests drive retries manually.
type WebhookScheduler func(delay time.Duration, fn func())

// AfterFuncScheduler is the production scheduler.
func AfterFuncScheduler(delay time.Duration, fn func()) {
	timer := time.AfterFunc(delay, fn)
	_ = timer
}

// WebhookStore owns subscriptions + dispatch for one process.
type WebhookStore struct {
	mu          sync.Mutex
	subs        map[string]*WebhookSubscription
	byWorkspace map[string][]string
	sender      WebhookSender
	scheduler   WebhookScheduler
	now         func() time.Time
}

// NewWebhookStore builds a store dispatching via sender.
func NewWebhookStore(sender WebhookSender, scheduler WebhookScheduler) *WebhookStore {
	if scheduler == nil {
		scheduler = AfterFuncScheduler
	}
	return &WebhookStore{
		subs: map[string]*WebhookSubscription{}, byWorkspace: map[string][]string{},
		sender: sender, scheduler: scheduler, now: time.Now,
	}
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
	minted := false
	if len(secret) == 0 {
		generated, err := mintWebhookSecret()
		if err != nil {
			return nil, nil, false, err
		}
		secret = generated
		minted = true
	} else if len(secret) < webhookMinSecretLength {
		return nil, nil, false, fmt.Errorf("events: webhook secret must be at least %d bytes", webhookMinSecretLength)
	}
	id, err := contract.MintPublicID(webhookSubscriptionPrefix, 12)
	if err != nil {
		return nil, nil, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.byWorkspace[workspaceID]) >= webhookMaxSubscriptions {
		return nil, nil, false, fmt.Errorf("events: too many webhook subscriptions for workspace")
	}
	now := s.clock()
	storedSecret := append([]byte(nil), secret...)
	sub := &WebhookSubscription{
		ID: id, WorkspaceID: workspaceID, URL: strings.TrimSpace(destination),
		EventTypes: append([]string(nil), normalized...), Secret: storedSecret,
		Status: WebhookStatusActive, CreatedAt: now, UpdatedAt: now,
		Delivered: map[string]struct{}{},
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
func (s *WebhookStore) Delete(workspaceID, id string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	workspaceID = strings.TrimSpace(workspaceID)
	sub, ok := s.subs[strings.TrimSpace(id)]
	if !ok || sub.WorkspaceID != workspaceID {
		return false
	}
	delete(s.subs, sub.ID)
	kept := s.byWorkspace[workspaceID][:0]
	for _, candidate := range s.byWorkspace[workspaceID] {
		if candidate != sub.ID {
			kept = append(kept, candidate)
		}
	}
	s.byWorkspace[workspaceID] = kept
	return true
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

// SignWebhook builds the signature + timestamp headers for a body.
func SignWebhook(secret, body []byte, at time.Time) map[string]string {
	stamp := strconv.FormatInt(at.UTC().Unix(), 10)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(stamp + "."))
	_, _ = mac.Write(body)
	return map[string]string{
		webhookSignatureHeader: hex.EncodeToString(mac.Sum(nil)),
		webhookTimestampHeader: stamp,
	}
}

// VerifyWebhookSignature verifies hex HMAC-SHA256 over
// "<timestamp>.<raw-body>" with timestamp tolerance. It compares in
// constant time and rejects empty secrets.
func VerifyWebhookSignature(secret, body []byte, headers map[string]string, now time.Time) error {
	if len(secret) == 0 {
		return fmt.Errorf("events: webhook secret is required")
	}
	signature := strings.TrimSpace(headerLookup(headers, webhookSignatureHeader))
	stamp := strings.TrimSpace(headerLookup(headers, webhookTimestampHeader))
	if signature == "" || stamp == "" {
		return fmt.Errorf("events: webhook signature headers are required")
	}
	decoded, err := hex.DecodeString(signature)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("events: webhook signature is malformed")
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
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(stamp + "."))
	_, _ = mac.Write(body)
	if subtle.ConstantTimeCompare(decoded, mac.Sum(nil)) != 1 {
		return fmt.Errorf("events: webhook signature mismatch")
	}
	return nil
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

// Dispatch delivers the envelope to every matching subscription of the
// workspace: attempt #1 runs inline (best-effort, sender timeout owns
// latency); failures schedule retries via the injected scheduler.
// Duplicate envelope ids per subscription collapse (dedup).
func (s *WebhookStore) Dispatch(ctx context.Context, envelope V1Envelope) {
	if s == nil {
		return
	}
	if err := ValidateV1Envelope(envelope); err != nil {
		return
	}
	s.mu.Lock()
	targets := []*WebhookSubscription{}
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
		targets = append(targets, sub)
	}
	s.mu.Unlock()
	for _, sub := range targets {
		s.attempt(ctx, sub.ID, envelope, 1)
	}
}

func (s *WebhookStore) attempt(ctx context.Context, subscriptionID string, envelope V1Envelope, attempt int) {
	s.mu.Lock()
	sub, ok := s.subs[subscriptionID]
	if !ok || sub.Status == WebhookStatusDeadLetter {
		s.mu.Unlock()
		return
	}
	if _, dup := sub.Delivered[envelope.ID]; dup {
		s.mu.Unlock()
		return
	}
	body, err := json.Marshal(envelope)
	s.mu.Unlock()
	if err != nil {
		s.recordAttempt(subscriptionID, envelope, attempt, 0, "failed", err.Error())
		s.afterAttempt(subscriptionID, envelope, attempt, err.Error())
		return
	}
	s.mu.Lock()
	secret := append([]byte(nil), sub.Secret...)
	destination := sub.URL
	s.mu.Unlock()
	headers := SignWebhook(secret, body, s.clock())
	headers[webhookDeliveryIDHeader] = envelope.ID
	headers[webhookAttemptHeader] = strconv.Itoa(attempt)
	sender := s.sender
	if sender == nil {
		s.recordAttempt(subscriptionID, envelope, attempt, 0, "failed", "no sender configured")
		s.afterAttempt(subscriptionID, envelope, attempt, "no sender configured")
		return
	}
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	status, sendErr := sender.Send(sendCtx, destination, headers, body)
	cancel()
	if sendErr == nil && status >= 200 && status < 300 {
		s.mu.Lock()
		if current, ok := s.subs[subscriptionID]; ok {
			current.Delivered[envelope.ID] = struct{}{}
			if current.Status == WebhookStatusDegraded && failureCount(current) == 0 {
				current.Status = WebhookStatusActive
			}
			current.UpdatedAt = s.clock()
		}
		s.mu.Unlock()
		s.recordAttempt(subscriptionID, envelope, attempt, status, "delivered", "")
		return
	}
	detail := ""
	if sendErr != nil {
		detail = sendErr.Error()
	} else {
		detail = fmt.Sprintf("unexpected status %d", status)
	}
	s.recordAttempt(subscriptionID, envelope, attempt, status, "failed", detail)
	s.afterAttempt(subscriptionID, envelope, attempt, detail)
}

func (s *WebhookStore) recordAttempt(subscriptionID string, envelope V1Envelope, attempt, status int, outcome, detail string) {
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
}

func (s *WebhookStore) afterAttempt(subscriptionID string, envelope V1Envelope, attempt int, lastError string) {
	s.mu.Lock()
	sub, ok := s.subs[subscriptionID]
	if !ok {
		s.mu.Unlock()
		return
	}
	if attempt >= webhookMaxAttempts {
		sub.Status = WebhookStatusDeadLetter
		sub.DeadLetter = append(sub.DeadLetter, WebhookDeadLetter{
			DeliveryID: envelope.ID, EventType: envelope.Type,
			Attempts: attempt, LastError: lastError, At: s.clock(),
		})
		for len(sub.DeadLetter) > webhookDeadLetterCap {
			sub.DeadLetter = append([]WebhookDeadLetter(nil), sub.DeadLetter[1:]...)
		}
		sub.UpdatedAt = s.clock()
		s.mu.Unlock()
		return
	}
	next := attempt + 1
	delay := WebhookRetryDelay(next)
	scheduler := s.scheduler
	s.mu.Unlock()
	if scheduler == nil {
		return
	}
	nextEnvelope := envelope
	scheduler(delay, func() {
		s.attempt(context.Background(), subscriptionID, nextEnvelope, next)
	})
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
