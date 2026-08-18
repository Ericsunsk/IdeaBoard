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
	protocol, capabilities, err = NormalizeProtocolCapabilities(ProtocolFluxFillJSONImage, []string{CapabilityImageFill})
	if err != nil || protocol != ProtocolFluxFillJSONImage || len(capabilities) != 1 || capabilities[0] != CapabilityImageFill {
		t.Fatalf("flux fill protocol normalization = %q, %v, %v", protocol, capabilities, err)
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
	if !ProtocolSupportsCapability(ProtocolFluxFillJSONImage, CapabilityImageFill) {
		t.Fatal("flux fill protocol should support image fill")
	}
	if ProtocolSupportsCapability(ProtocolFluxFillJSONImage, CapabilityImageGeneration) {
		t.Fatal("flux fill protocol should not support image generation")
	}
}

func TestValidateProtocolAPITypeSeparatesSelfHostedImageServices(t *testing.T) {
	if err := ValidateProtocolAPIType(ProtocolFluxFillJSONImage, "self-hosted"); err != nil {
		t.Fatalf("self-hosted flux fill endpoint should be valid: %v", err)
	}
	if err := ValidateProtocolAPIType(ProtocolImageURLJSONImage, "openai-compatible"); err != nil {
		t.Fatalf("legacy OpenAI-compatible image endpoint should remain valid: %v", err)
	}
	if err := ValidateProtocolAPIType(ProtocolFluxFillJSONImage, "google-cloud"); err == nil {
		t.Fatal("Google Cloud must not be accepted for the flux fill request format")
	}
	if err := ValidateProtocolAPIType(ProtocolModelAPI, "google-cloud"); err != nil {
		t.Fatalf("model API should accept Google Cloud: %v", err)
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
	if !AllowsModellessAccounts(ProtocolFluxFillJSONImage, []string{CapabilityImageFill}) {
		t.Fatal("image-fill endpoint should allow model-less accounts")
	}
}
