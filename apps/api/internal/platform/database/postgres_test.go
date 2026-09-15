package database

import (
	"context"
	"testing"
	"time"

	"ideaboard/api/internal/platform/config"
)

func TestNewPostgresHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, err := NewPostgres(ctx, config.DatabaseConfig{
		Host: "127.0.0.1", Port: "5432", User: "ideaboard", Database: "ideaboard", SSLMode: "disable",
	})
	if err == nil {
		t.Fatal("NewPostgres succeeded with cancelled context")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("cancelled connect took %v, want fast fail", elapsed)
	}
}

func TestPostgresConnectContextBoundsBackgroundParent(t *testing.T) {
	before := time.Now()
	ctx, cancel := postgresConnectContext(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("postgresConnectContext should set a deadline")
	}
	if deadline.Before(before) || deadline.After(before.Add(postgresConnectTimeout+time.Second)) {
		t.Fatalf("deadline = %v, want within %v from start %v", deadline, postgresConnectTimeout, before)
	}
}

func TestNextConnectDelayStaysBounded(t *testing.T) {
	delay := postgresConnectBaseDelay
	for range 200 {
		delay = nextConnectDelay(delay)
		if delay <= 0 || delay > postgresConnectMaxDelay {
			t.Fatalf("delay = %v, want within (0, %v]", delay, postgresConnectMaxDelay)
		}
	}
}
