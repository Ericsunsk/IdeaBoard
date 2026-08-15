package providerendpoints

import "testing"

func TestNormalizeProtocolCapabilitiesEnforcesCompatibility(t *testing.T) {
	protocol, capabilities, err := NormalizeProtocolCapabilities(ProtocolImageURLJSONImage, []string{CapabilityBackgroundRemoval})
	if err != nil || protocol != ProtocolImageURLJSONImage || len(capabilities) != 1 {
		t.Fatalf("custom protocol normalization = %q, %v, %v", protocol, capabilities, err)
	}
	if capabilities[0] != CapabilityBackgroundRemoval {
		t.Fatalf("custom protocol capabilities = %v", capabilities)
	}
	if _, _, err := NormalizeProtocolCapabilities(ProtocolImageURLJSONImage, nil); err == nil {
		t.Fatal("image processing protocol without a selected capability should fail")
	}
	if _, _, err := NormalizeProtocolCapabilities(ProtocolImageURLJSONImage, []string{CapabilityBackgroundRemoval, CapabilityImageUpscale}); err == nil {
		t.Fatal("image processing protocol with multiple capabilities should fail")
	}
	if _, _, err := NormalizeProtocolCapabilities(ProtocolImageURLJSONImage, []string{CapabilityImageGeneration}); err == nil {
		t.Fatal("image processing protocol with an unsupported capability should fail")
	}
	protocol, capabilities, err = NormalizeProtocolCapabilities(ProtocolModelAPI, nil)
	if err != nil || protocol != ProtocolModelAPI || len(capabilities) != 1 || capabilities[0] != CapabilityImageGeneration {
		t.Fatalf("model protocol normalization = %q, %v, %v", protocol, capabilities, err)
	}
}

func TestProtocolSupportsCapabilityChecksExactCapability(t *testing.T) {
	if !ProtocolSupportsCapability(ProtocolModelAPI, CapabilityImageGeneration) {
		t.Fatal("model protocol should support image generation")
	}
	if ProtocolSupportsCapability(ProtocolModelAPI, CapabilityImageUpscale) {
		t.Fatal("model protocol should not support image upscale")
	}
}

func TestCustomProtocolDoesNotSupportModelDetection(t *testing.T) {
	if ProtocolSupportsModelDetection(ProtocolImageURLJSONImage) {
		t.Fatal("custom protocol must not expose model detection")
	}
	if !AllowsModellessAccounts(ProtocolImageURLJSONImage, []string{CapabilityBackgroundRemoval}) {
		t.Fatal("background-removal endpoint should allow model-less accounts")
	}
	if AllowsModellessAccounts(ProtocolImageURLJSONImage, []string{CapabilityBackgroundRemoval, CapabilityImageUpscale}) {
		t.Fatal("combined image transform endpoint should not allow model-less accounts")
	}
}
