// Package workermedia owns the HMAC presigned URL primitives for the sole
// Worker image egress handoff (ai-proxy): presigned GETs for inputs,
// one presigned PUT for the output. Every URL binds method, exact key,
// content-type, and short expiry; verification fails closed on any
// mismatch. It lives in platform so both the Worker transport
// (runtime/interfaces/worker/providers/infrastructure) and the public
// media route (runtime/interfaces/http) share one implementation without
// an interfaces/* cross-import.
package workermedia

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// RoutePrefix is the public HMAC media route serving the Worker handoff
	// (no session auth; HMAC query only).
	RoutePrefix = "/api/v1/worker/media/"
	// SlotContentType is the legacy exact-PNG binding. New outputs use SlotImagePolicy.
	SlotContentType = "image/png"
	// SlotImagePolicy binds a fixed, versioned PNG/JPEG/WebP allowlist to
	// the HMAC token. It is not a MIME wildcard or a stored content type.
	SlotImagePolicy = "worker-image-v1"
	// SlotTTL bounds every presigned URL (short validity).
	SlotTTL = 5 * time.Minute
	// SlotKeyRoot aligns slots with the future Media key scheme
	// (workspacemedia SlotKey root: uploads/ws/<ws>/<upload>/<file>).
	SlotKeyRoot = "uploads/ws/"
)

// AllowedOutputContentType deliberately excludes SVG, HTML and other formats.
func AllowedOutputContentType(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/webp":
		return true
	default:
		return false
	}
}

// MintOutputKey renders the exact storage key for the single result:
// uploads/ws/<ws-suffix>/<task-suffix>/result-0.png. Workspace and task ids
// fall back to closed constants (never empty, never traversal).
func MintOutputKey(workspaceID, taskID string) string {
	ws := strings.TrimPrefix(strings.TrimSpace(workspaceID), "ws_")
	if ws == "" {
		ws = "unknown"
	}
	task := strings.TrimSpace(taskID)
	if task == "" {
		task = "unknown"
	}
	return fmt.Sprintf("%s%s/%s/result-0.png", SlotKeyRoot, SafeSegment(ws), SafeSegment(task))
}

// SafeSegment sanitizes one key segment (no slashes, no traversal).
func SafeSegment(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "/", "_")
	s = strings.ReplaceAll(s, "\x00", "_")
	if s == "" || s == "." || s == ".." {
		return "unknown"
	}
	return s
}

// IsTraversal reports path traversal: any ".." segment or backslash escape.
func IsTraversal(key string) bool {
	for _, segment := range strings.Split(strings.TrimSpace(key), "/") {
		if segment == ".." || strings.Contains(segment, "\\") {
			return true
		}
	}
	return false
}

// ValidKey reports whether a verified key may ride the handoff (anywhere
// under uploads/, no traversal).
func ValidKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" || IsTraversal(key) {
		return false
	}
	return strings.HasPrefix(key, "uploads/")
}

// ValidSlotKey reports whether a verified key may receive a Worker output:
// exact slot root, no traversal. Inputs may live anywhere under uploads/;
// outputs stay pinned under the slot root.
func ValidSlotKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" || IsTraversal(key) {
		return false
	}
	return strings.HasPrefix(key, SlotKeyRoot)
}

// SignURL mints a presigned Worker media URL.
func SignURL(origin, secret, method, key, contentType string, expiresAt time.Time) string {
	base := strings.TrimRight(strings.TrimSpace(origin), "/")
	escaped := (&url.URL{Path: strings.TrimPrefix(strings.TrimSpace(key), "/")}).EscapedPath()
	sig := Signature(secret, method, key, contentType, expiresAt)
	return fmt.Sprintf("%s%s%s?ct=%s&exp=%d&sig=%s",
		base, RoutePrefix, strings.TrimPrefix(escaped, "/"),
		url.QueryEscape(strings.TrimSpace(contentType)), expiresAt.UTC().Unix(), sig)
}

// Signature renders sig=HMAC(secret, method\nkey\ncontentType\nexpUnix).
func Signature(secret, method, key, contentType string, expiresAt time.Time) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strings.ToUpper(strings.TrimSpace(method)) + "\n" + strings.TrimSpace(key) + "\n" + strings.TrimSpace(contentType) + "\n" + strconv.FormatInt(expiresAt.UTC().Unix(), 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verified holds one verified presigned URL.
type Verified struct {
	Key         string
	ContentType string
	ExpiresAt   time.Time
}

// VerifyURL verifies a presigned Worker media URL for one HTTP method. It
// returns the key and content-type when the signature, method binding, key
// shape, and expiry all check out; anything else fails closed.
func VerifyURL(secret, rawURL, method string) (Verified, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return Verified{}, fmt.Errorf("worker media URL is invalid")
	}
	path := strings.TrimSpace(parsed.EscapedPath())
	if path == "" {
		path = strings.TrimSpace(parsed.Path)
	}
	route := strings.TrimRight(RoutePrefix, "/") + "/"
	idx := strings.Index(path, route)
	if idx < 0 {
		return Verified{}, fmt.Errorf("worker media URL has no media route")
	}
	rawKey, err := url.PathUnescape(path[idx+len(route):])
	if err != nil || !ValidKey(rawKey) {
		return Verified{}, fmt.Errorf("worker media key is invalid")
	}
	q := parsed.Query()
	ct := strings.TrimSpace(q.Get("ct"))
	expUnix, err := strconv.ParseInt(strings.TrimSpace(q.Get("exp")), 10, 64)
	if err != nil || expUnix <= 0 {
		return Verified{}, fmt.Errorf("worker media URL has no expiry")
	}
	expiresAt := time.Unix(expUnix, 0).UTC()
	if time.Now().UTC().After(expiresAt) {
		return Verified{}, fmt.Errorf("worker media URL is expired")
	}
	sig := strings.TrimSpace(q.Get("sig"))
	if sig == "" {
		return Verified{}, fmt.Errorf("worker media URL is unsigned")
	}
	want := Signature(secret, method, rawKey, ct, expiresAt)
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return Verified{}, fmt.Errorf("worker media signature mismatch")
	}
	return Verified{Key: rawKey, ContentType: ct, ExpiresAt: expiresAt}, nil
}

// ParseKey extracts the exact storage key from a Worker media URL with or
// without its presigned query.
//
// Deprecated: parsing alone performs no signature check. Production must
// use VerifyURL for signed URLs. Query-stripped output echoes must only
// be consumed by WorkerOutputSlot.Accept, which binds the exact slot key
// before reading and validating the complete image. Kept for handoff
// internals and test adapters only.
func ParseKey(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return "", fmt.Errorf("worker media URL is invalid")
	}
	path := strings.TrimSpace(parsed.EscapedPath())
	if path == "" {
		path = strings.TrimSpace(parsed.Path)
	}
	route := strings.TrimRight(RoutePrefix, "/") + "/"
	idx := strings.Index(path, route)
	if idx < 0 {
		return "", fmt.Errorf("worker media URL has no media route")
	}
	rawKey, err := url.PathUnescape(path[idx+len(route):])
	if err != nil || !ValidKey(rawKey) {
		return "", fmt.Errorf("worker media key is invalid")
	}
	return rawKey, nil
}
