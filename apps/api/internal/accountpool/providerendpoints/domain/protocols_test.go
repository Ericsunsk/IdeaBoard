package domain

import "testing"

func TestNormalizeProtocolCapabilitiesEnforcesCompatibility(t *testing.T) {
	t.Run("valid image pipeline with cutout", func(t *testing.T) {
		protocol, capabilities, err := NormalizeProtocolCapabilities(ProtocolImagePipeline, []string{CapabilityCutout})
		assertNormalization(t, protocol, capabilities, err, ProtocolImagePipeline, CapabilityCutout)
	})

	t.Run("valid image fill", func(t *testing.T) {
		protocol, capabilities, err := NormalizeProtocolCapabilities(ProtocolImageFill, []string{CapabilityFill})
		assertNormalization(t, protocol, capabilities, err, ProtocolImageFill, CapabilityFill)
	})

	t.Run("valid default model API", func(t *testing.T) {
		protocol, capabilities, err := NormalizeProtocolCapabilities(ProtocolModelAPI, nil)
		assertNormalization(t, protocol, capabilities, err, ProtocolModelAPI, CapabilityGeneration)
	})
}

func assertNormalization(t *testing.T, protocol string, capabilities []string, err error, wantProtocol string, wantCapabilities ...string) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if protocol != wantProtocol {
		t.Fatalf("protocol = %q, want %q", protocol, wantProtocol)
	}
	if len(capabilities) != len(wantCapabilities) {
		t.Fatalf("capabilities = %v, want %v", capabilities, wantCapabilities)
	}
	for i, c := range wantCapabilities {
		if capabilities[i] != c {
			t.Fatalf("capabilities[%d] = %q, want %q", i, capabilities[i], c)
		}
	}
}

func TestNormalizeProtocolCapabilitiesRejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name         string
		protocol     string
		capabilities []string
	}{
		{"legacy image transform", "image-url-json-to-image", []string{CapabilityCutout}},
		{"missing capability", ProtocolImagePipeline, nil},
		{"multiple capabilities", ProtocolImagePipeline, []string{CapabilityCutout, CapabilityUpscale}},
		{"unsupported capability", ProtocolImagePipeline, []string{CapabilityGeneration}},
		{"legacy image fill", "flux-fill-json-image", []string{CapabilityFill}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := NormalizeProtocolCapabilities(tc.protocol, tc.capabilities); err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestProtocolSupportsCapabilityChecksExactCapability(t *testing.T) {
	model, ok := CanonicalProtocol(ProtocolModelAPI)
	if !ok || !containsCapability(protocolSpecs[model].definition.Capabilities, CapabilityGeneration) {
		t.Fatal("model protocol should support image generation")
	}
	if !ok || containsCapability(protocolSpecs[model].definition.Capabilities, CapabilityUpscale) {
		t.Fatal("model protocol should not support upscale")
	}
	fill, ok := CanonicalProtocol(ProtocolImageFill)
	if !ok || !containsCapability(protocolSpecs[fill].definition.Capabilities, CapabilityFill) {
		t.Fatal("image fill protocol should support image fill")
	}
	if !ok || containsCapability(protocolSpecs[fill].definition.Capabilities, CapabilityGeneration) {
		t.Fatal("image fill protocol should not support image generation")
	}
}

func TestValidateProtocolAPITypeSeparatesSelfHostedImageServices(t *testing.T) {
	if err := ValidateProtocolAPIType(ProtocolImageFill, "self-hosted"); err != nil {
		t.Fatalf("self-hosted image fill endpoint should be valid: %v", err)
	}
	if err := ValidateProtocolAPIType(ProtocolImagePipeline, "openai-compatible"); err != nil {
		t.Fatalf("OpenAI-compatible image endpoint should be valid: %v", err)
	}
	if err := ValidateProtocolAPIType(ProtocolImageFill, "google-cloud"); err == nil {
		t.Fatal("Google Cloud must not be accepted for the image fill request format")
	}
	if err := ValidateProtocolAPIType(ProtocolModelAPI, "google-cloud"); err != nil {
		t.Fatalf("model API should accept Google Cloud: %v", err)
	}
}

func TestCustomProtocolDoesNotSupportModelDetection(t *testing.T) {
	if ProtocolSupportsModelDetection(ProtocolImagePipeline) {
		t.Fatal("custom protocol must not expose model detection")
	}
	if !AllowsModellessAccounts(ProtocolImagePipeline, []string{CapabilityCutout}) {
		t.Fatal("cutout endpoint should allow model-less accounts")
	}
	if AllowsModellessAccounts(ProtocolImagePipeline, []string{CapabilityCutout, CapabilityUpscale}) {
		t.Fatal("combined image transform endpoint should not allow model-less accounts")
	}
	if !AllowsModellessAccounts(ProtocolImageFill, []string{CapabilityFill}) {
		t.Fatal("fill endpoint should allow model-less accounts")
	}
}

func TestProtocolRegistryResolvesCanonicalProtocols(t *testing.T) {
	canonical, ok := CanonicalProtocol(ProtocolImageFill)
	if !ok || canonical != ProtocolImageFill {
		t.Fatalf("canonical fill protocol = %q, %v", canonical, ok)
	}
	if !ProtocolSupportsAPIType(ProtocolImagePipeline, " self-hosted ") {
		t.Fatal("image pipeline should accept self-hosted API type")
	}
	if ProtocolSupportsAPIType(ProtocolImageFill, "google-ai") {
		t.Fatal("image fill should reject Google API types")
	}
	if _, ok := CanonicalProtocol("image-url-json-to-image"); ok {
		t.Fatal("legacy image transform protocol must not resolve")
	}
	if _, ok := CanonicalProtocol("flux-fill-json-image"); ok {
		t.Fatal("legacy image fill protocol must not resolve")
	}
}

func TestProtocolRegistryResolvesRuntimeAdapters(t *testing.T) {
	route, ok := RuntimeAdapterFor(ProtocolImagePipeline, "upscale")
	if !ok || route != RuntimeAdapterUpscale {
		t.Fatalf("upscale route = %q, %v", route, ok)
	}
	route, ok = RuntimeAdapterFor(ProtocolImageFill, "outpaint")
	if !ok || route != RuntimeAdapterFluxFill {
		t.Fatalf("flux outpaint route = %q, %v", route, ok)
	}
	if _, ok := RuntimeAdapterFor(ProtocolModelAPI, "inpaint"); ok {
		t.Fatal("model protocol must reject inpaint")
	}
}
