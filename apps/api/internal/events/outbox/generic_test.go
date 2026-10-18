package outbox

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestOutboxRetryBackoffSQLMatchesPolicy pins the shared retry fragment to
// the OutboxRetry* policy constants: the SQL literal cannot reference Go
// constants, so this test fails the build if they drift apart.
func TestOutboxRetryBackoffSQLMatchesPolicy(t *testing.T) {
	baseSeconds := int(OutboxRetryBaseDelay / time.Second)
	if OutboxRetryBaseDelay != time.Duration(baseSeconds)*time.Second {
		t.Fatalf("OutboxRetryBaseDelay = %v, want whole seconds", OutboxRetryBaseDelay)
	}
	maxHours := int(OutboxRetryMaxDelay / time.Hour)
	if OutboxRetryMaxDelay != time.Duration(maxHours)*time.Hour {
		t.Fatalf("OutboxRetryMaxDelay = %v, want whole hours", OutboxRetryMaxDelay)
	}
	for _, want := range []string{
		fmt.Sprintf("left($2, %d)", OutboxRetryErrorMaxRunes),
		fmt.Sprintf("interval '%d hour'", maxHours),
		fmt.Sprintf("interval '%d seconds'", baseSeconds),
		fmt.Sprintf("LEAST(attempts, %d)", OutboxRetryMaxShift),
	} {
		if !strings.Contains(outboxRetryBackoffSQL, want) {
			t.Fatalf("outboxRetryBackoffSQL missing %q (policy drift):\n%s", want, outboxRetryBackoffSQL)
		}
	}
}
