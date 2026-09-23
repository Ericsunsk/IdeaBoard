package principal

// Operator role grant for the Control Plane (issue #808).
//
// Construction (New/NewExtended) still rejects control:* capabilities for
// every non-internal_worker subject, so a public capability can never
// imply a control capability. Operators — human_session principals whose
// business role is admin — receive explicit control capabilities through
// WithControlGrant (the role grant), never implicitly. Control handlers
// authorize with HasControlCapability/RequireControlCapability, never
// with the legacy IsAdmin role check.

const (
	// ControlWildcard implies every Control Plane family capability.
	ControlWildcard = "control:"
	// ControlWildcardStar is the literal wildcard grant stored on
	// operator principals minted via WithControlGrant.
	ControlWildcardStar = "control:*"
)

// Control family capabilities (frozen; mirrored in openapi.control.yaml
// x-capabilities and the Control audit record).
const (
	ControlCapabilityUsers      = "control:users"
	ControlCapabilityWorkspaces = "control:workspaces"
	ControlCapabilityBilling    = "control:billing"
	ControlCapabilityCredits    = "control:credits"
	ControlCapabilityPayments   = "control:payments"
	ControlCapabilityPlatform   = "control:platform"
	ControlCapabilityTasks      = "control:tasks"
)

// ControlCapabilities lists every Control Plane family capability.
func ControlCapabilities() []string {
	return []string{
		ControlCapabilityUsers,
		ControlCapabilityWorkspaces,
		ControlCapabilityBilling,
		ControlCapabilityCredits,
		ControlCapabilityPayments,
		ControlCapabilityPlatform,
		ControlCapabilityTasks,
	}
}

// WithControlGrant returns a copy of p carrying explicit Control Plane
// capabilities via the operator role grant. It fails closed unless p is a
// valid human_session principal with the admin business role: service
// accounts, API keys, OAuth apps, internal workers, and non-admin users
// can never receive the grant here (internal workers are minted with
// their control capabilities at construction instead).
//
// The granted set is the literal ControlWildcardStar entry. Capability
// checks must go through HasControlCapability, which honors the
// wildcard, so least-privilege operators holding individual family
// capabilities keep working without a wildcard.
func WithControlGrant(p Principal) (Principal, error) {
	if !p.IsValid() {
		return Principal{}, ErrInvalid
	}
	if p.SubjectType() != SubjectHumanSession || p.Role() != RoleAdmin {
		return Principal{}, ErrInvalid
	}
	copy := p
	capabilities := make(map[string]struct{}, len(p.capabilities)+1)
	for capability := range p.capabilities {
		capabilities[capability] = struct{}{}
	}
	capabilities[ControlWildcardStar] = struct{}{}
	copy.capabilities = capabilities
	if !copy.IsValid() {
		return Principal{}, ErrInvalid
	}
	return copy, nil
}

// HasControlCapability reports whether p holds a Control Plane family
// capability, either directly or via the control:* wildcard grant.
// Public subjects never hold either (enforced at construction and by
// WithControlGrant), so this check fails closed for them.
func HasControlCapability(p Principal, capability string) bool {
	if p.HasCapability(capability) {
		return true
	}
	return p.HasCapability(ControlWildcardStar)
}
