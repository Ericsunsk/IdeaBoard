package funds

import (
	"context"
	"testing"
	"time"
)

// TestIdentityWireProjection pins the handler contract: the wire carries ba_
// only, the internal join handle stays opaque but stable through the
// projection.
func TestIdentityWireProjection(t *testing.T) {
	identity := Identity{
		WorkspaceInternalID:    11,
		WorkspacePublicID:      "ws_test123456",
		BillingAccountID:       9001,
		BillingAccountPublicID: "ba_test123456",
		Shared:                 false,
		OwnerType:              OwnerTypeWorkspace,
		CreatedAt:              time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
		UpdatedAt:              time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
	}
	if got := identity.BA(); got != "ba_test123456" {
		t.Fatalf("BA() = %q, want ba_test123456", got)
	}
	projected := identity.ToBillingAccount()
	if projected.PublicID != "ba_test123456" || projected.InternalID != 9001 {
		t.Fatalf("ToBillingAccount() = %+v, want ba_/9001", projected)
	}
	if projected.WorkspaceInternalID != 11 || projected.WorkspacePublicID != "ws_test123456" {
		t.Fatalf("ToBillingAccount() workspace = %+v, want 11/ws_test123456", projected)
	}
}

// TestResolveFundsRejectsMalformedWorkspace pins fail-closed resolution: a
// malformed ws_ id never provisions or returns a zero handle.
func TestResolveFundsRejectsMalformedWorkspace(t *testing.T) {
	store := NewStore(nil)
	if _, err := store.ResolveFunds(context.Background(), "not-a-workspace"); err == nil {
		t.Fatal("ResolveFunds(malformed) = nil, want fail-closed error")
	}
	if _, err := store.ResolveFundsInTx(context.Background(), nil, "ws_test123456"); err == nil {
		t.Fatal("ResolveFundsInTx(nil tx) = nil, want fail-closed error")
	}
}

// TestResolveFundsRequiresStoreConfigured pins the borrowed-or-begin
// discipline: without a carrier and without a pool the seam fails closed
// with the historical database error instead of synthesizing an identity.
func TestResolveFundsRequiresStoreConfigured(t *testing.T) {
	store := NewStore(nil)
	// Valid ws_ shape but no ambient carrier and no pool: borrow fails and
	// begin fails closed (never a zero Identity with nil error).
	identity, err := store.ResolveFunds(context.Background(), "ws_0123456789ab")
	if err == nil {
		t.Fatalf("ResolveFunds without carrier/pool = %+v, want error", identity)
	}
	if identity.BillingAccountID != 0 || identity.BA() != "" {
		t.Fatalf("failed ResolveFunds must not return an identity: %+v", identity)
	}
}

// TestBillingAccountProjectionPreservesTenancyShape guards zero-semantic
// drift: the deep identity projects onto the agreed tenancy shape without
// reinterpreting the mapping.
func TestBillingAccountProjectionPreservesTenancyShape(t *testing.T) {
	identity := Identity{BillingAccountID: 7, BillingAccountPublicID: "ba_abc", OwnerType: OwnerTypeOrganization, Shared: true}
	projected := identity.ToBillingAccount()
	if projected.OwnerType != OwnerTypeOrganization || !projected.Shared {
		t.Fatalf("projection lost funds facts: %+v", projected)
	}
}
