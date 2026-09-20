import { computed, reactive, ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { createAdminGatewayResource } from '../../models/infrastructure/adminGatewayResource'
import { getAdminModelCatalogQueryKey } from '../../models/application/useAdminModelCatalogWorkspace'
import type { AdminModelItem } from '../../models/domain/catalogData'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/toast'
import { pricingMappingFeature } from './groups'
import type { AdminBillableItemCard } from './billableCatalogTypes'

// 定价页模型映射域唯一出口：目录（与模型页同 queryKey 共享缓存）、
// 映射读写、行状态派生。Page 只剩价格逻辑，Dialog 只收 props。
export function usePricingModelAdapter() {
  const toast = useAdminToast()
  const gateway = createAdminGatewayResource()
  const modelByFeature = reactive<Record<string, string>>({})
  const modelPending = reactive<Record<string, boolean>>({})
  const modelErrors = reactive<Record<string, string>>({})

  // 不自动拉取：由 loadModels 显式触发，写入共享 key 供模型页复用。
  const catalogQuery = useQuery({
    queryKey: getAdminModelCatalogQueryKey(),
    queryFn: () => gateway.catalog.list(),
    enabled: false,
    retry: false,
  })
  const availableModels = computed<AdminModelItem[]>(() => catalogQuery.data.value ?? [])
  const modelsLoading = ref(false)

  async function loadModels() {
    modelsLoading.value = true
    // refetch 失败时 TanStack Query 默认保留旧 data：按是否有缓存区分文案，
    // 避免“暂不可用/暂未配置”与实际展示 stale 数据不符。
    const hadCatalogCache = (catalogQuery.data.value?.length ?? 0) > 0
    const hadMappingCache = Object.keys(modelByFeature).length > 0
    try {
      const result = await catalogQuery.refetch()
      // retry:false + 默认 throwOnError:false 时 refetch 不抛错，需显式检查。
      if (result.isError) throw result.error
    } catch (error) {
      if (hadCatalogCache) {
        toast.warning(formatAdminDisplayError(error, '加载模型目录失败，已展示缓存数据'))
      } else {
        // 模型目录失败时下拉为空：显式提示，与“无模型”空态区分。
        toast.error(formatAdminDisplayError(error, '加载模型目录失败，模型选择暂不可用'))
      }
    }
    // Model mappings ride along but never gate pricing: a mapping fetch
    // failure keeps stale rows (with warning) instead of failing the page.
    try {
      const mappings = await gateway.mappings.list()
      // 成功时全量替换：避免已删除映射的旧值残留；失败时保留旧值。
      for (const key of Object.keys(modelByFeature)) delete modelByFeature[key]
      for (const mapping of mappings) modelByFeature[mapping.feature] = mapping.model
    } catch (error) {
      if (hadMappingCache) {
        toast.warning(formatAdminDisplayError(error, '加载模型映射失败，已展示缓存数据'))
      } else {
        toast.error(formatAdminDisplayError(error, '加载模型映射失败，模型行暂未配置'))
      }
    } finally {
      modelsLoading.value = false
    }
  }

  async function persistModelMapping(feature: string, model: string) {
    if (!feature || modelPending[feature]) return
    modelPending[feature] = true
    modelErrors[feature] = ''
    try {
      await gateway.mappings.save(feature, model)
      modelByFeature[feature] = model
      toast.success('模型映射已保存，价格不受影响')
    } catch (error) {
      modelErrors[feature] = formatAdminDisplayError(error, '保存模型映射失败，请重试')
    } finally {
      modelPending[feature] = false
    }
  }

  // 映射特性按 namespace 解析（ai-image 看后缀约定，ai-search 看冒号后缀，
  // 未知返回 null）：调用方无映射行，避免多卡共享 '' 键交叉污染。
  function saveModelForSku(sku: string, model: string) {
    const feature = pricingMappingFeature(sku)
    if (!feature) return
    void persistModelMapping(feature, model)
  }

  function saveModelForItem(item: AdminBillableItemCard, model: string) {
    saveModelForSku(item.sku, model)
  }

  function modelValueFor(sku: string): string | undefined {
    const feature = pricingMappingFeature(sku)
    return feature ? modelByFeature[feature] : undefined
  }

  function modelPendingFor(sku: string): boolean | undefined {
    const feature = pricingMappingFeature(sku)
    return feature ? modelPending[feature] : undefined
  }

  function modelErrorFor(sku: string): string | undefined {
    const feature = pricingMappingFeature(sku)
    return feature ? modelErrors[feature] : undefined
  }

  return {
    availableModels,
    modelsLoading,
    loadModels,
    saveModelForSku,
    saveModelForItem,
    modelValueFor,
    modelPendingFor,
    modelErrorFor,
  }
}
