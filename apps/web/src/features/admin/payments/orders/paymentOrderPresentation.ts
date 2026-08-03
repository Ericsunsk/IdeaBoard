import type { AdminPaymentOrderStatus } from './paymentOrderTypes'

type StatusTone = 'success' | 'info' | 'warn' | 'danger' | 'secondary' | 'contrast'

const lifecyclePresentation: Record<
  AdminPaymentOrderStatus,
  { label: string; severity: StatusTone }
> = {
  creating: { label: '创建中', severity: 'info' },
  checkout_open: { label: '待支付', severity: 'warn' },
  async_payment_pending: { label: '异步确认中', severity: 'info' },
  paid_pending_credit: { label: '待到账', severity: 'warn' },
  credited: { label: '已到账', severity: 'success' },
  cancelled: { label: '已取消', severity: 'secondary' },
  expired: { label: '已过期', severity: 'contrast' },
  payment_failed: { label: '支付失败', severity: 'danger' },
}

const stripeLabels: Record<string, string> = {
  creating: '创建中',
  open: '待支付',
  async_payment_pending: '异步处理中',
  paid: '已支付',
  cancelled: '已取消',
  expired: '已过期',
  payment_failed: '失败',
  unknown: '未知',
}

const creditLabels: Record<string, string> = {
  pending: '待到账',
  credited: '已到账',
  not_applicable: '未触发',
}

export const paymentOrderStatusOptions = [
  { label: '创建中', value: 'creating' },
  { label: '待支付', value: 'checkout_open' },
  { label: '异步确认中', value: 'async_payment_pending' },
  { label: '已支付待到账', value: 'paid_pending_credit' },
  { label: '已到账', value: 'credited' },
  { label: '已取消', value: 'cancelled' },
  { label: '已过期', value: 'expired' },
  { label: '支付失败', value: 'payment_failed' },
] satisfies Array<{ label: string; value: AdminPaymentOrderStatus }>

export function getPaymentOrderStatusPresentation(status: AdminPaymentOrderStatus) {
  return lifecyclePresentation[status]
}

export function getStripeStatusLabel(status: string): string {
  return stripeLabels[status] ?? (status || '未知')
}

export function getCreditStatusLabel(status: string): string {
  return creditLabels[status] ?? (status || '未知')
}

export function formatPaymentDateTime(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'short',
    timeZone: 'Asia/Shanghai',
  }).format(date)
}

export function formatPaymentMoney(amountMinor?: number, currency?: string): string {
  if (amountMinor === undefined || !Number.isSafeInteger(amountMinor) || amountMinor < 0) return '—'
  const normalizedCurrency = currency?.trim().toUpperCase()
  if (!normalizedCurrency || !/^[A-Z]{3}$/.test(normalizedCurrency)) return '—'
  const fractionDigits =
    new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: normalizedCurrency,
    }).resolvedOptions().maximumFractionDigits ?? 2
  const divisor = 10 ** fractionDigits
  return new Intl.NumberFormat('zh-CN', {
    style: 'currency',
    currency: normalizedCurrency,
    currencyDisplay: 'symbol',
  }).format(amountMinor / divisor)
}
