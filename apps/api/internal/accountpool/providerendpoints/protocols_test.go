package providerendpoints

import "testing"

func TestNormalizeProtocolCapabilitiesEnforcesCompatibility(t *testing.T) {
	protocol, capabilities, err := NormalizeProtocolCapabilities(ProtocolImageURLJSONPNG, []string{CapabilityBackgroundRemoval})
	if err != nil || protocol != ProtocolImageURLJSONPNG || len(capabilities) != 1 || capabilities[0] != CapabilityBackgroundRemoval {
		t.Fatalf("custom protocol normalization = %q, %v, %v", protocol, capabilities, err)
	}
	if _, _, err := NormalizeProtocolCapabilities(ProtocolImageURLJSONPNG, []string{CapabilityImageGeneration}); err == nil {
		t.Fatal("expected incompatible capability to be rejected")
	}
	if _, _, err := NormalizeProtocolCapabilities(ProtocolModelAPI, nil); err == nil {
		t.Fatal("expected empty capability selection to be rejected")
	}
}

func TestCustomProtocolDoesNotSupportModelDetection(t *testing.T) {
	if ProtocolSupportsModelDetection(ProtocolImageURLJSONPNG) {
		t.Fatal("custom protocol must not expose model detection")
	}
	if !AllowsModellessAccounts(ProtocolImageURLJSONPNG, []string{CapabilityBackgroundRemoval}) {
		t.Fatal("background-removal endpoint should allow model-less accounts")
	}
}
