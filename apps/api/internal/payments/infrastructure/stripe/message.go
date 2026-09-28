package stripe

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgPrepareConfiguration              = "验证 Stripe 账户"                               // PrepareConfiguration @ adapter.go
	msgPrepareConfiguration2             = "Stripe 账户响应无效"                              // PrepareConfiguration @ adapter.go
	msgPrepareConfiguration3             = "Stripe 账户尚未启用收款"                            // PrepareConfiguration @ adapter.go
	msgPrepareConfiguration4             = "读取 Stripe Payment Method Configuration"     // PrepareConfiguration @ adapter.go
	msgPrepareConfiguration5             = "Stripe Payment Method Configuration 无效或未启用" // PrepareConfiguration @ adapter.go
	msgEnsureProduct                     = "读取 Stripe Product"                          // EnsureProduct @ adapter.go
	msgEnsureProduct2                    = "查找 Stripe Product"                          // EnsureProduct @ adapter.go
	msgEnsureProduct3                    = "创建 Stripe Product"                          // EnsureProduct @ adapter.go
	msgEnsureProduct4                    = "Stripe Product 响应无效"                        // EnsureProduct @ adapter.go
	msgUpdateProduct                     = "更新 Stripe Product"                          // UpdateProduct @ adapter.go
	msgEnsureWebhookEndpoint             = "读取 Stripe Webhook Endpoint"                 // EnsureWebhookEndpoint @ adapter.go
	msgEnsureWebhookEndpoint2            = "创建 Stripe Webhook Endpoint"                 // EnsureWebhookEndpoint @ adapter.go
	msgEnsureWebhookEndpoint3            = "Stripe Webhook Endpoint 响应无效"               // EnsureWebhookEndpoint @ adapter.go
	msgUpdateWebhookEndpoint             = "更新 Stripe Webhook Endpoint"                 // UpdateWebhookEndpoint @ adapter.go
	msgMapStripeError                    = "失败，请稍后重试"                                   // MapStripeError @ adapter.go
	msgMapStripeError2                   = "Stripe API Key 无效"                          // MapStripeError @ adapter.go
	msgMapStripeError3                   = "当前 Stripe API Key 权限不足"                     // MapStripeError @ adapter.go
	msgMapStripeError4                   = "Stripe 拒绝了当前配置"                             // MapStripeError @ adapter.go
	msgMapStripeCreateError              = " 被 Stripe 拒绝"                               // MapStripeCreateError @ adapter.go
	msgCreateCustomer                    = "创建 Stripe Customer"                         // CreateCustomer @ checkout.go
	msgCreateCustomer2                   = "Stripe Customer 响应无效"                       // CreateCustomer @ checkout.go
	msgCreateCheckoutSession             = "Stripe Payment Method Configuration 未配置"    // CreateCheckoutSession @ checkout.go
	msgCreateCheckoutSession2            = "创建 Stripe Checkout Session"                 // CreateCheckoutSession @ checkout.go
	msgRetrieveCheckoutSession           = "读取 Stripe Checkout Session"                 // RetrieveCheckoutSession @ checkout.go
	msgExpireCheckoutSession             = "关闭 Stripe Checkout Session"                 // ExpireCheckoutSession @ checkout.go
	msgCheckoutSessionFact               = "Stripe Checkout Session 响应无效"               // CheckoutSessionFact @ checkout.go
	msgCheckoutSessionFact2              = "Stripe Checkout Session 状态无效"               // CheckoutSessionFact @ checkout.go
	msgInvalidCheckoutBinding            = "Stripe Checkout Session 订单绑定无效"             // InvalidCheckoutBinding @ checkout.go
	msgInvalidCheckoutExpirationResponse = "Stripe Checkout Session 关闭结果无效"             // InvalidCheckoutExpirationResponse @ checkout.go
	msgRetrieveDispute                   = "Stripe Dispute 参数无效"                        // RetrieveDispute @ dispute.go
	msgRetrieveDispute2                  = "读取 Stripe Dispute"                          // RetrieveDispute @ dispute.go
	msgRetrieveDispute3                  = "Stripe Dispute 响应无效"                        // RetrieveDispute @ dispute.go
	msgCreateRefund                      = "Stripe Refund 参数无效"                         // CreateRefund @ refund.go
	msgCreateRefund2                     = "创建 Stripe Refund"                           // CreateRefund @ refund.go
	msgCreateRefund3                     = "Stripe Refund 响应无效"                         // CreateRefund @ refund.go
	msgRetrieveRefund                    = "读取 Stripe Refund"                           // RetrieveRefund @ refund.go
)
