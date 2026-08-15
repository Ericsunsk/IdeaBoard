package providerendpoints

import (
	"fmt"
	"sort"
	"strings"
)

const (
	ProtocolModelAPI            = "model-api"
	ProtocolImageURLJSONImage   = "image-url-json-to-image"
	CapabilityImageGeneration   = "image-generation"
	CapabilityBackgroundRemoval = "background-removal"
	CapabilityImageUpscale      = "image-upscale"
)

type ProtocolDefinition struct {
	ID           string   `json:"id"`
	Capabilities []string `json:"capabilities"`
}

var protocolCapabilities = map[string][]string{
	ProtocolModelAPI:          {CapabilityImageGeneration},
	ProtocolImageURLJSONImage: {CapabilityBackgroundRemoval, CapabilityImageUpscale},
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

func NormalizeProtocolCapabilities(protocol string, capabilities []string) (string, []string, error) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	allowed, ok := protocolCapabilities[protocol]
	if !ok {
		return "", nil, fmt.Errorf("不支持的 ProviderEndpoint Protocol")
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, capability := range allowed {
		allowedSet[capability] = struct{}{}
	}
	seen := make(map[string]struct{}, len(capabilities))
	normalized := make([]string, 0, len(capabilities))
	for _, raw := range capabilities {
		capability := strings.ToLower(strings.TrimSpace(raw))
		if capability == "" {
			continue
		}
		if _, ok := allowedSet[capability]; !ok {
			return "", nil, fmt.Errorf("Protocol %q 不支持 Capability %q", protocol, capability)
		}
		if _, ok := seen[capability]; ok {
			continue
		}
		seen[capability] = struct{}{}
		normalized = append(normalized, capability)
	}
	if len(normalized) == 0 {
		return "", nil, fmt.Errorf("ProviderEndpoint 至少需要一个 Capability")
	}
	sort.Strings(normalized)
	return protocol, normalized, nil
}

func ProtocolSupportsModelDetection(protocol string) bool {
	return strings.TrimSpace(protocol) == ProtocolModelAPI
}

func ProtocolSupportsCapability(protocol, capability string) bool {
	_, _, err := NormalizeProtocolCapabilities(protocol, []string{capability})
	return err == nil
}

func AllowsModellessAccounts(protocol string, capabilities []string) bool {
	if strings.TrimSpace(protocol) != ProtocolImageURLJSONImage || len(capabilities) == 0 {
		return false
	}
	for _, capability := range capabilities {
		if capability == CapabilityBackgroundRemoval || capability == CapabilityImageUpscale {
			return true
		}
	}
	return false
}
