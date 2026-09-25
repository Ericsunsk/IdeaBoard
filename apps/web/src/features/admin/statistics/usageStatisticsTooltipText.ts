// Statistics tooltip 文案的数字格式单源：调用量取整数千分位，百分比保留一位小数。
// Why: 4 处 tooltip 回调之前手写 `toLocaleString()` / `toFixed(1) + '%'`，格式分散；
// 收敛到这里后，有限输入的渲染字符串与原来逐字节一致（由 contract spec 锁定），
// NaN/null/undefined 防御性地映射为零值（tooltip 入参上游已做非负归一，实际走不到该分支）。
import { formatLocaleNumber } from '../../../shared/i18n/formatters'

export function formatCount(value: number | null | undefined): string {
  const rounded = Math.round(Number(value) || 0)
  return formatLocaleNumber(rounded)
}

export function formatPercent1(value: number | null | undefined): string {
  const parsed = Number(value) || 0
  return `${parsed.toFixed(1)}%`
}
