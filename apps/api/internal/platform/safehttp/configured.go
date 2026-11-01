package safehttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ConfiguredEndpoint is one operator-trusted egress base (env/DB service
// configuration validated once at load time): an http(s) URL with a
// non-empty host. Trust covers the host only — every request still resolves
// DNS fresh and pins the dial to the resolved IPs, but private/loopback IPs
// are permitted because the operator chose the endpoint. Anything the user
// can influence (image URLs, provider paths, webhook callbacks, presigned
// fetches) must keep using the strict allowlist path (DoWithAllowHosts /
// RequestOptions.AllowHosts), never this type.
type ConfiguredEndpoint struct {
	base     string
	scheme   string
	hostname string
	port     string
}

// ParseConfiguredEndpoint validates one operator endpoint at load/startup
// time: the scheme must be http(s) and the host non-empty. It performs no
// DNS and no IP check — per-request resolution still happens on every send,
// so no validation verdict is cached.
func ParseConfiguredEndpoint(raw string) (ConfiguredEndpoint, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return ConfiguredEndpoint{}, errors.New(errorrender.MsgValidateConfiguredEndpoint)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return ConfiguredEndpoint{}, errors.New(errorrender.MsgValidateConfiguredEndpoint)
	}
	return ConfiguredEndpoint{
		base:     trimmed,
		scheme:   strings.ToLower(parsed.Scheme),
		hostname: normalizeHost(parsed.Hostname()),
		port:     defaultPort(parsed),
	}, nil
}

// Valid reports whether the endpoint holds a load-time validated base.
// The zero value is invalid and fails every send closed.
func (e ConfiguredEndpoint) Valid() bool {
	return e.hostname != "" && (e.scheme == "http" || e.scheme == "https")
}

// BaseURL returns the normalized configured base (no trailing slash).
func (e ConfiguredEndpoint) BaseURL() string {
	return e.base
}

// Hostname returns the normalized configured host.
func (e ConfiguredEndpoint) Hostname() string {
	return e.hostname
}

// matches reports whether target stays inside the endpoint: same scheme,
// same host, same effective port, no userinfo. Request URLs and every
// redirect hop failing this check fail closed with ErrPolicyViolation.
func (e ConfiguredEndpoint) matches(target *url.URL) bool {
	if target == nil || !e.Valid() {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(target.Scheme), e.scheme) {
		return false
	}
	if target.User != nil {
		return false
	}
	if normalizeHost(target.Hostname()) != e.hostname {
		return false
	}
	return defaultPort(target) == e.port
}

func checkConfiguredTarget(target *url.URL, endpoint ConfiguredEndpoint) error {
	if !endpoint.matches(target) {
		return fmt.Errorf(errorrender.MsgDoConfiguredRequest, ErrPolicyViolation)
	}
	return nil
}

// sendConfiguredOnce issues one attempt against the configured endpoint:
// fresh DNS on every attempt (no transport pooling, so no validation
// verdict is cached), dial pinned to the resolved IPs, private/loopback
// IPs permitted, request and every redirect hop pinned to the endpoint.
func (c *Client) sendConfiguredOnce(ctx context.Context, req *http.Request, body []byte, attemptTimeout time.Duration, endpoint ConfiguredEndpoint) (*http.Response, error) {
	if !endpoint.Valid() {
		return nil, fmt.Errorf("outbound configured endpoint is required")
	}
	attempt := req.Clone(ctx)
	if err := replayRequestBody(attempt, req, body); err != nil {
		return nil, err
	}
	if err := checkConfiguredTarget(attempt.URL, endpoint); err != nil {
		return nil, err
	}
	if _, err := net.DefaultResolver.LookupIPAddr(ctx, attempt.URL.Hostname()); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New(errorrender.MsgResolvePolicyWithTimeout4)
	}
	transport := configuredTransport()
	client := &http.Client{
		Timeout:   attemptTimeout,
		Transport: transport,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			// Same 3-hop cap as the strict path; every hop stays inside
			// the endpoint and re-resolves fresh (here and at dial time).
			if len(via) >= 3 {
				return errors.New(errorrender.MsgDoWithAllowHosts2)
			}
			if err := checkConfiguredTarget(next.URL, endpoint); err != nil {
				return err
			}
			if _, err := net.DefaultResolver.LookupIPAddr(next.Context(), next.URL.Hostname()); err != nil {
				if next.Context().Err() != nil {
					return next.Context().Err()
				}
				return errors.New(errorrender.MsgResolvePolicyWithTimeout4)
			}
			return nil
		},
	}
	return client.Do(attempt)
}

// configuredTransport builds a fresh (never pooled) transport that pins
// each dial to freshly resolved IPs without the private-IP refusal: the
// operator trusted this host at load time. Fresh construction per send is
// deliberate — a shared pool would cache a validation verdict across DNS
// changes (rebinding), which the configured path must not do.
func configuredTransport() *http.Transport {
	transport := cloneDefaultTransport()
	// This per-attempt transport is never reused. Close its connections when
	// response bodies finish instead of retaining an unreachable idle pool.
	transport.DisableKeepAlives = true
	dialer := &net.Dialer{
		Timeout:   defaultTimeout,
		KeepAlive: 30 * time.Second,
	}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		addrs, err := lookupDialIPs(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ipAddr := range addrs {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ipAddr.IP.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, errors.New(errorrender.MsgRestrictedDialContext)
	}
	return transport
}

// replayRequestBody attaches a replayable body to one attempt: the snapshot
// when the call buffered one, otherwise a fresh reader from the caller's
// GetBody hook. Shared by the strict and configured send paths so retries
// resend identical bytes on both.
func replayRequestBody(attempt *http.Request, req *http.Request, body []byte) error {
	if body != nil {
		attempt.Body = io.NopCloser(bytes.NewReader(body))
		attempt.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
		attempt.ContentLength = int64(len(body))
	} else if req.Body != nil && req.Body != http.NoBody && req.GetBody != nil {
		// Clone keeps Body shallow, so a request supplied with GetBody would be
		// consumed by the first attempt and replayed as an empty body. Ask the
		// caller's replay hook for a fresh reader on every attempt.
		freshBody, err := req.GetBody()
		if err != nil {
			return fmt.Errorf("outbound request body replay failed: %w", err)
		}
		attempt.Body = freshBody
	}
	return nil
}
