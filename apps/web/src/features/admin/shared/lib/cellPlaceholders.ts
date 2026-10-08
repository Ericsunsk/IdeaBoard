// Admin 单元格空态占位单源：数字表用 —，标识类用 -。
// pricing 的「未设置 / —」是业务语义区分（档内未定价 vs 档位不适用），
// 见 billingPricingColumns.ts，不在此收敛，语义不明的 cell 一律跳过不迁。
export const ADMIN_NUMERIC_EMPTY = '—'

export const ADMIN_IDENTITY_EMPTY = '-'

export type AdminEmptyKind = 'numeric' | 'identity'

/**
 * 统一空态占位：null / undefined / 空串回占位符，其余原样转字符串。
 * 注意 `||` 语义差异：0 会穿透返回 '0'（数字 0 是有效值，不是空态）。
 */
export function emptyPlaceholder(value: unknown, kind: AdminEmptyKind = 'identity'): string {
  if (value === null || value === undefined || value === '') {
    return kind === 'numeric' ? ADMIN_NUMERIC_EMPTY : ADMIN_IDENTITY_EMPTY
  }
  return String(value)
}
