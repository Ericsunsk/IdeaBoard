package events

// Production webhook sender (issue #807).

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

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
