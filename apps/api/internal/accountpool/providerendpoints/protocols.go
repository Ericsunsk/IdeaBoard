package providerendpoints

import (
	"fmt"
	"sort"
	"strings"
)

const (
	ProtocolModelAPI            = "model-api"
	ProtocolImageURLJSONImage   = "image-url-json-to-image"
	ProtocolFluxFillJSONImage   = "flux-fill-json-image"
	CapabilityImageGeneration   = "image-generation"
	CapabilityBackgroundRemoval = "background-removal"
	CapabilityImageUpscale      = "image-upscale"
	CapabilityImageFill         = "image-fill"
)

type ProtocolDefinition struct {
	ID           string   `json:"id"`
	Capabilities []string `json:"capabilities"`
}

var protocolCapabilities = map[string][]string{
	ProtocolModelAPI:          {CapabilityImageGeneration},
	ProtocolImageURLJSONImage: {CapabilityBackgroundRemoval, CapabilityImageUpscale},
	ProtocolFluxFillJSONImage: {CapabilityImageFill},
}

func ProtocolRegistry() []ProtocolDefinition {
	ids := make([]string, 0, len(protocolCapabilities))
	for id := range protocolCapabilities {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]ProtocolDefinition, 0, len(ids))
	for _, id := range ids {
		result = append(result, ProtocolDefinition{ID: id, Capabilities: append([]string(nil), protocolCapabilities[id]...)})
	}
	return result
}

func NormalizeProtocolCapabilities(protocol string, requested []string) (string, []string, error) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	allowed, ok := protocolCapabilities[protocol]
	if !ok {
		return "", nil, fmt.Errorf("不支持的 ProviderEndpoint Protocol")
	}
	if protocol == ProtocolImageURLJSONImage || protocol == ProtocolFluxFillJSONImage {
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
	return strings.TrimSpace(protocol) == ProtocolModelAPI
}

func ProtocolSupportsCapability(protocol, capability string) bool {
	allowed, ok := protocolCapabilities[strings.ToLower(strings.TrimSpace(protocol))]
	return ok && containsCapability(allowed, strings.ToLower(strings.TrimSpace(capability)))
}

func AllowsModellessAccounts(protocol string, capabilities []string) bool {
	if len(capabilities) != 1 {
		return false
	}
	return (strings.TrimSpace(protocol) == ProtocolImageURLJSONImage &&
		(capabilities[0] == CapabilityBackgroundRemoval || capabilities[0] == CapabilityImageUpscale)) ||
		(strings.TrimSpace(protocol) == ProtocolFluxFillJSONImage && capabilities[0] == CapabilityImageFill)
}
