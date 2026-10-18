package accountlimits

import "testing"

func TestMaxTimeoutSecondsCoversServerlessColdStart(t *testing.T) {
	if MaxTimeoutSeconds <= 0 {
		t.Fatalf("MaxTimeoutSeconds = %d, want positive", MaxTimeoutSeconds)
	}
	if MaxTimeoutSeconds != 900 {
		t.Fatalf("MaxTimeoutSeconds = %d, want 900", MaxTimeoutSeconds)
	}
}
