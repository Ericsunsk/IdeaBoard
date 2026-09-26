package events

// Svix-compatible webhook signing (issue #807).
//
// SignWebhook builds the signature headers for a body: svix-id carries
// the delivery id (evt_, stable across retries), svix-timestamp the
// unix-second signing time, and svix-signature "v1,<base64>" HMAC-SHA256
// over "<id>.<timestamp>.<raw-body>" keyed by the whsec_-decoded secret.
// VerifyWebhookSignature checks those headers with timestamp tolerance.
// Behavior here is byte-identical to the pre-split implementation: the
// official Svix known-answer vector and rotation semantics are pinned by
// webhooks_test.go.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

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
