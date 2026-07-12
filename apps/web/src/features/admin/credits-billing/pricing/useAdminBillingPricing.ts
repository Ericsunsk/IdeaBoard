import { computed, ref, shallowRef } from 'vue'
import {
  filterBillingCatalog,
  isCatalogItemBillable,
  isCatalogItemUnpriced,
} from './billingPricingModel'
import {
  createAdminBillingPricingResource,
  type AdminBillingPricingResource,
} from './billingPricingResource'
import type {
  AdminBillingCatalogItem,
  AdminBillingCoverageItem,
  AdminBillingPrice,
  BillingCatalogFilter,
  CreateAdminBillingPriceInput,
  CreateAdminBillingSKUInput,
  UpdateAdminBillingSKUInput,
} from './billingPricingTypes'

const catalogPageSize = 24
const pricePageSize = 20

export interface AdminBillingPricingFeedback {
  success: (message: string) => void
  error: (error: unknown, fallback: string) => void
}

export interface UseAdminBillingPricingOptions {
  resource?: AdminBillingPricingResource
  feedback: AdminBillingPricingFeedback
}

export function useAdminBillingPricing(options: UseAdminBillingPricingOptions) {
  const resource = options.resource ?? createAdminBillingPricingResource()
  const coverageItems = ref<AdminBillingCoverageItem[]>([])
  const catalogItems = ref<AdminBillingCatalogItem[]>([])
  const selectedSKU = ref<AdminBillingCatalogItem>()
  const prices = ref<AdminBillingPrice[]>([])
  const searchKeyword = ref('')
  const catalogFilter = ref<BillingCatalogFilter>('all')
  const catalogLoading = ref(false)
  const coverageLoading = ref(false)
  const pricesLoading = ref(false)
  const catalogError = ref('')
  const coverageError = ref('')
  const pricesError = ref('')
  const mutationPending = ref(false)
  const catalogCursors = ref<(string | undefined)[]>([undefined])
  const catalogPageIndex = ref(0)
  const nextCatalogCursor = ref<string>()
  const priceCursors = ref<(number | undefined)[]>([undefined])
  const pricePageIndex = ref(0)
  const nextPriceCursor = ref<number>()
  const priceLoadSequence = shallowRef(0)

  const visibleCatalogItems = computed(() =>
    filterBillingCatalog(catalogItems.value, searchKeyword.value, catalogFilter.value),
  )
  const currentCatalogPage = computed(() => catalogPageIndex.value + 1)
  const catalogTotalPages = computed(
    () => currentCatalogPage.value + (nextCatalogCursor.value ? 1 : 0),
  )
  const currentPricePage = computed(() => pricePageIndex.value + 1)
  const priceTotalPages = computed(() => currentPricePage.value + (nextPriceCursor.value ? 1 : 0))
  const unpricedCount = computed(() => catalogItems.value.filter(isCatalogItemUnpriced).length)
  const billableCount = computed(() => catalogItems.value.filter(isCatalogItemBillable).length)

  async function loadCoverage() {
    coverageLoading.value = true
    coverageError.value = ''
    try {
      coverageItems.value = await resource.listCoverage()
    } catch (error) {
      coverageError.value = 'AI Workflow 计费覆盖加载失败'
      options.feedback.error(error, coverageError.value)
    } finally {
      coverageLoading.value = false
    }
  }

  async function loadCatalog(
    preferredSKU = selectedSKU.value?.sku,
    fallbackSelection?: AdminBillingCatalogItem,
  ) {
    catalogLoading.value = true
    catalogError.value = ''
    try {
      const page = await resource.listSKUs({
        afterSku: catalogCursors.value[catalogPageIndex.value],
        limit: catalogPageSize,
      })
      catalogItems.value = page.items
      nextCatalogCursor.value = page.nextAfterSku

      const nextSelection =
        page.items.find((item) => item.sku === preferredSKU) ??
        (fallbackSelection && fallbackSelection.sku === preferredSKU
          ? fallbackSelection
          : page.items.at(0))
      selectedSKU.value = nextSelection
      resetPricePagination()
      if (nextSelection) {
        await loadPrices(nextSelection.sku)
      } else {
        prices.value = []
      }
    } catch (error) {
      catalogError.value = 'SKU Catalog 加载失败'
      options.feedback.error(error, catalogError.value)
    } finally {
      catalogLoading.value = false
    }
  }

  async function loadPrices(sku = selectedSKU.value?.sku) {
    if (!sku) {
      prices.value = []
      return
    }

    const sequence = priceLoadSequence.value + 1
    priceLoadSequence.value = sequence
    pricesLoading.value = true
    pricesError.value = ''
    try {
      const page = await resource.listPrices(sku, {
        beforeVersion: priceCursors.value[pricePageIndex.value],
        limit: pricePageSize,
      })
      if (sequence !== priceLoadSequence.value || selectedSKU.value?.sku !== sku) return
      prices.value = page.prices
      nextPriceCursor.value = page.nextBeforeVersion
    } catch (error) {
      if (sequence !== priceLoadSequence.value) return
      pricesError.value = '价格版本加载失败'
      options.feedback.error(error, pricesError.value)
    } finally {
      if (sequence === priceLoadSequence.value) pricesLoading.value = false
    }
  }

  async function selectSKU(item: AdminBillingCatalogItem) {
    if (selectedSKU.value?.sku === item.sku) return
    selectedSKU.value = item
    resetPricePagination()
    await loadPrices(item.sku)
  }

  async function nextCatalogPage() {
    if (!nextCatalogCursor.value || catalogLoading.value) return
    catalogCursors.value = catalogCursors.value.slice(0, catalogPageIndex.value + 1)
    catalogCursors.value.push(nextCatalogCursor.value)
    catalogPageIndex.value += 1
    await loadCatalog()
  }

  async function previousCatalogPage() {
    if (catalogPageIndex.value === 0 || catalogLoading.value) return
    catalogPageIndex.value -= 1
    await loadCatalog()
  }

  async function nextPricePage() {
    if (!nextPriceCursor.value || pricesLoading.value) return
    priceCursors.value = priceCursors.value.slice(0, pricePageIndex.value + 1)
    priceCursors.value.push(nextPriceCursor.value)
    pricePageIndex.value += 1
    await loadPrices()
  }

  async function previousPricePage() {
    if (pricePageIndex.value === 0 || pricesLoading.value) return
    pricePageIndex.value -= 1
    await loadPrices()
  }

  async function createSKU(input: CreateAdminBillingSKUInput): Promise<boolean> {
    let created: AdminBillingCatalogItem | undefined
    const succeeded = await runMutation(
      'SKU 已创建；该标识将永久占用且不可复用',
      '创建 SKU 失败',
      async () => {
        created = await resource.createSKU(input)
        selectedSKU.value = created
        prices.value = []
        resetCatalogPagination()
        resetPricePagination()
      },
    )
    if (succeeded && created) {
      await Promise.all([loadCatalog(created.sku, created), loadCoverage()])
    }
    return succeeded
  }

  async function updateSKU(input: UpdateAdminBillingSKUInput): Promise<boolean> {
    const selected = selectedSKU.value
    if (!selected) return false

    const succeeded = await runMutation('SKU 信息已更新', '更新 SKU 失败', async () => {
      const updated = await resource.updateSKU(selected.sku, input)
      replaceCatalogItem(updated)
      selectedSKU.value = updated
    })
    if (succeeded) await loadCoverage()
    return succeeded
  }

  async function createPrice(input: CreateAdminBillingPriceInput): Promise<boolean> {
    const selected = selectedSKU.value
    if (!selected) return false

    const succeeded = await runMutation('新价格版本已发布', '发布价格版本失败', async () => {
      const created = await resource.createPrice(selected.sku, input)
      const updatedItem = {
        ...selected,
        currentPriceVersionId: created.priceVersionId,
        currentPrice: created,
      }
      replaceCatalogItem(updatedItem)
      selectedSKU.value = updatedItem
      resetPricePagination()
    })
    if (succeeded) await Promise.all([loadPrices(selected.sku), loadCoverage()])
    return succeeded
  }

  async function setPriceEnabled(
    price: AdminBillingPrice,
    enabled: boolean,
    reason: string,
  ): Promise<boolean> {
    const selected = selectedSKU.value
    if (!selected) return false

    const succeeded = await runMutation(
      enabled ? '价格版本已启用' : '价格版本已停用',
      '更新价格状态失败',
      async () => {
        const updated = await resource.setPriceEnabled(selected.sku, price.version, {
          enabled,
          reason,
        })
        prices.value = prices.value.map((item) =>
          item.priceVersionId === updated.priceVersionId ? updated : item,
        )
        if (selected.currentPriceVersionId === updated.priceVersionId) {
          const updatedItem = { ...selected, currentPrice: updated }
          replaceCatalogItem(updatedItem)
          selectedSKU.value = updatedItem
        }
      },
    )
    if (succeeded) await loadCoverage()
    return succeeded
  }

  async function runMutation(
    successMessage: string,
    fallback: string,
    operation: () => Promise<void>,
  ): Promise<boolean> {
    if (mutationPending.value) return false
    mutationPending.value = true
    try {
      await operation()
      options.feedback.success(successMessage)
      return true
    } catch (error) {
      options.feedback.error(error, fallback)
      return false
    } finally {
      mutationPending.value = false
    }
  }

  function replaceCatalogItem(updated: AdminBillingCatalogItem) {
    catalogItems.value = catalogItems.value.map((item) =>
      item.sku === updated.sku ? updated : item,
    )
  }

  function resetCatalogPagination() {
    catalogCursors.value = [undefined]
    catalogPageIndex.value = 0
    nextCatalogCursor.value = undefined
  }

  function resetPricePagination() {
    priceCursors.value = [undefined]
    pricePageIndex.value = 0
    nextPriceCursor.value = undefined
  }

  return {
    coverageItems,
    catalogItems,
    visibleCatalogItems,
    selectedSKU,
    prices,
    searchKeyword,
    catalogFilter,
    coverageLoading,
    catalogLoading,
    pricesLoading,
    coverageError,
    catalogError,
    pricesError,
    mutationPending,
    currentCatalogPage,
    catalogTotalPages,
    currentPricePage,
    priceTotalPages,
    nextCatalogCursor,
    nextPriceCursor,
    unpricedCount,
    billableCount,
    loadCoverage,
    loadCatalog,
    loadPrices,
    selectSKU,
    nextCatalogPage,
    previousCatalogPage,
    nextPricePage,
    previousPricePage,
    createSKU,
    updateSKU,
    createPrice,
    setPriceEnabled,
  }
}
