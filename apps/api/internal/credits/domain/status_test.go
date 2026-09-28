package domain

import "testing"

// TestCanHoldTransitionMirrorsSQLGuards pins the durable hold
// transition table to execute_credit_command_v2 in
// internal/db/schema/fragments/credits.sql: partial settlements keep
// the hold held; full settlement flips it to captured/released;
// settled holds reject further commands. Every true entry was already
// possible before typing.
func TestCanHoldTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]HoldStatus{
		{HoldStatusHeld, HoldStatusHeld},
		{HoldStatusHeld, HoldStatusCaptured},
		{HoldStatusHeld, HoldStatusReleased},
	}
	for _, transition := range allowed {
		if !CanHoldTransition(transition[0], transition[1]) {
			t.Errorf("CanHoldTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	terminals := []HoldStatus{HoldStatusCaptured, HoldStatusReleased}
	live := []HoldStatus{HoldStatusHeld, HoldStatusCaptured, HoldStatusReleased}
	for _, from := range terminals {
		for _, to := range live {
			if CanHoldTransition(from, to) {
				t.Errorf("CanHoldTransition(%q, %q) = true, want false (terminal has no outgoing transitions)", from, to)
			}
		}
	}
	if CanHoldTransition(HoldStatusCaptured, HoldStatusReleased) {
		t.Error("CanHoldTransition(captured, released) = true, want false (settlement is final)")
	}
}

// TestCanWalletTransitionMirrorsSQLGuards pins the durable wallet
// transition table to set_credit_wallet_status in
// internal/db/schema/fragments/credits.sql.
func TestCanWalletTransitionMirrorsSQLGuards(t *testing.T) {
	if !CanWalletTransition(WalletStatusActive, WalletStatusFrozen) {
		t.Error("CanWalletTransition(active, frozen) = false, want true")
	}
	if !CanWalletTransition(WalletStatusFrozen, WalletStatusActive) {
		t.Error("CanWalletTransition(frozen, active) = false, want true")
	}
	if CanWalletTransition(WalletStatusActive, WalletStatusActive) {
		t.Error("CanWalletTransition(active, active) = true, want false (no self-transition guard exists)")
	}
	if CanWalletTransition(WalletStatusFrozen, WalletStatusFrozen) {
		t.Error("CanWalletTransition(frozen, frozen) = true, want false (no self-transition guard exists)")
	}
}

// TestStatusWireWords pins the wire/DB words: typing must not change
// the strings on the wire or in the database.
func TestStatusWireWords(t *testing.T) {
	for status, want := range map[WalletStatus]string{
		WalletStatusActive: "active",
		WalletStatusFrozen: "frozen",
	} {
		if string(status) != want {
			t.Errorf("WalletStatus = %q, want %q", string(status), want)
		}
	}
	for status, want := range map[HoldStatus]string{
		HoldStatusHeld: "held", HoldStatusCaptured: "captured", HoldStatusReleased: "released",
	} {
		if string(status) != want {
			t.Errorf("HoldStatus = %q, want %q", string(status), want)
		}
	}
}
