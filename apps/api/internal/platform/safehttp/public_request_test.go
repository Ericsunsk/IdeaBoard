package safehttp

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestPublicRequestRejectsPrivateHostEvenWhenNamed(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://127.0.0.1/source", nil)
	if _, err := DoPublicRequest(context.Background(), req, time.Second, []string{"127.0.0.1"}); !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("private IP permitted: %v", err)
	}
}

func TestPublicRequestValidatesEveryRedirect(t *testing.T) {
	previous := cloneDefaultTransport
	defer func() { cloneDefaultTransport = previous; safeTransportPool = sync.Map{} }()
	var redirect string
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/start" {
			http.Redirect(w, r, redirect, http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	cloneDefaultTransport = func() *http.Transport {
		return &http.Transport{DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", server.Listener.Addr().String(), server.Client().Transport.(*http.Transport).TLSClientConfig)
		}}
	}
	for _, test := range []struct {
		target  string
		allowed bool
	}{
		{"https://93.184.216.34/final", true},
		{"https://1.1.1.1/final", false},
		{"http://93.184.216.34/final", false},
		{"https://93.184.216.34:444/final", false},
		{"https://user@93.184.216.34/final", false},
	} {
		t.Run(test.target, func(t *testing.T) {
			safeTransportPool = sync.Map{}
			redirect = test.target
			requests = 0
			req, _ := http.NewRequest(http.MethodGet, "https://93.184.216.34/start", nil)
			response, err := DoPublicRequest(context.Background(), req, time.Second, []string{"93.184.216.34"})
			if response != nil {
				response.Body.Close()
			}
			if test.allowed {
				if err != nil || requests != 2 {
					t.Fatalf("valid redirect failed: %v", err)
				}
			} else if !errors.Is(err, ErrPolicyViolation) || requests != 1 {
				t.Fatalf("untrusted redirect dispatched: %v", err)
			}
		})
	}
}
