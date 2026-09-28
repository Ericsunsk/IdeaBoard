package ports

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgInvalidStripeAPIKey     = "无效的 Stripe API Key：请使用 sk_ 或 rk_ 开头的对应环境 Key" // ErrInvalidStripeAPIKey
	msgInvalidWebhookSignature = "无效的 Stripe Webhook 签名"                        // ErrInvalidWebhookSignature
	msgWebhookSignatureExpired = "检测到 Stripe Webhook 签名已过期"                     // ErrWebhookSignatureExpired
	msgMalformedWebhookPayload = "无效的 Stripe Webhook 数据"                        // ErrMalformedWebhookPayload
)
