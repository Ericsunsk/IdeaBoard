// Statistics tooltip 文案的数字格式单源：调用量取整数千分位，百分比保留一位小数。
// Why: 4 处 tooltip 回调之前手写 `toLocaleString()` / `toFixed(1) + '%'`，格式分散；
// 收敛到这里后，有限输入的渲染字符串与原来逐字节一致（由 contract spec 锁定），
// 非法输入（NaN/null/undefined/非数字）显式渲染为 `—`，不得静默归零伪造数据。
import { formatLocaleNumber } from '../../../../core/i18n/formatters'

const INVALID_STATISTIC_TEXT = '—'

function toFiniteStatisticNumber(value: number | null | undefined): number | null {
  if (value === null || value === undefined) return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

export function formatCount(value: number | null | undefined): string {
  const parsed = toFiniteStatisticNumber(value)
  if (parsed === null) return INVALID_STATISTIC_TEXT
  return formatLocaleNumber(Math.round(parsed))
}

export function formatPercent1(value: number | null | undefined): string {
  const parsed = toFiniteStatisticNumber(value)
  if (parsed === null) return INVALID_STATISTIC_TEXT
  return `${parsed.toFixed(1)}%`
}
