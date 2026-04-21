package safehttp

import "testing"

func TestValidateURLRejectsLocalhostByDefault(t *testing.T) {
	if _, err := ValidateURL("http://localhost:6000/uploads/test.png"); err == nil {
		t.Fatal("expected localhost URL to be rejected by default")
	}
}

func TestValidateURLWithAllowHostsAllowsTrustedLocalhost(t *testing.T) {
	if _, err := ValidateURLWithAllowHosts(
		"http://localhost:6000/uploads/test.png",
		"http://localhost:6000/v1",
	); err != nil {
		t.Fatalf("expected trusted localhost URL to pass validation, got %v", err)
	}
}

func TestValidateURLWithAllowHostsAllowsEquivalentLoopbackIP(t *testing.T) {
	if _, err := ValidateURLWithAllowHosts(
		"http://127.0.0.1:6000/uploads/test.png",
		"http://localhost:6000/v1",
	); err != nil {
		t.Fatalf("expected equivalent loopback IP to pass validation, got %v", err)
	}
}

func TestValidateURLWithAllowHostsStillRejectsOtherPrivateHosts(t *testing.T) {
	if _, err := ValidateURLWithAllowHosts(
		"http://10.0.0.8:6000/uploads/test.png",
		"http://localhost:6000/v1",
	); err == nil {
		t.Fatal("expected unmatched private host to be rejected")
	}
}
