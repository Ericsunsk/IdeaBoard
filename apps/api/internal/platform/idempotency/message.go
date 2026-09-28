package idempotency

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgRequired  = "缺少幂等键，请重试此次操作" // CodeRequired
	msgRequired2 = "幂等键无效，请重试此次操作" // CodeRequired
	msgConflict  = "幂等请求与原请求不一致"   // CodeConflict
)
