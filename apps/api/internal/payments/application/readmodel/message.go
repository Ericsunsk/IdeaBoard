package readmodel

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgListRecharges = "%w：必须在 1 到 %d 之间" // ListRecharges @ service.go
)
