// Colocated admin messages for Stripe settings (zh-only, no `t()` wrapping).
//
// Copy Policy (`core/CONTEXT.md`): admin/ops surfaces keep their literals next
// to the feedback factory that owns them instead of locale bundles. This module
// is the pattern example: one literal per toast, messages kept identical.

export const PAYMENTS_MESSAGES = {
  stripeSettingsSaved: 'Stripe 设置已保存',
  // Stripe settings dialog.
  stripeHeader: 'Stripe 设置',
  stripeDescription: '配置 Stripe 密钥与税务编码后保存。',
  stripeLoading: '加载 Stripe 设置',
  accountLabel: '账户',
  accountUnset: '尚未配置',
  modeLabel: '模式',
  productReadyState: (ready: boolean) => (ready ? 'Product Ready' : 'Product Not ready'),
  webhookReadyState: (ready: boolean) => (ready ? 'Webhook Ready' : 'Webhook Not ready'),
  apiKeyHint: '支持 sk_ 或 rk_ 格式。',
  taxCodePlaceholder: '例如：txcd_10105001',
  purchasesLabel: '充值开放',
  purchasesState: (enabled: boolean) => (enabled ? '已开放' : '已关闭'),
  saveSettings: '保存',
  apiKeyRequired: '请输入 Stripe API Key',
  pmcRequired: '请输入 Payment Method Configuration ID',
  pmcInvalid: '请输入有效的 pmc_... ID',
  productTaxCodeRequired: '请输入 Product Tax Code',
  apiKeyPlaceholder: (configured: boolean) =>
    configured ? '已配置，留空则保持不变' : '输入 Stripe API Key',
  stripeLoadFailed: '加载 Stripe 设置失败，请重试',
  stripeSaveFailed: '保存 Stripe 设置失败，请重试',
} as const
