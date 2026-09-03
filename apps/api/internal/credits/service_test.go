package credits

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
)

func TestCommandsInSQLTxRejectsNilTransaction(t *testing.T) {
	commands, err := CommandsInSQLTx(nil)
	if err == nil || commands != nil {
		t.Fatalf("commands=%v error=%v, want explicit nil transaction error", commands, err)
	}
}

func TestNewServiceNilPoolFailsClosed(t *testing.T) {
	service := NewService(nil)
	if service == nil {
		t.Fatal("NewService(nil) returned nil; existing callers require a non-nil facade")
	}
	if _, err := service.Grant(context.Background(), ports.GrantCommand{}); !errors.Is(err, errServiceUnavailable) {
		t.Fatalf("Grant() error = %v, want %v", err, errServiceUnavailable)
	}
	if _, err := service.GetWallet(context.Background(), 1); !errors.Is(err, errServiceUnavailable) {
		t.Fatalf("GetWallet() error = %v, want %v", err, errServiceUnavailable)
	}
	if _, err := service.ListLedger(context.Background(), domain.ListLedgerInput{}); !errors.Is(err, errServiceUnavailable) {
		t.Fatalf("ListLedger() error = %v, want %v", err, errServiceUnavailable)
	}
}

func TestNilServiceFailsClosed(t *testing.T) {
	var service *Service
	if _, err := service.Grant(context.Background(), ports.GrantCommand{}); !errors.Is(err, errServiceUnavailable) {
		t.Fatalf("nil Service Grant() error = %v, want %v", err, errServiceUnavailable)
	}
}
