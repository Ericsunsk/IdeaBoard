// SKU 到 workflow feature 的唯一映射点：非 ai-image 前缀返回 null，
// 调用方不得再用 '' 做共享键（多卡交叉污染）。
const IMAGE_SKU_PREFIX = 'ai-image:'

export function featureOfSku(sku: string): string | null {
  if (!sku.startsWith(IMAGE_SKU_PREFIX)) return null
  const feature = sku.slice(IMAGE_SKU_PREFIX.length)
  return feature || null
}
