package wiring

import (
	"testing"
)

// Base pairing test for service_wiring.go.
// The identity lifecycle composes provisioning + lifecycle over the pool;
// construction must never panic even with nil deps (fail-closed at call
// time, not at wiring time).
func TestNewIdentityLifecycleReturnsNonNil(t *testing.T) {
	lc := NewIdentityLifecycle(nil, nil, make([]byte, 32))
	if lc == nil {
		t.Fatal("NewIdentityLifecycle returned nil, want non-nil composer")
	}
	if lc.provisioning == nil || lc.lifecycle == nil {
		t.Fatal("IdentityLifecycle missing provisioning or lifecycle service")
	}
}
