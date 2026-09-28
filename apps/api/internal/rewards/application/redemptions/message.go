package redemptions

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgGrantRewardCodeCredits = "兑换奖励" // GrantRewardCodeCredits @ service.go
)
