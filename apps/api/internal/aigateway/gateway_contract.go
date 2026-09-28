package aigateway

import (
	"fmt"
	"strings"
)

// These headers are part of the gateway's admin/probe HTTP contract. Runtime
// execution owns a separate port contract; keeping this small set local avoids
// making the business package depend on the runtime composition layer.
const (
	gatewayHeaderAuthorization = "cf-aig-authorization"
	gatewayHeaderMaxAttempts   = "cf-aig-max-attempts"
	gatewayHeaderSkipCache     = "cf-aig-skip-cache"
	gatewayHeaderModel         = "cf-aig-model"
)

// splitGatewayModel parses an admin mapping of provider/model[@gateway-path].
func splitGatewayModel(raw string) (provider, model, pathOverride string, err error) {
	rest := raw
	if i := strings.Index(rest, "@"); i >= 0 {
		rest, pathOverride = rest[:i], strings.TrimSpace(rest[i+1:])
		if pathOverride == "" || strings.ContainsAny(pathOverride, " \t\n") {
			return "", "", "", fmt.Errorf(msgSplitGatewayModel)
		}
	}
	parts := strings.SplitN(strings.TrimSpace(rest), "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" ||
		strings.ContainsAny(rest, " \t\n") {
		return "", "", "", fmt.Errorf(msgSplitGatewayModel2)
	}
	return strings.ToLower(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1]), pathOverride, nil
}
