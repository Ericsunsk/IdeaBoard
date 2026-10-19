// Package realtime owns the board presence ticket: a short-lived HMAC
// bearer that binds one board access version to one caller.
//
// Ticket shape is frozen by the Y.js presence contract:
//
//	ticket = base64url(payload) + "." + base64url(hex(hmac-sha256))
//
// payload JSON is {"bid","av","cap","exp","uid"} where bid is the board_
// public id, av is the current access_version, cap is read|write derived
// from boards.CapabilitiesFor, exp is unix seconds (now+60s), and uid is
// the caller usr_ public identity. The HMAC input is the base64url
// payload segment itself (JWT-style), keyed by REALTIME_TICKET_SECRET.
//
// The package is dependency-free (no DB/River/HTTP): issuance (Issue)
// serves POST .../realtime-ticket, verification (Verify) plus the
// version/membership gate (Admit) serves GET /internal/realtime/admit.
// Both HTTP surfaces reuse this package; no second ticket implementation
// may exist.
package realtime

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// TicketTTL is the presence ticket lifetime: 60 seconds from issuance.
const TicketTTL = 60 * time.Second

// Ticket capability values carried in the payload cap field.
const (
	CapRead  = "read"
	CapWrite = "write"
)

// Payload is the signed ticket body. Field names are wire-frozen.
type Payload struct {
	BID string `json:"bid"`
	AV  int64  `json:"av"`
	Cap string `json:"cap"`
	Exp int64  `json:"exp"`
	UID string `json:"uid"`
}

var (
	// ErrInvalidTicket reports a malformed ticket, unknown fields, a bad
	// shape, or a signature mismatch. Callers render it as admit=false
	// (internal) or 404/500 per surface; never leak which check failed.
	ErrInvalidTicket = errors.New("realtime: invalid ticket")
	// ErrExpired reports a ticket past its exp claim.
	ErrExpired = errors.New("realtime: ticket expired")
	// ErrTicketSecretMissing reports issuance/verification without a secret.
	ErrTicketSecretMissing = errors.New("realtime: ticket secret is not configured")
)

// CapFor derives the ticket cap from the caller-visible write affordance:
// writers get write, every other reader gets read. Callers pass
// boards.CapabilitiesFor(role, ok, archived).CanWrite so the ticket cap
// never diverges from the board policy.
func CapFor(canWrite bool) string {
	if canWrite {
		return CapWrite
	}
	return CapRead
}

func validatePayloadFields(bid string, av int64, cap, uid string, exp int64) error {
	if strings.TrimSpace(bid) == "" || av <= 0 || strings.TrimSpace(uid) == "" || exp <= 0 {
		return ErrInvalidTicket
	}
	switch cap {
	case CapRead, CapWrite:
	default:
		return ErrInvalidTicket
	}
	return nil
}

// Issue mints one ticket for (bid, av, cap, uid) with exp = now+TicketTTL.
// The secret must be non-empty; empty fails closed without a fallback.
func Issue(secret, bid string, av int64, cap, uid string, now time.Time) (ticket string, exp time.Time, err error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", time.Time{}, ErrTicketSecretMissing
	}
	bid = strings.TrimSpace(bid)
	cap = strings.TrimSpace(cap)
	uid = strings.TrimSpace(uid)
	if now.IsZero() {
		return "", time.Time{}, ErrInvalidTicket
	}
	exp = now.UTC().Truncate(time.Second).Add(TicketTTL)
	payload := Payload{BID: bid, AV: av, Cap: cap, Exp: exp.Unix(), UID: uid}
	if err := validatePayloadFields(payload.BID, payload.AV, payload.Cap, payload.UID, payload.Exp); err != nil {
		return "", time.Time{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", time.Time{}, ErrInvalidTicket
	}
	segment := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(segment))
	hexSig := hex.EncodeToString(mac.Sum(nil))
	sigSegment := base64.RawURLEncoding.EncodeToString([]byte(hexSig))
	return segment + "." + sigSegment, exp, nil
}

// Verify checks the ticket signature and expiry and returns the payload.
// It reports ErrTicketSecretMissing without a secret, ErrExpired when
// now >= exp (matching the sidecar boundary), and ErrInvalidTicket for
// malformed/tampered tickets. It does NOT check the access version
// against current state; Admit does that.
func Verify(secret, ticket string, now time.Time) (Payload, error) {
	var zero Payload
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return zero, ErrTicketSecretMissing
	}
	ticket = strings.TrimSpace(ticket)
	if ticket == "" || now.IsZero() {
		return zero, ErrInvalidTicket
	}
	payloadSegment, sigSegment, found := strings.Cut(ticket, ".")
	if !found || strings.TrimSpace(payloadSegment) == "" || strings.TrimSpace(sigSegment) == "" || strings.Contains(sigSegment, ".") {
		return zero, ErrInvalidTicket
	}
	hexBytes, err := base64.RawURLEncoding.DecodeString(sigSegment)
	if err != nil {
		return zero, ErrInvalidTicket
	}
	hexSig := strings.TrimSpace(string(hexBytes))
	if hexSig == "" {
		return zero, ErrInvalidTicket
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payloadSegment))
	expected := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(hexSig), []byte(expected)) != 1 {
		return zero, ErrInvalidTicket
	}
	raw, err := base64.RawURLEncoding.DecodeString(payloadSegment)
	if err != nil {
		return zero, ErrInvalidTicket
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var payload Payload
	if err := decoder.Decode(&payload); err != nil {
		return zero, ErrInvalidTicket
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return zero, ErrInvalidTicket
	}
	payload.BID = strings.TrimSpace(payload.BID)
	payload.Cap = strings.TrimSpace(payload.Cap)
	payload.UID = strings.TrimSpace(payload.UID)
	if err := validatePayloadFields(payload.BID, payload.AV, payload.Cap, payload.UID, payload.Exp); err != nil {
		return zero, err
	}
	if now.UTC().Unix() >= payload.Exp {
		return zero, ErrExpired
	}
	return payload, nil
}

// Admit verifies the ticket and gates it against current board state:
// signature + expiry must hold, the ticket av must equal the current
// access_version (a kick/ban bumps it, so old tickets die on next call),
// the board must be live, and the caller must still hold board read.
// It returns the verified payload and the admit decision; any failure
// (including expired/tampered/stale) reports admit=false with the
// verification error for logging (callers must not leak the reason).
func Admit(secret, ticket string, now time.Time, currentAV int64, archived, hasRead bool) (Payload, bool, error) {
	payload, err := Verify(secret, ticket, now)
	if err != nil {
		return Payload{}, false, err
	}
	if archived || !hasRead || currentAV <= 0 || payload.AV != currentAV {
		return payload, false, nil
	}
	return payload, true, nil
}
