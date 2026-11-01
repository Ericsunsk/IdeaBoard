package wiring

import (
	"ideaboard/api/internal/platform/errorrender"
	"strings"
	"testing"
)

// Base pairing test for errors.go: backend-owned copy must stay
// non-empty so fail-closed paths never render a blank message.
func TestMessageCopyIsNonEmpty(t *testing.T) {
	if strings.TrimSpace(errorrender.MsgRuntimeCompositionWiringCallWorkerAPI) == "" {
		t.Fatal("msgCallWorkerAPI is empty, want user-facing copy")
	}
}
