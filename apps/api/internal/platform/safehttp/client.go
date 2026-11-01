package safehttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/retrypolicy"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultTimeout bounds one outbound call (all attempts included) when
	// the caller sets none.
	DefaultTimeout = 30 * time.Second
	// DefaultMaxTimeout caps any per-call timeout: callers cannot extend a
	// single outbound call beyond this, matching the ADR-0010 gateway total
	// budget that the first migration targets.
	DefaultMaxTimeout = 300 * time.Second
	// DefaultMaxAttempts bounds client-side sends of one call.
	DefaultMaxAttempts = 3
	// hardMaxAttempts is the absolute retry ceiling: no caller config can
	// push client-side attempts past it.
	hardMaxAttempts = 5
	// DefaultInitialBackoff seeds exponential backoff between attempts.
	DefaultInitialBackoff = 500 * time.Millisecond
	// DefaultMaxBackoff caps exponential backoff growth.
	DefaultMaxBackoff = 5 * time.Second
	// DefaultMaxRetryAfterWait caps one honored Retry-After delay so a
	// rogue upstream header cannot park a worker indefinitely.
	DefaultMaxRetryAfterWait = 30 * time.Second
	// maxBufferedBodyBytes caps request-body buffering for replay across
	// retries. Gateway payloads are small JSON; larger bodies fail fast
	// instead of ballooning worker memory.
	maxBufferedBodyBytes = 16 << 20
	// maxStatusBodyPreview bounds the body kept on an exhausted status
	// error. Callers needing full payloads succeed on 2xx; this preview
	// only feeds failure classification and logs.
	maxStatusBodyPreview = 4 << 10
)

// HTTPStatusError is the terminal outcome when a retryable HTTP status
// (408/429/5xx) exhausts the bounded attempts. It preserves the last
// status, a truncated body preview, and the advertised Retry-After so
// callers keep classifying failures exactly like single-shot responses.
type HTTPStatusError struct {
	StatusCode int
	Body       string
	RetryAfter time.Duration
	Attempts   int
	Target     string
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return "outbound request failed"
	}
	return fmt.Sprintf("outbound request to %s failed with status %d after %d attempts", e.Target, e.StatusCode, e.Attempts)
}

// Config is the retry/timeout policy for one Client. Zero values take the
// documented defaults; MaxAttempts above hardMaxAttempts clamps down.
type Config struct {
	// Timeout bounds one call (all attempts) when the request sets none.
	Timeout time.Duration
	// MaxTimeout caps any effective timeout, per-request or default.
	MaxTimeout time.Duration
	// MaxAttempts bounds sends per call, clamped to [1, hardMaxAttempts].
	MaxAttempts int
	// InitialBackoff seeds exponential backoff; MaxBackoff caps it.
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	// MaxRetryAfterWait caps one honored Retry-After delay.
	MaxRetryAfterWait time.Duration
	// Metrics receives request/retry observations; nil keeps DefaultMetrics.
	Metrics *Metrics
	// Sleep waits between attempts; nil uses a context-aware timer. Tests
	// override it to skip real waiting while asserting computed delays.
	Sleep func(ctx context.Context, d time.Duration) error
}

// RequestOptions scopes one call. Zero values fall back to client Config.
type RequestOptions struct {
	// Timeout bounds this call (all attempts); zero keeps client default.
	// Values above the client MaxTimeout clamp down to the cap.
	Timeout time.Duration
	// MaxAttempts overrides the client bound for this call (single-shot
	// probes pass 1); zero keeps client default, above-cap clamps down.
	MaxAttempts int
	// AllowHosts pins egress through the safehttp SSRF allowlist.
	AllowHosts []string
	// Configured pins egress to one operator-trusted endpoint validated at
	// load time (ParseConfiguredEndpoint). It must not combine with
	// AllowHosts or PublicHosts: mixing fails closed. The request and every
	// redirect hop must stay inside the endpoint host; private/loopback IPs
	// are permitted there. User-influenced URLs must never use this field.
	Configured ConfiguredEndpoint
	// Target names this egress in logs/metrics; empty defaults to the
	// request host.
	Target string
	// PublicHosts restricts HTTPS hosts on every hop without granting private egress.
	PublicHosts []string
}

// Client is the standard outbound HTTP client.
type Client struct {
	timeout        time.Duration
	maxTimeout     time.Duration
	maxAttempts    int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	maxRetryAfter  time.Duration
	metrics        *Metrics
	sleep          func(ctx context.Context, d time.Duration) error
}

// NewClient validates the policy fail-fast and fills defaults.
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	maxTimeout := cfg.MaxTimeout
	if maxTimeout <= 0 {
		maxTimeout = DefaultMaxTimeout
	}
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	if maxAttempts > hardMaxAttempts {
		maxAttempts = hardMaxAttempts
	}
	initialBackoff := cfg.InitialBackoff
	if initialBackoff <= 0 {
		initialBackoff = DefaultInitialBackoff
	}
	maxBackoff := cfg.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = DefaultMaxBackoff
	}
	maxRetryAfter := cfg.MaxRetryAfterWait
	if maxRetryAfter <= 0 {
		maxRetryAfter = DefaultMaxRetryAfterWait
	}
	metrics := cfg.Metrics
	if metrics == nil {
		metrics = DefaultMetrics
	}
	sleep := cfg.Sleep
	if sleep == nil {
		sleep = sleepWithContext
	}
	return &Client{
		timeout:        timeout,
		maxTimeout:     maxTimeout,
		maxAttempts:    maxAttempts,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
		maxRetryAfter:  maxRetryAfter,
		metrics:        metrics,
		sleep:          sleep,
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Do sends one request through safehttp with timeout cap, bounded retries
// and Retry-After respect. Non-retryable responses (2xx, 3xx, non-retryable
// 4xx) return as-is for the caller to interpret; retryable failures exhaust
// MaxAttempts and then surface the last error/response.
//
// The overall call never exceeds the effective timeout: one derived context
// deadline bounds every attempt plus every inter-attempt wait, so retries
// buy earlier recovery, never a longer budget.
func (c *Client) Do(ctx context.Context, req *http.Request, opts RequestOptions) (*http.Response, error) {
	if c == nil {
		return nil, fmt.Errorf("outbound client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("outbound request is required")
	}
	target := strings.TrimSpace(opts.Target)
	if target == "" {
		target = req.URL.Hostname()
	}
	startedAt := time.Now()
	maxAttempts := c.maxAttempts
	if opts.MaxAttempts > 0 {
		maxAttempts = opts.MaxAttempts
		if maxAttempts > hardMaxAttempts {
			maxAttempts = hardMaxAttempts
		}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = c.timeout
	}
	if c.maxTimeout > 0 && timeout > c.maxTimeout {
		slog.WarnContext(ctx, "outbound timeout clamped to cap",
			"target", target, "requested_timeout", timeout.String(), "max_timeout", c.maxTimeout.String())
		timeout = c.maxTimeout
	}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	body, err := snapshotRequestBody(callCtx, req)
	if err != nil {
		cancel()
		return nil, c.finishWithLatency(target, err, time.Since(startedAt))
	}
	if req.Body != nil {
		defer req.Body.Close()
	}
	// No defer cancel here: the call budget must stay alive until the
	// caller finishes reading a returned response body. Error paths cancel
	// explicitly; success paths transfer cancel ownership to the wrapped
	// body (Close releases the timer). Letting the deferred cancel fire at
	// Do return would abort large streaming reads (image downloads,
	// storage objects) with "context canceled" once the socket buffer
	// drains.

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := callCtx.Err(); err != nil {
			cancel()
			return nil, c.finishWithLatency(target, attemptExhaustedError(callCtx, ctx, err), time.Since(startedAt))
		}
		resp, err := c.sendOnce(callCtx, req, body, timeout, opts)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				cancel()
				return nil, c.finishWithLatency(target, err, time.Since(startedAt))
			}
			if errors.Is(err, ErrPolicyViolation) {
				cancel()
				return nil, c.finishWithLatency(target, err, time.Since(startedAt))
			}
			timedOut := isTimeoutError(err, callCtx, ctx)
			lastErr = err
			if attempt == maxAttempts {
				cancel()
				if timedOut {
					slog.WarnContext(ctx, "outbound call timed out after bounded attempts",
						"target", target, "attempts", attempt, "timeout", timeout.String(), "err", err)
					return nil, c.finishWithLatency(target, lastErr, time.Since(startedAt))
				}
				slog.WarnContext(ctx, "outbound call failed after bounded attempts",
					"target", target, "attempts", attempt, "err", err)
				return nil, c.finishWithLatency(target, lastErr, time.Since(startedAt))
			}
			c.metrics.observeRetry(target, retryReason(0, timedOut, !timedOut))
			wait := c.backoffForAttempt(attempt)
			slog.WarnContext(ctx, "outbound attempt failed; retrying",
				"target", target, "attempt", attempt, "max_attempts", maxAttempts,
				"reason", retryReason(0, timedOut, !timedOut), "wait", wait.String(), "err", err)
			if sleepErr := c.sleep(callCtx, wait); sleepErr != nil {
				cancel()
				return nil, c.finishWithLatency(target, attemptExhaustedError(callCtx, ctx, sleepErr), time.Since(startedAt))
			}
			continue
		}
		if !retryableStatus(resp.StatusCode) {
			c.metrics.observeRequest(target, outcomeForStatus(resp.StatusCode))
			c.metrics.observeDuration(target, outcomeForStatus(resp.StatusCode), time.Since(startedAt))
			return withCallBudget(resp, cancel), nil
		}
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		preview := readStatusBodyPreview(resp.Body)
		drainAndClose(resp.Body)
		if attempt == maxAttempts {
			cancel()
			slog.WarnContext(ctx, "outbound call failed after bounded attempts",
				"target", target, "attempts", attempt, "status", resp.StatusCode)
			return nil, c.finishWithLatency(target, &HTTPStatusError{
				StatusCode: resp.StatusCode,
				Body:       preview,
				RetryAfter: retryAfter,
				Attempts:   attempt,
				Target:     target,
			}, time.Since(startedAt))
		}
		c.metrics.observeRetry(target, retryReason(resp.StatusCode, false, false))
		wait, honored := c.waitForRetry(attempt, retryAfter)
		if honored {
			c.metrics.observeRetryAfter(target)
		}
		slog.WarnContext(ctx, "outbound attempt failed; retrying",
			"target", target, "attempt", attempt, "max_attempts", maxAttempts,
			"reason", retryReason(resp.StatusCode, false, false),
			"status", resp.StatusCode, "wait", wait.String(), "retry_after_honored", honored)
		if sleepErr := c.sleep(callCtx, wait); sleepErr != nil {
			cancel()
			return nil, c.finishWithLatency(target, attemptExhaustedError(callCtx, ctx, sleepErr), time.Since(startedAt))
		}
	}
	cancel()
	return nil, c.finishWithLatency(target, lastErr, time.Since(startedAt))
}

// cancelOnCloseBody keeps the call budget (deadline + timer) alive for
// streaming body reads after Do returns. Close releases the timer, so
// callers that always close bodies never leak; callers that don't are
// still bounded by the call deadline.
type cancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelOnCloseBody) Close() error {
	if b == nil || b.ReadCloser == nil {
		if b != nil && b.cancel != nil {
			b.cancel()
		}
		return nil
	}
	err := b.ReadCloser.Close()
	if b.cancel != nil {
		b.cancel()
	}
	return err
}

// withCallBudget transfers the call's cancel ownership to the returned
// response body. A nil body (never produced by the safehttp transport, but
// guarded anyway) cancels immediately since there is nothing to bound.
func withCallBudget(resp *http.Response, cancel context.CancelFunc) *http.Response {
	if resp == nil {
		if cancel != nil {
			cancel()
		}
		return nil
	}
	if resp.Body == nil {
		if cancel != nil {
			cancel()
		}
		return resp
	}
	resp.Body = &cancelOnCloseBody{ReadCloser: resp.Body, cancel: cancel}
	return resp
}

// finishWithLatency records the terminal outcome plus RED latency. A
// non-positive latency skips the histogram (legacy callers); Do always
// passes the measured call latency.
func (c *Client) finishWithLatency(target string, err error, latency time.Duration) error {
	outcome := ""
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		outcome = outcomeCanceled
		c.metrics.observeRequest(target, outcome)
	case errors.Is(err, ErrPolicyViolation):
		outcome = outcomeClientError
		c.metrics.observeRequest(target, outcome)
	case isTimeoutError(err, nil, nil):
		outcome = outcomeTimeout
		c.metrics.observeRequest(target, outcome)
	default:
		var statusErr *HTTPStatusError
		if errors.As(err, &statusErr) {
			outcome = outcomeForStatus(statusErr.StatusCode)
			c.metrics.observeRequest(target, outcome)
		} else {
			outcome = outcomeTransportErr
			c.metrics.observeRequest(target, outcome)
		}
	}
	if outcome != "" && latency > 0 {
		c.metrics.observeDuration(target, outcome, latency)
	}
	return err
}

// readStatusBodyPreview keeps a bounded preview of an error body for
// failure classification. The body is drained and closed by the caller.
func readStatusBodyPreview(body io.Reader) string {
	if body == nil {
		return ""
	}
	preview, err := io.ReadAll(io.LimitReader(body, maxStatusBodyPreview+1))
	if err != nil {
		return ""
	}
	if len(preview) > maxStatusBodyPreview {
		preview = preview[:maxStatusBodyPreview]
	}
	return string(preview)
}

// sendOnce issues one attempt with a replayable body and a per-attempt
// client timeout. The parent callCtx already bounds the whole call; the
// per-attempt http.Client timeout keeps a single hung socket from eating
// the full budget silently. A Configured endpoint takes the operator-trust
// path; anything else stays on the strict allowlist path.
func (c *Client) sendOnce(ctx context.Context, req *http.Request, body []byte, attemptTimeout time.Duration, opts RequestOptions) (*http.Response, error) {
	if opts.Configured.Valid() {
		if len(opts.AllowHosts) > 0 || len(opts.PublicHosts) > 0 {
			return nil, fmt.Errorf("outbound egress policy is ambiguous: %w", ErrPolicyViolation)
		}
		return c.sendConfiguredOnce(ctx, req, body, attemptTimeout, opts.Configured)
	}
	attempt := req.Clone(ctx)
	if err := replayRequestBody(attempt, req, body); err != nil {
		return nil, err
	}
	allowHosts, publicHosts := opts.AllowHosts, opts.PublicHosts
	if len(publicHosts) > 0 {
		response, err := DoPublicRequest(ctx, attempt, attemptTimeout, publicHosts)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if errors.Is(err, ErrPolicyViolation) {
				return nil, ErrPolicyViolation
			}
			return nil, errors.New("public outbound request failed")
		}
		return response, nil
	}
	return DoRequestWithAllowHosts(ctx, attempt, attemptTimeout, allowHosts...)
}

// snapshotRequestBody buffers a non-replayable request body once so retries
// resend identical bytes. Requests with GetBody set need no buffering.
func snapshotRequestBody(ctx context.Context, req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	if req.GetBody != nil {
		return nil, nil
	}
	originalBody := req.Body
	type readResult struct {
		payload []byte
		err     error
	}
	resultCh := make(chan readResult, 1)
	go func() {
		payload, err := io.ReadAll(io.LimitReader(originalBody, maxBufferedBodyBytes+1))
		_ = originalBody.Close()
		resultCh <- readResult{payload, err}
	}()
	var result readResult
	select {
	case <-ctx.Done():
		_ = originalBody.Close()
		return nil, ctx.Err()
	case result = <-resultCh:
	}
	if result.err != nil {
		return nil, fmt.Errorf("outbound request body read failed: %w", result.err)
	}
	payload := result.payload
	if int64(len(payload)) > maxBufferedBodyBytes {
		return nil, fmt.Errorf("outbound request body exceeds %d bytes", maxBufferedBodyBytes)
	}
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	}
	return payload, nil
}

// waitForRetry prefers an advertised Retry-After delay (capped) over
// exponential backoff for retryable statuses.
func (c *Client) waitForRetry(failedAttempt int, retryAfter time.Duration) (time.Duration, bool) {
	if retryAfter > 0 {
		wait := retryAfter
		if c.maxRetryAfter > 0 && wait > c.maxRetryAfter {
			wait = c.maxRetryAfter
		}
		return wait, true
	}
	return c.backoffForAttempt(failedAttempt), false
}

func (c *Client) backoffForAttempt(failedAttempt int) time.Duration {
	wait := c.initialBackoff
	for i := 1; i < failedAttempt; i++ {
		if wait >= c.maxBackoff || wait > time.Duration(1<<63-1)/2 {
			return c.maxBackoff
		}
		wait *= 2
		if wait >= c.maxBackoff {
			return c.maxBackoff
		}
	}
	if wait > c.maxBackoff {
		return c.maxBackoff
	}
	return wait
}

func outcomeForStatus(status int) string {
	switch {
	case status >= 200 && status < 400:
		return outcomeSuccess
	case status >= 500:
		return outcomeServerError
	default:
		return outcomeClientError
	}
}

// isTimeoutError reports deadline-driven failures. A nil ctx skips the
// context check (metric classification of a finished error chain).
func isTimeoutError(err error, callCtx, parentCtx context.Context) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if callCtx != nil && errors.Is(callCtx.Err(), context.DeadlineExceeded) {
		return true
	}
	if parentCtx != nil && errors.Is(parentCtx.Err(), context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// attemptExhaustedError prefers caller cancellation over the derived
// deadline so orchestrators keep converging cancel vs timeout correctly.
func attemptExhaustedError(callCtx, parentCtx context.Context, err error) error {
	if parentCtx != nil && errors.Is(parentCtx.Err(), context.Canceled) {
		return parentCtx.Err()
	}
	if callCtx != nil && errors.Is(callCtx.Err(), context.Canceled) {
		return callCtx.Err()
	}
	return err
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 64<<10))
	_ = body.Close()
}

// merged from internal/platform/safehttp/retry.go (structural consolidation; symbols unchanged).
// parseRetryAfter parses a Retry-After header value: delta-seconds or an
// HTTP date. It mirrors runtime ports' parser locally because platform
// packages must not import business contexts. Unparseable or past values
// read as zero (no wait); callers fall back to backoff.
func parseRetryAfter(raw string) time.Duration {
	return retrypolicy.ParseRetryAfter(raw)
}

// retryableStatus reports whether an HTTP status is worth one more attempt:
// 408/429 (client should wait and retry) and 5xx (server-side transient).
// Every other status — including 4xx auth/quota/input failures — is final.
func retryableStatus(status int) bool {
	return retrypolicy.RetryableHTTPStatus(status)
}

// retryReason classifies one retry for logs and metrics. It stays a small
// closed set so metric cardinality never grows with upstream bodies.
func retryReason(status int, timedOut, transport bool) string {
	switch {
	case timedOut:
		return "timeout"
	case transport:
		return "transport_error"
	case status == http.StatusTooManyRequests:
		return "http_429"
	case status == http.StatusRequestTimeout:
		return "http_408"
	case status >= http.StatusInternalServerError:
		return "http_5xx"
	default:
		return "unknown"
	}
}
