import { featureOfSku } from './skuFeature'

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

// 有映射面的 namespace：建表的子集，映射可以先行（如 ai-search 暂无独立表）。
const MAPPABLE_NAMESPACES: readonly string[] = ['ai-image', 'ai-search']

function pricingNamespaceOfSku(sku: string): string {
  const cut = sku.indexOf(':')
  return cut > 0 ? sku.slice(0, cut) : ''
}

export function pricingTableKeyOfSku(sku: string): string {
  const namespace = pricingNamespaceOfSku(sku)
  const table = PRICING_NAMESPACE_TABLES.find((candidate) => candidate.namespace === namespace)
  return table ? table.namespace : PRICING_OTHER_TABLE.namespace
}

// 映射特性：namespace 唯一从 SKU 派生（单源），调用方不得再传 namespace
//（曾出现 sku/namespace 不一致拼出垃圾 key）。ai-image 走后缀约定，
// ai-search 取冒号后缀；未知返回 null，调用方不渲染映射行。
// 根治说明：后端 vision 存储已隔离（ai_gateway_vision_model vs
// ai_gateway_model_<slug>），workflow 侧以 ReservedVisionSlug 永久预留
// "vision"；前端再预留 ai-image:vision 返回 null，即使目录误配该 SKU 也不建
// 映射行，三层互锁使 ai-image:vision / ai-search:vision 覆盖不可能发生。
// findPricingMappingCollisions 保留作通用新增同名兜底（见 Page loadCatalog）。
export function pricingMappingFeature(sku: string): string | null {
  const namespace = pricingNamespaceOfSku(sku)
  if (!namespace) return null
  if (!MAPPABLE_NAMESPACES.includes(namespace)) return null
  if (namespace === 'ai-image') {
    const feature = featureOfSku(sku)
    // "vision" 预留给 ai-search（对齐后端 workflow.ReservedVisionSlug）。
    if (feature === 'vision') return null
    return feature
  }
  const suffix = sku.slice(namespace.length + 1)
  return suffix || null
}

// 裸 key 碰撞检测（纯函数）：同一 bare feature 被多 namespace 认领即碰撞，
// 调用方保存 A 会覆盖 B 的显示。当前目录无碰撞，仅防新增同名 feature。
export function findPricingMappingCollisions(skus: readonly string[]): Array<{
  feature: string
  namespaces: string[]
  skus: string[]
}> {
  const byFeature = new Map<string, { namespaces: Set<string>; skus: string[] }>()
  for (const sku of skus) {
    const feature = pricingMappingFeature(sku)
    if (!feature) continue
    const namespace = pricingNamespaceOfSku(sku)
    let entry = byFeature.get(feature)
    if (!entry) {
      entry = { namespaces: new Set<string>(), skus: [] }
      byFeature.set(feature, entry)
    }
    entry.namespaces.add(namespace)
    entry.skus.push(sku)
  }
  return [...byFeature.entries()]
    .filter(([, entry]) => entry.namespaces.size > 1)
    .map(([feature, entry]) => ({
      feature,
      namespaces: [...entry.namespaces].sort(),
      skus: entry.skus,
    }))
}

// 排序键：namespace 按表序，未知沉底。
export function pricingNamespaceRank(sku: string): number {
  const namespace = pricingNamespaceOfSku(sku)
  const index = PRICING_NAMESPACE_TABLES.findIndex((table) => table.namespace === namespace)
  return index >= 0 ? index : Number.MAX_SAFE_INTEGER
}
