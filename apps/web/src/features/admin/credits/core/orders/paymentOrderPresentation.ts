import { formatMinorCurrencyValue } from '@/core/utils/currency'
import { formatLocaleDate } from '@/core/i18n/formatters'
import { parseDateGuard } from '@/core/datetime/parse'
import { SHANGHAI_TIME_ZONE } from '@/core/datetime/shanghai'
import type { AdminPaymentOrderStatus } from './paymentOrderTypes'

// Pill 直连：与 StatusBadge `tone` 同构，模块内自有定义以免依赖组件层。
type PaymentOrderStatusTone = 'success' | 'danger' | 'warning' | 'neutral' | 'info'

const lifecyclePresentation: Record<
  AdminPaymentOrderStatus,
  { label: string; tone: PaymentOrderStatusTone }
> = {
  creating: { label: '创建中', tone: 'info' },
  checkout_open: { label: '待支付', tone: 'warning' },
  async_payment_pending: { label: '异步确认中', tone: 'info' },
  paid_pending_credit: { label: '待到账', tone: 'warning' },
  credited: { label: '已到账', tone: 'success' },
  cancelled: { label: '已取消', tone: 'neutral' },
  expired: { label: '已过期', tone: 'neutral' },
  payment_failed: { label: '支付失败', tone: 'danger' },
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

export const ORDER_STATUS_FILTER_ALL = 'all' as const

export type PaymentOrderStatusFilter = typeof ORDER_STATUS_FILTER_ALL | AdminPaymentOrderStatus

export const paymentOrderStatusOptions = [
  { label: '全部状态', value: ORDER_STATUS_FILTER_ALL },
  { label: '创建中', value: 'creating' },
  { label: '待支付', value: 'checkout_open' },
  { label: '异步确认中', value: 'async_payment_pending' },
  { label: '已支付待到账', value: 'paid_pending_credit' },
  { label: '已到账', value: 'credited' },
  { label: '已取消', value: 'cancelled' },
  { label: '已过期', value: 'expired' },
  { label: '支付失败', value: 'payment_failed' },
] satisfies Array<{ label: string; value: PaymentOrderStatusFilter }>

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
  const date = parseDateGuard(value ?? null)
  if (!date) return '—'
  return formatLocaleDate(
    date,
    {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: SHANGHAI_TIME_ZONE,
    },
    'zh-CN',
  )
}

export function formatPaymentMoney(amountMinor?: number, currency?: string): string {
  // Payment tables degrade to an em dash instead of throwing; the exact
  // BigInt math lives in the shared currency helper (float division here
  // used to lose minor units past MAX_SAFE_INTEGER).
  try {
    if (amountMinor === undefined || currency === undefined) return '—'
    return formatMinorCurrencyValue(amountMinor, currency, 'zh-CN')
  } catch {
    return '—'
  }
}
