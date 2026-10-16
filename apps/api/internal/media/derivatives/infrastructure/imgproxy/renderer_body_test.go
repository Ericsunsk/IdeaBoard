package imgproxy

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

type canceledImageBody struct {
	cancel context.CancelFunc
}

func (b canceledImageBody) Read([]byte) (int, error) {
	b.cancel()
	return 0, context.Canceled
}

func (canceledImageBody) Close() error { return nil }

type imageTransport func(*http.Request) (*http.Response, error)

func (f imageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRendererCancellationWhileReadingImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	renderer := &Renderer{client: &http.Client{Transport: imageTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/webp"}},
			Body:       canceledImageBody{cancel: cancel},
		}, nil
	})}}
	if _, err := renderer.fetch(ctx, "http://renderer/image"); !errors.Is(err, context.Canceled) {
		t.Fatalf("body read error = %v, want context.Canceled", err)
	}
}

func TestRendererTransportTimeoutIsUnavailable(t *testing.T) {
	renderer := &Renderer{client: &http.Client{Transport: imageTransport(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})}}
	if _, err := renderer.fetch(context.Background(), "http://renderer/image"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("transport timeout error = %v, want ErrUnavailable", err)
	}
}
