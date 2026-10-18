package credits_test

import (
	"context"
	"strings"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
)

func requireServiceUnavailable(t *testing.T, err error, op string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s error = nil, want service-unavailable failure", op)
	}
	// The nil-pool sentinel is unexported; pin the fail-closed behavior
	// through the existing error surface (non-nil + stable message).
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
	if _, err := service.Grant(context.Background(), ports.GrantCommand{}); err == nil || !strings.Contains(err.Error(), "database pool not configured") {
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
		_, err := service.Grant(context.Background(), ports.GrantCommand{})
		return err
	}(), "nil Service Grant()")
}
