import { formatLocaleNumber } from '../../../../shared/i18n/formatters'
import type { AdminBillableItemCard, PricingVariant } from './billableCatalogTypes'

// 定价表各列定宽（单源，调列宽只改这里；对齐任务监控页 columns.ts 惯例）。
export const billingPricingColumnWidths = {
  service: '200px',
  status: '105px',
  variant: '96px',
  pool: '230px',
  actions: '120px',
  enabled: '72px',
} as const

// 四列平铺：variant 不在功能档位内显示 —（不适用），在档内未定价显示未设置。
export function formatPricingCellValue(
  item: AdminBillableItemCard,
  variant: PricingVariant,
): string {
  if (!item.pricingVariants.includes(variant)) return '—'
  const price = item.currentPrices[variant]
  return price ? formatLocaleNumber(price.unitCredits) : '未设置'
}
