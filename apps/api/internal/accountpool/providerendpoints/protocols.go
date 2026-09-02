package providerendpoints

import (
	"fmt"
	"sort"
	"strings"
)

const (
	ProtocolModelAPI      = "model-api"
	ProtocolImagePipeline = "image-pipeline"
	ProtocolImageFill     = "image-fill"
	CapabilityGeneration  = "generation"
	CapabilityCutout      = "cutout"
	CapabilityUpscale     = "upscale"
	CapabilityFill        = "fill"
)

type ProtocolDefinition struct {
	ID           string   `json:"id"`
	Capabilities []string `json:"capabilities"`
}

// Runtime adapter kinds are deliberately provider-agnostic. The worker maps
// these values to concrete adapters; the registry owns the protocol contract.
const (
	RuntimeAdapterModelImage = "model-image"
	RuntimeAdapterCutout     = "cutout"
	RuntimeAdapterUpscale    = "upscale"
	RuntimeAdapterFluxFill   = "flux-fill"
)

type protocolSpec struct {
	definition      ProtocolDefinition
	allowedAPITypes []string
	adapters        map[string]string
}

var protocolSpecs = map[string]protocolSpec{
	ProtocolModelAPI: {
		definition: ProtocolDefinition{ID: ProtocolModelAPI, Capabilities: []string{CapabilityGeneration}},
		adapters: map[string]string{
			"generation": RuntimeAdapterModelImage,
			"render":     RuntimeAdapterModelImage,
			"variations": RuntimeAdapterModelImage,
			"fusion":     RuntimeAdapterModelImage,
		},
	},
	ProtocolImagePipeline: {
		definition:      ProtocolDefinition{ID: ProtocolImagePipeline, Capabilities: []string{CapabilityCutout, CapabilityUpscale}},
		allowedAPITypes: []string{"openai-compatible", "self-hosted"},
		adapters: map[string]string{
			"cutout":  RuntimeAdapterCutout,
			"upscale": RuntimeAdapterUpscale,
		},
	},
	ProtocolImageFill: {
		definition:      ProtocolDefinition{ID: ProtocolImageFill, Capabilities: []string{CapabilityFill}},
		allowedAPITypes: []string{"openai-compatible", "self-hosted"},
		adapters: map[string]string{
			"inpaint":  RuntimeAdapterFluxFill,
			"outpaint": RuntimeAdapterFluxFill,
		},
	},
}

var canonicalProtocols = []string{
	ProtocolModelAPI,
	ProtocolImagePipeline,
	ProtocolImageFill,
}

func ProtocolRegistry() []ProtocolDefinition {
	result := make([]ProtocolDefinition, 0, len(canonicalProtocols))
	for _, id := range canonicalProtocols {
		spec := protocolSpecs[id]
		result = append(result, ProtocolDefinition{ID: spec.definition.ID, Capabilities: append([]string(nil), spec.definition.Capabilities...)})
	}
	return result
}

func CanonicalProtocol(protocol string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(protocol))
	_, ok := protocolSpecs[normalized]
	return normalized, ok
}

func ProtocolSupportsAPIType(protocol, apiType string) bool {
	canonical, ok := CanonicalProtocol(protocol)
	if !ok {
		return false
	}
	apiType = strings.ToLower(strings.TrimSpace(apiType))
	if apiType == "" {
		return false
	}
	spec := protocolSpecs[canonical]
	if len(spec.allowedAPITypes) == 0 {
		switch apiType {
		case "openai-compatible", "self-hosted", "google-ai", "google-cloud":
			return true
		default:
			return false
		}
	}
	return containsCapability(spec.allowedAPITypes, apiType)
}

func RuntimeAdapterFor(protocol, operation string) (string, bool) {
	canonical, ok := CanonicalProtocol(protocol)
	if !ok {
		return "", false
	}
	spec := protocolSpecs[canonical]
	operation = strings.ToLower(strings.TrimSpace(operation))
	route, ok := spec.adapters[operation]
	return route, ok
}

func NormalizeProtocolCapabilities(protocol string, requested []string) (string, []string, error) {
	var ok bool
	protocol, ok = CanonicalProtocol(protocol)
	if !ok {
		return "", nil, fmt.Errorf("不支持的 ProviderEndpoint Protocol")
	}
	allowed := protocolSpecs[protocol].definition.Capabilities
	if protocol == ProtocolImagePipeline || protocol == ProtocolImageFill {
		if len(requested) != 1 {
			return "", nil, fmt.Errorf("图片处理 API 必须选择一个处理功能")
		}
		capability := strings.ToLower(strings.TrimSpace(requested[0]))
		if !containsCapability(allowed, capability) {
			return "", nil, fmt.Errorf("图片处理 API 必须选择一个处理功能")
		}
		return protocol, []string{capability}, nil
	}
	normalized := append([]string(nil), allowed...)
	sort.Strings(normalized)
	return protocol, normalized, nil
}

func ValidateProtocolAPIType(protocol, apiType string) error {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	apiType = strings.ToLower(strings.TrimSpace(apiType))
	if !ProtocolSupportsAPIType(protocol, apiType) {
		if canonical, ok := CanonicalProtocol(protocol); !ok || len(protocolSpecs[canonical].allowedAPITypes) == 0 {
			return nil
		}
		return fmt.Errorf("端点请求格式 %q 仅支持 self-hosted 或 openai-compatible 服务类型", protocol)
	}
	return nil
}

func containsCapability(allowed []string, requested string) bool {
	for _, capability := range allowed {
		if capability == requested {
			return true
		}
	}
	return false
}

func requestedCapabilities(capability string) []string {
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return nil
	}
	return []string{capability}
}

func ProtocolSupportsModelDetection(protocol string) bool {
	canonical, ok := CanonicalProtocol(protocol)
	return ok && canonical == ProtocolModelAPI
}

func AllowsModellessAccounts(protocol string, capabilities []string) bool {
	if len(capabilities) != 1 {
		return false
	}
	p, ok := CanonicalProtocol(protocol)
	if !ok {
		return false
	}
	return ((p == ProtocolImagePipeline) &&
		(capabilities[0] == CapabilityCutout || capabilities[0] == CapabilityUpscale)) ||
		((p == ProtocolImageFill) && capabilities[0] == CapabilityFill)
}
