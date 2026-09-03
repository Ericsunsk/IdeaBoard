package submission

// Claim is the durable idempotency fact linking one client action to its task.
// The persistence adapter derives the request fingerprint from Billing.
type Claim struct {
	UserID         int64
	ClientActionID string
	TaskID         string
	RequestHash    string
}

// Lookup identifies the submission replay that must match the original
// billing facts.
type Lookup struct {
	UserID         int64
	ClientActionID string
	RequestHash    string
}
