package realtime

import (
	"strings"
	"testing"
	"time"
)

const testSecret = "test-realtime-ticket-secret-with-sufficient-entropy"

func TestIssueVerifyRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	ticket, exp, err := Issue(testSecret, "board_abc123", 2, CapWrite, "usr_test123", now)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if want := now.Add(TicketTTL); !exp.Equal(want) {
		t.Fatalf("exp = %v, want %v", exp, want)
	}
	if parts := strings.Split(ticket, "."); len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		t.Fatalf("ticket = %q, want base64url.payload.base64url(sig)", ticket)
	}
	payload, err := Verify(testSecret, ticket, now.Add(30*time.Second))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if payload.BID != "board_abc123" || payload.AV != 2 || payload.Cap != CapWrite || payload.UID != "usr_test123" {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Exp != exp.Unix() {
		t.Fatalf("payload.Exp = %d, want %d", payload.Exp, exp.Unix())
	}
}

func TestCapFor(t *testing.T) {
	if got := CapFor(true); got != CapWrite {
		t.Fatalf("CapFor(true) = %q, want write", got)
	}
	if got := CapFor(false); got != CapRead {
		t.Fatalf("CapFor(false) = %q, want read", got)
	}
}

func TestIssueRequiresSecret(t *testing.T) {
	now := time.Now()
	if _, _, err := Issue("", "board_abc", 1, CapRead, "usr_x", now); err == nil {
		t.Fatal("Issue without secret succeeded, want fail-closed error")
	}
	if _, err := Verify("", "bogus.ticket", now); err == nil {
		t.Fatal("Verify without secret succeeded, want fail-closed error")
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	ticket, _, err := Issue(testSecret, "board_abc123", 3, CapRead, "usr_test123", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(testSecret, ticket, now.Add(TicketTTL).Add(time.Second)); err == nil {
		t.Fatal("Verify past exp succeeded, want expiry rejection")
	} else if err != ErrExpired {
		t.Fatalf("Verify past exp error = %v, want ErrExpired", err)
	}
}

func TestVerifyRejectsTampered(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	ticket, _, err := Issue(testSecret, "board_abc123", 3, CapRead, "usr_test123", now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(ticket, ".")
	if len(parts) != 2 {
		t.Fatalf("ticket = %q", ticket)
	}
	// Flip the last payload character (keeps base64 valid, breaks the HMAC).
	tamperedPayload := parts[0][:len(parts[0])-1] + flipChar(parts[0][len(parts[0])-1])
	tampered := tamperedPayload + "." + parts[1]
	if _, err := Verify(testSecret, tampered, now); err == nil {
		t.Fatal("Verify tampered payload succeeded, want rejection")
	}
	// Wrong secret must also reject.
	if _, err := Verify("another-secret-with-sufficient-entropy-000", ticket, now); err == nil {
		t.Fatal("Verify wrong secret succeeded, want rejection")
	}
	// Truncated/garbage shapes reject.
	for _, bad := range []string{"", "noparts", ".", ticket + ".extra", parts[0] + "."} {
		if _, err := Verify(testSecret, bad, now); err == nil {
			t.Fatalf("Verify(%q) succeeded, want rejection", bad)
		}
	}
}

func flipChar(c byte) string {
	if c == 'A' {
		return "B"
	}
	return "A"
}

func TestAdmitGatesVersionArchiveAndRead(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	ticket, _, err := Issue(testSecret, "board_abc123", 5, CapRead, "usr_test123", now)
	if err != nil {
		t.Fatal(err)
	}
	at := now.Add(10 * time.Second)
	if _, admit, err := Admit(testSecret, ticket, at, 5, false, true); err != nil || !admit {
		t.Fatalf("Admit current = %v, %v; want admit=true", admit, err)
	}
	// Stale version (kick/ban bumped access_version) denies the old ticket.
	if _, admit, _ := Admit(testSecret, ticket, at, 6, false, true); admit {
		t.Fatal("Admit stale av succeeded, want deny")
	}
	// Archived boards deny.
	if _, admit, _ := Admit(testSecret, ticket, at, 5, true, true); admit {
		t.Fatal("Admit archived succeeded, want deny")
	}
	// Revoked read denies.
	if _, admit, _ := Admit(testSecret, ticket, at, 5, false, false); admit {
		t.Fatal("Admit without read succeeded, want deny")
	}
	// Expired tickets deny.
	if _, admit, _ := Admit(testSecret, ticket, now.Add(61*time.Second), 5, false, true); admit {
		t.Fatal("Admit expired succeeded, want deny")
	}
}
