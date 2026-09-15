package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultHealthcheckPort = "6000"

func main() {
	os.Exit(run())
}

func run() int {
	url := healthcheckURL()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return 1
	}
	return 0
}

func healthcheckURL() string {
	url := strings.TrimSpace(os.Getenv("HEALTHCHECK_URL"))
	if url == "" {
		port := strings.TrimSpace(os.Getenv("APP_PORT"))
		if port == "" {
			port = defaultHealthcheckPort
		}
		url = "http://" + net.JoinHostPort("127.0.0.1", port) + "/api/health"
	}
	return url
}
