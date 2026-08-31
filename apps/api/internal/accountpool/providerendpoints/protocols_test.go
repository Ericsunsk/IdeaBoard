package providerendpoints

import "testing"

func TestNormalizeProtocolCapabilitiesEnforcesCompatibility(t *testing.T) {
	protocol, capabilities, err := NormalizeProtocolCapabilities(ProtocolImagePipeline, []string{CapabilityCutout})
	if err != nil || protocol != ProtocolImagePipeline || len(capabilities) != 1 {
		t.Fatalf("custom protocol normalization = %q, %v, %v", protocol, capabilities, err)
	}
	if capabilities[0] != CapabilityCutout {
		t.Fatalf("custom protocol capabilities = %v", capabilities)
	}
	protocol, capabilities, err = NormalizeProtocolCapabilities(ProtocolImageURLJSONImage, []string{CapabilityCutout})
	if err != nil || protocol != ProtocolImagePipeline || len(capabilities) != 1 {
		t.Fatalf("custom protocol alias normalization = %q, %v, %v", protocol, capabilities, err)
	}
	if _, _, err := NormalizeProtocolCapabilities(ProtocolImagePipeline, nil); err == nil {
		t.Fatal("image processing protocol without a selected capability should fail")
	}
	if _, _, err := NormalizeProtocolCapabilities(ProtocolImagePipeline, []string{CapabilityCutout, CapabilityUpscale}); err == nil {
		t.Fatal("image processing protocol with multiple capabilities should fail")
	}
	if _, _, err := NormalizeProtocolCapabilities(ProtocolImagePipeline, []string{CapabilityGeneration}); err == nil {
		t.Fatal("image processing protocol with an unsupported capability should fail")
	}
	protocol, capabilities, err = NormalizeProtocolCapabilities(ProtocolImageFill, []string{CapabilityFill})
	if err != nil || protocol != ProtocolImageFill || len(capabilities) != 1 || capabilities[0] != CapabilityFill {
		t.Fatalf("image fill protocol normalization = %q, %v, %v", protocol, capabilities, err)
	}
	protocol, capabilities, err = NormalizeProtocolCapabilities(ProtocolFluxFillJSONImage, []string{CapabilityFill})
	if err != nil || protocol != ProtocolImageFill || len(capabilities) != 1 || capabilities[0] != CapabilityFill {
		t.Fatalf("flux fill alias normalization = %q, %v, %v", protocol, capabilities, err)
	}
	protocol, capabilities, err = NormalizeProtocolCapabilities(ProtocolModelAPI, nil)
	if err != nil || protocol != ProtocolModelAPI || len(capabilities) != 1 || capabilities[0] != CapabilityGeneration {
		t.Fatalf("model protocol normalization = %q, %v, %v", protocol, capabilities, err)
	}
}

func TestProtocolSupportsCapabilityChecksExactCapability(t *testing.T) {
	if !ProtocolSupportsCapability(ProtocolModelAPI, CapabilityGeneration) {
		t.Fatal("model protocol should support image generation")
	}
	if ProtocolSupportsCapability(ProtocolModelAPI, CapabilityUpscale) {
		t.Fatal("model protocol should not support upscale")
	}
	if !ProtocolSupportsCapability(ProtocolImageFill, CapabilityFill) {
		t.Fatal("image fill protocol should support image fill")
	}
	if ProtocolSupportsCapability(ProtocolImageFill, CapabilityGeneration) {
		t.Fatal("image fill protocol should not support image generation")
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
	if !AllowsModellessAccounts(ProtocolImageURLJSONImage, []string{CapabilityCutout}) {
		t.Fatal("cutout endpoint should allow model-less accounts")
	}
	if AllowsModellessAccounts(ProtocolImageURLJSONImage, []string{CapabilityCutout, CapabilityUpscale}) {
		t.Fatal("combined image transform endpoint should not allow model-less accounts")
	}
	if !AllowsModellessAccounts(ProtocolFluxFillJSONImage, []string{CapabilityFill}) {
		t.Fatal("fill endpoint should allow model-less accounts")
	}
}

func TestProtocolRegistryResolvesAliasesAndRuntimeAdapters(t *testing.T) {
	canonical, ok := CanonicalProtocol(ProtocolFluxFillJSONImage)
	if !ok || canonical != ProtocolImageFill {
		t.Fatalf("canonical flux protocol = %q, %v", canonical, ok)
	}
	if !ProtocolSupportsAPIType(ProtocolImageURLJSONImage, " self-hosted ") {
		t.Fatal("image pipeline alias should accept self-hosted API type")
	}
	if ProtocolSupportsAPIType(ProtocolFluxFillJSONImage, "google-ai") {
		t.Fatal("image fill alias should reject Google API types")
	}
	route, ok := RuntimeAdapterFor(ProtocolImageURLJSONImage, "upscale")
	if !ok || route != RuntimeAdapterUpscale {
		t.Fatalf("upscale route = %q, %v", route, ok)
	}
	route, ok = RuntimeAdapterFor(ProtocolFluxFillJSONImage, "outpaint")
	if !ok || route != RuntimeAdapterFluxFill {
		t.Fatalf("flux outpaint route = %q, %v", route, ok)
	}
	if _, ok := RuntimeAdapterFor(ProtocolModelAPI, "inpaint"); ok {
		t.Fatal("model protocol must reject inpaint")
	}
}
