package principal

// Control Plane family labels for the Control Plane audit record
// (issue #808 follow-up: admin-only convergence).
//
// Production sessions mint no control capabilities at login
// (identity/auth/middleware.go builds principals via New with an empty
// capability set, and NewExtended has no non-test callers), so the
// Control Plane guard enforces the admin business role plus the
// human/worker subject gate directly. The family strings below survive
// only as audit labels (ControlAuditRecord.Capability) and as the
// openapi.control.yaml x-capabilities mirror; they grant nothing.
// Construction (New/NewExtended) still rejects control:* capabilities
// for every non-worker, non-admin-human subject, so a public capability
// can never imply a control label.

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
