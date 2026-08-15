package providerendpoints

import "testing"

func TestNormalizeProtocolCapabilitiesEnforcesCompatibility(t *testing.T) {
	protocol, capabilities, err := NormalizeProtocolCapabilities(ProtocolImageURLJSONImage, []string{CapabilityBackgroundRemoval})
	if err != nil || protocol != ProtocolImageURLJSONImage || len(capabilities) != 1 || capabilities[0] != CapabilityBackgroundRemoval {
		t.Fatalf("custom protocol normalization = %q, %v, %v", protocol, capabilities, err)
	}
	if _, _, err := NormalizeProtocolCapabilities(ProtocolImageURLJSONImage, []string{CapabilityImageGeneration}); err == nil {
		t.Fatal("expected incompatible capability to be rejected")
	}
	if _, _, err := NormalizeProtocolCapabilities(ProtocolModelAPI, nil); err == nil {
		t.Fatal("expected empty capability selection to be rejected")
	}
}

func TestCustomProtocolDoesNotSupportModelDetection(t *testing.T) {
	if ProtocolSupportsModelDetection(ProtocolImageURLJSONImage) {
		t.Fatal("custom protocol must not expose model detection")
	}
	if !AllowsModellessAccounts(ProtocolImageURLJSONImage, []string{CapabilityBackgroundRemoval}) {
		t.Fatal("background-removal endpoint should allow model-less accounts")
	}
	if !AllowsModellessAccounts(ProtocolImageURLJSONImage, []string{CapabilityBackgroundRemoval, CapabilityImageUpscale}) {
		t.Fatal("combined image transform endpoint should allow model-less accounts")
	}
}
