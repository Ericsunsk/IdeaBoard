package domain

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgInvalidIdeaQuantity         = "充值 Idea 数量必须在 500 到 500,000 之间" // ErrInvalidIdeaQuantity
	msgTermsAcceptanceRequired     = "请先同意服务条款与退款政策"                  // ErrTermsAcceptanceRequired
	msgCheckoutIdempotencyKey      = "充值请求标识无效"                       // ErrCheckoutIdempotencyKey
	msgCheckoutIdempotencyConflict = "充值请求与原请求不一致"                    // ErrCheckoutIdempotencyConflict
	msgCheckoutRequestRejected     = "stripe 拒绝创建支付页面"                // ErrCheckoutRequestRejected
	msgPurchasesUnavailable        = "充值服务暂不可用"                       // ErrPurchasesUnavailable
	msgWalletNotRechargeable       = "当前账户暂不可充值"                      // ErrWalletNotRechargeable
)
