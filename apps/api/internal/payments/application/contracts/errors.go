package contracts

import (
	"errors"

	paymentports "ideaboard/api/internal/payments/ports"
)

// Application errors are shared by services and HTTP adapters. Keeping the
// sentinels here lets adapters map stable outcomes without importing the
// payments root package.
var (
	ErrPaymentOrderNotFound     = errors.New(msgPaymentOrderNotFound)
	ErrPendingCheckoutLimit     = errors.New(msgPendingCheckoutLimit)
	ErrPaymentAlreadyInProgress = errors.New(msgPaymentAlreadyInProgress)
	ErrPaymentAlreadyCompleted  = errors.New(msgPaymentAlreadyCompleted)
	ErrCheckoutSessionExpired   = errors.New(msgCheckoutSessionExpired)

	ErrRechargeHistoryCursor = errors.New(msgRechargeHistoryCursor)
	ErrRechargeHistoryLimit  = errors.New(msgRechargeHistoryLimit)

	ErrAdminPaymentOrderFilter   = errors.New(msgAdminPaymentOrderFilter)
	ErrAdminPaymentOrderCursor   = errors.New(msgAdminPaymentOrderCursor)
	ErrAdminPaymentOrderNotFound = errors.New(msgAdminPaymentOrderNotFound)

	ErrStripeConfigurationNotFound       = errors.New(msgStripeConfigurationNotFound)
	ErrStripeAPIKeyRequired              = errors.New(msgStripeAPIKeyRequired)
	ErrInvalidStripeAPIKey               = paymentports.ErrInvalidStripeAPIKey
	ErrInvalidPaymentMethodConfiguration = errors.New(msgInvalidPaymentMethodConfiguration)
	ErrInvalidProductTaxCode             = errors.New(msgInvalidProductTaxCode)
	ErrInvalidWebhookURL                 = errors.New(msgInvalidWebhookURL)
	ErrStripePermissionDenied            = errors.New(msgStripePermissionDenied)
	ErrStripeAccountUnavailable          = errors.New(msgStripeAccountUnavailable)
	ErrStripeCapabilityUnavailable       = errors.New(msgStripeCapabilityUnavailable)
	ErrStripeProviderUnavailable         = errors.New(msgStripeProviderUnavailable)
	ErrStripeConfigurationInvalid        = errors.New(msgStripeConfigurationInvalid)
	ErrStripeConfigurationChanged        = errors.New("stripe configuration changed")

	ErrRefundUnavailable      = errors.New(msgRefundUnavailable)
	ErrRefundOrderNotEligible = errors.New(msgRefundOrderNotEligible)
	ErrRefundAmountInvalid    = errors.New(msgRefundAmountInvalid)
	ErrRefundAmountExceeds    = errors.New(msgRefundAmountExceeds)
	ErrRefundIdempotency      = errors.New(msgRefundIdempotency)
	ErrRefundProviderConflict = errors.New(msgRefundProviderConflict)

	ErrDisputeUnavailable      = errors.New(msgDisputeUnavailable)
	ErrDisputeOrderNotEligible = errors.New(msgDisputeOrderNotEligible)
	ErrDisputeProviderConflict = errors.New(msgDisputeProviderConflict)
	ErrDisputeTransition       = errors.New(msgDisputeTransition)
)

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgPaymentOrderNotFound              = "充值订单不存在"                                    // ErrPaymentOrderNotFound
	msgPendingCheckoutLimit              = "最多同时保留 3 笔待支付订单，请先完成或放弃一笔"                  // ErrPendingCheckoutLimit
	msgPaymentAlreadyInProgress          = "该数量的支付正在处理中，请等待支付结果"                        // ErrPaymentAlreadyInProgress
	msgPaymentAlreadyCompleted           = "该充值已完成，请勿重复支付"                              // ErrPaymentAlreadyCompleted
	msgCheckoutSessionExpired            = "支付页面已过期，请重新发起充值"                            // ErrCheckoutSessionExpired
	msgRechargeHistoryCursor             = "充值记录游标无效"                                   // ErrRechargeHistoryCursor
	msgRechargeHistoryLimit              = "充值记录 limit 无效"                              // ErrRechargeHistoryLimit
	msgAdminPaymentOrderFilter           = "支付订单筛选条件无效"                                 // ErrAdminPaymentOrderFilter
	msgAdminPaymentOrderCursor           = "支付订单游标无效"                                   // ErrAdminPaymentOrderCursor
	msgAdminPaymentOrderNotFound         = "支付订单不存在"                                    // ErrAdminPaymentOrderNotFound
	msgStripeConfigurationNotFound       = "尚未完成 Stripe 配置"                             // ErrStripeConfigurationNotFound
	msgStripeAPIKeyRequired              = "请填写 Stripe API Key"                         // ErrStripeAPIKeyRequired
	msgInvalidPaymentMethodConfiguration = "无效的 Stripe Payment Method Configuration ID" // ErrInvalidPaymentMethodConfiguration
	msgInvalidProductTaxCode             = "无效的 Stripe Product Tax Code 格式"             // ErrInvalidProductTaxCode
	msgInvalidWebhookURL                 = "无效的 Stripe Webhook URL"                     // ErrInvalidWebhookURL
	msgStripePermissionDenied            = "当前 Stripe API Key 权限不足"                     // ErrStripePermissionDenied
	msgStripeAccountUnavailable          = "当前 Stripe 账户不可用"                            // ErrStripeAccountUnavailable
	msgStripeCapabilityUnavailable       = "当前 Stripe 账户暂不具备收款能力"                       // ErrStripeCapabilityUnavailable
	msgStripeProviderUnavailable         = "当前 Stripe 服务暂不可用"                           // ErrStripeProviderUnavailable
	msgStripeConfigurationInvalid        = "收到的 Stripe 配置不完整"                           // ErrStripeConfigurationInvalid
	msgRefundUnavailable                 = "退款服务暂不可用"                                   // ErrRefundUnavailable
	msgRefundOrderNotEligible            = "支付订单当前不满足退款条件"                              // ErrRefundOrderNotEligible
	msgRefundAmountInvalid               = "退款 Idea 数量无效"                               // ErrRefundAmountInvalid
	msgRefundAmountExceeds               = "退款 Idea 数量超过可退余额"                           // ErrRefundAmountExceeds
	msgRefundIdempotency                 = "退款幂等键无效"                                    // ErrRefundIdempotency
	msgRefundProviderConflict            = "检测到 Stripe Refund identity 冲突"              // ErrRefundProviderConflict
	msgDisputeUnavailable                = "支付争议服务暂不可用"                                 // ErrDisputeUnavailable
	msgDisputeOrderNotEligible           = "支付争议无法关联有效支付订单"                             // ErrDisputeOrderNotEligible
	msgDisputeProviderConflict           = "检测到 Stripe Dispute identity 冲突"             // ErrDisputeProviderConflict
	msgDisputeTransition                 = "无效的 Dispute 状态迁移"                           // ErrDisputeTransition
)
