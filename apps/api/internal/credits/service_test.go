package credits_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/platform/database"
)

func requireServiceUnavailable(t *testing.T, err error, op string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s error = nil, want service-unavailable failure", op)
	}
	// The nil-pool guard converges on the shared database sentinel; pin
	// both the sentinel and the stable message surface.
	if !errors.Is(err, database.ErrPoolNotConfigured) {
		t.Fatalf("%s error = %v, want %v", op, err, database.ErrPoolNotConfigured)
	}
	if !strings.Contains(err.Error(), "database pool not configured") {
		t.Fatalf("%s error = %v, want service-unavailable failure", op, err)
	}
}

func TestCommandsInTxRejectsNilTransaction(t *testing.T) {
	commands, err := credits.CommandsInTx(nil)
	if err == nil || commands != nil {
		t.Fatalf("commands=%v error=%v, want explicit nil transaction error", commands, err)
	}
}

func TestNewServiceNilPoolFailsClosed(t *testing.T) {
	service := credits.NewService(nil)
	if service == nil {
		t.Fatal("NewService(nil) returned nil; existing callers require a non-nil facade")
	}
	if _, err := service.Administrative().Grant(context.Background(), ports.GrantCommand{}); err == nil || !strings.Contains(err.Error(), "database pool not configured") {
		t.Fatalf("Grant() error = %v, want service-unavailable failure", err)
	}
	if _, err := service.GetWallet(context.Background(), 1); err == nil || !strings.Contains(err.Error(), "database pool not configured") {
		t.Fatalf("GetWallet() error = %v, want service-unavailable failure", err)
	}
	if _, err := service.ListLedger(context.Background(), domain.ListLedgerInput{}); err == nil || !strings.Contains(err.Error(), "database pool not configured") {
		t.Fatalf("ListLedger() error = %v, want service-unavailable failure", err)
	}
}

func TestNilServiceFailsClosed(t *testing.T) {
	var service *credits.Service
	requireServiceUnavailable(t, func() error {
		_, err := service.Administrative().Grant(context.Background(), ports.GrantCommand{})
		return err
	}(), "nil Service Grant()")
}
