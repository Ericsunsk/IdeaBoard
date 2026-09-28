export interface PricingNamespaceTable {
  namespace: string
  title: string
}

// 定价分表单源：按 SKU namespace（Image/Other），新业务上线加一行。
// 后缀级 AI 生图/CV 拆分已退役：粒度太细且要人维护名单。
export const PRICING_NAMESPACE_TABLES: readonly PricingNamespaceTable[] = [
  { namespace: 'ai-image', title: 'Image' },
]

export const PRICING_OTHER_TABLE: PricingNamespaceTable = {
  namespace: 'other',
  title: 'Other',
}

function pricingNamespaceOfSku(sku: string): string {
  const cut = sku.indexOf(':')
  return cut > 0 ? sku.slice(0, cut) : ''
}

export function pricingTableKeyOfSku(sku: string): string {
  const namespace = pricingNamespaceOfSku(sku)
  const table = PRICING_NAMESPACE_TABLES.find((candidate) => candidate.namespace === namespace)
  return table ? table.namespace : PRICING_OTHER_TABLE.namespace
}

// 排序键：namespace 按表序，未知沉底。
export function pricingNamespaceRank(sku: string): number {
  const namespace = pricingNamespaceOfSku(sku)
  const index = PRICING_NAMESPACE_TABLES.findIndex((table) => table.namespace === namespace)
  return index >= 0 ? index : Number.MAX_SAFE_INTEGER
}
