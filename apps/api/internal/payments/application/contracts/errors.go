package contracts

import (
	"errors"

	paymentports "ideaboard/api/internal/payments/ports"
)

// Application errors are shared by services and HTTP adapters. Keeping the
// sentinels here lets adapters map stable outcomes without importing the
// payments root package.
var (
	ErrPaymentOrderNotFound     = errors.New("充值订单不存在")
	ErrPendingCheckoutLimit     = errors.New("最多同时保留 3 笔待支付订单，请先完成或放弃一笔")
	ErrPaymentAlreadyInProgress = errors.New("该数量的支付正在处理中，请等待支付结果")
	ErrPaymentAlreadyCompleted  = errors.New("该充值已完成，请勿重复支付")
	ErrCheckoutSessionExpired   = errors.New("支付页面已过期，请重新发起充值")

	ErrRechargeHistoryCursor = errors.New("充值记录游标无效")
	ErrRechargeHistoryLimit  = errors.New("充值记录 limit 无效")

	ErrAdminPaymentOrderFilter   = errors.New("支付订单筛选条件无效")
	ErrAdminPaymentOrderCursor   = errors.New("支付订单游标无效")
	ErrAdminPaymentOrderNotFound = errors.New("支付订单不存在")

	ErrStripeConfigurationNotFound       = errors.New("尚未完成 Stripe 配置")
	ErrStripeAPIKeyRequired              = errors.New("请填写 Stripe API Key")
	ErrInvalidStripeAPIKey               = paymentports.ErrInvalidStripeAPIKey
	ErrInvalidPaymentMethodConfiguration = errors.New("无效的 Stripe Payment Method Configuration ID")
	ErrInvalidProductTaxCode             = errors.New("无效的 Stripe Product Tax Code 格式")
	ErrInvalidWebhookURL                 = errors.New("无效的 Stripe Webhook URL")
	ErrStripePermissionDenied            = errors.New("当前 Stripe API Key 权限不足")
	ErrStripeAccountUnavailable          = errors.New("当前 Stripe 账户不可用")
	ErrStripeCapabilityUnavailable       = errors.New("当前 Stripe 账户暂不具备收款能力")
	ErrStripeProviderUnavailable         = errors.New("当前 Stripe 服务暂时不可用")
	ErrStripeConfigurationInvalid        = errors.New("收到的 Stripe 配置不完整")
	ErrStripeConfigurationChanged        = errors.New("stripe configuration changed")

	ErrRefundUnavailable      = errors.New("退款服务暂不可用")
	ErrRefundOrderNotEligible = errors.New("支付订单当前不满足退款条件")
	ErrRefundExpired          = errors.New("支付订单已超过 14 天退款期限")
	ErrRefundAmountInvalid    = errors.New("退款 Idea 数量无效")
	ErrRefundAmountExceeds    = errors.New("退款 Idea 数量超过可退余额")
	ErrRefundIdempotency      = errors.New("退款幂等键无效")
	ErrRefundProviderConflict = errors.New("检测到 Stripe Refund identity 冲突")

	ErrDisputeUnavailable      = errors.New("支付争议服务暂不可用")
	ErrDisputeOrderNotEligible = errors.New("支付争议无法关联有效支付订单")
	ErrDisputeProviderConflict = errors.New("检测到 Stripe Dispute identity 冲突")
	ErrDisputeTransition       = errors.New("无效的 Dispute 状态迁移")
)
