import { computed, onUnmounted, ref, type ComputedRef } from 'vue'
import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { useAdminCollectionQuery } from '../../shared'
import { useConfirm } from '@/core/composables/useConfirmDialog'
import {
  useAdminModelCatalogFeedback,
  type AdminModelCatalogFeedback,
} from './useAdminModelCatalogFeedback'
import { useAppClipboard } from '@/core/composables/useAppClipboard'
import { useFailureToast } from '@/core/composables/useFailureToast'
import { createAdminGatewayResource, type AdminGatewayCatalog } from '../resource'
import type { AdminModelItem, ModelObservedSnapshot } from '../core/catalogData'
import { modelIdentityKey, matchesModelKeyword, MODEL_HOSTING_OPTIONS } from '../core/catalogData'

// Query key is owned by this workspace.
function getAdminModelCatalogQueryKey() {
  return ['admin', 'models', 'catalog'] as const
}

// 观测 query key 同样归 workspace 所有：卡片观测区与目录列表共享同一失效节奏。
function getAdminModelObservedQueryKey() {
  return ['admin', 'models', 'observed'] as const
}

function mirrorCatalogLoadFailure(
  isListError: ComputedRef<boolean>,
  getError: () => unknown,
  feedback: AdminModelCatalogFeedback,
) {
  useFailureToast(isListError, feedback.listFailed, {
    shouldFireOnMount: false,
    resolveError: getError,
  })
}

// 模型目录页的用例编排：collection query、筛选、刷新、CRUD、复制。
// Page 只做布局与绑定。resource 可注入，测试传 fake 即脱离 MSW 整页。
export function useAdminModelCatalogWorkspace(
  resource: AdminGatewayCatalog = createAdminGatewayResource().catalog,
) {
  const adminConfirm = useConfirm()
  const feedback = useAdminModelCatalogFeedback()
  const { copyText } = useAppClipboard()
  const queryClient = useQueryClient()
  const queryKey = getAdminModelCatalogQueryKey()

  const {
    data: catalogData,
    loading: isLoading,
    initialError: isListError,
    backgroundError: isListStale,
    refresh: refetchCatalog,
    query: catalogQuery,
  } = useAdminCollectionQuery({
    queryKey,
    queryFn: () => resource.list(),
  })
  const allModels = computed<AdminModelItem[]>(() => catalogData.value ?? [])

  mirrorCatalogLoadFailure(isListError, () => catalogQuery.error.value, feedback)

  // 卡片观测区：7 天聚合只读透传，失败即空（卡片按暂无数据缺省，不抛错误横幅）。
  const { data: observedData, loading: isObservedLoading } = useAdminCollectionQuery({
    queryKey: getAdminModelObservedQueryKey(),
    queryFn: () => resource.listObserved(),
  })
  const observedByModel = computed(() => {
    const map = new Map<string, ModelObservedSnapshot>()
    for (const item of observedData.value ?? []) map.set(item.model.trim().toLowerCase(), item)
    return map
  })

  function invalidateCatalog() {
    return queryClient.invalidateQueries({ queryKey })
  }

  const saveCatalogMutation = useMutation({
    mutationFn: (input: { items: AdminModelItem[] }) => resource.save(input.items),
    onSuccess: () => {
      void invalidateCatalog()
    },
  })
  const isSaving = computed(() => saveCatalogMutation.isPending.value)
  const isRefreshing = ref(false)

  // 添加模型与编辑模型弹窗状态
  const addDialogVisible = ref(false)
  const editDialogVisible = ref(false)
  const editingModel = ref<AdminModelItem | null>(null)

  // 新增网关弹窗状态
  const gatewayDialogVisible = ref(false)

  // 过滤状态
  const searchKeyword = ref('')
  const selectedMode = ref('')
  const selectedProvider = ref('')
  const selectedHosting = ref('')

  // 复制交互状态
  const copiedModelId = ref<string | null>(null)
  let copyTimer: ReturnType<typeof setTimeout> | null = null

  // 筛选选项计算（mode 取数据去重，不定义分类体系）
  const modeOptions = computed(() => {
    const set = new Set(
      allModels.value.map((m) => m.mode?.trim()).filter((v): v is string => Boolean(v)),
    )
    return Array.from(set)
  })

  const providerOptions = computed(() => {
    const set = new Set(allModels.value.map((m) => m.provider))
    return Array.from(set)
  })

  const hostingOptions = [...MODEL_HOSTING_OPTIONS]

  const hasActiveFilter = computed(() => {
    return Boolean(
      searchKeyword.value.trim() ||
      selectedMode.value ||
      selectedProvider.value ||
      selectedHosting.value,
    )
  })

  const filteredModels = computed(() => {
    const keyword = searchKeyword.value
    return allModels.value.filter((model) => {
      if (!matchesModelKeyword(model, keyword)) return false
      if (selectedMode.value && model.mode !== selectedMode.value) {
        return false
      }
      if (selectedProvider.value && model.provider !== selectedProvider.value) {
        return false
      }
      if (selectedHosting.value && model.hosting !== selectedHosting.value) {
        return false
      }
      return true
    })
  })

  function clearFilters() {
    searchKeyword.value = ''
    selectedMode.value = ''
    selectedProvider.value = ''
    selectedHosting.value = ''
  }

  async function handleRefresh() {
    if (isRefreshing.value) return
    isRefreshing.value = true
    try {
      const result = await refetchCatalog()
      if (result.isError) throw result.error
      feedback.catalogSynced()
    } catch (caughtError) {
      feedback.syncFailed(caughtError)
    } finally {
      isRefreshing.value = false
    }
  }

  function handleCreateModel() {
    addDialogVisible.value = true
  }

  // 手动新增：走 EditDialog 空表单入库。
  function handleCreateManualModel() {
    addDialogVisible.value = false
    editingModel.value = null
    editDialogVisible.value = true
  }

  async function handleImportModels(items: AdminModelItem[]) {
    try {
      const existingKeys = new Set(allModels.value.map(modelIdentityKey))
      const toAdd = items.filter((item) => {
        const key = modelIdentityKey(item)
        if (existingKeys.has(key)) return false
        existingKeys.add(key)
        return true
      })
      if (toAdd.length === 0) {
        feedback.importSkippedAsExisting()
        addDialogVisible.value = false
        return
      }
      await saveCatalogMutation.mutateAsync({
        items: [...toAdd, ...allModels.value],
      })
      addDialogVisible.value = false
      feedback.imported(toAdd.length)
    } catch (caughtError) {
      feedback.addFailed(caughtError)
    }
  }

  function handleEditModel(model: AdminModelItem) {
    editingModel.value = model
    editDialogVisible.value = true
  }

  async function handleDeleteModel(model: AdminModelItem) {
    const confirmed = await adminConfirm.confirm({
      title: '删除模型',
      message: `确定要从模型库中移除模型「${model.name} (${model.id})」吗？引用该模型的功能定价映射将一并移除，需重新指定模型。该操作将记录操作人。`,
      confirmLabel: '确认删除',
      intent: 'danger',
    })
    if (!confirmed) return

    const nextList = allModels.value.filter((m) => modelIdentityKey(m) !== modelIdentityKey(model))
    try {
      await saveCatalogMutation.mutateAsync({ items: nextList })
      feedback.deleted(model.name)
    } catch (caughtError) {
      feedback.deleteFailed(caughtError)
    }
  }

  async function handleSaveModel(item: AdminModelItem) {
    try {
      const originalKey = editingModel.value ? modelIdentityKey(editingModel.value) : null
      const nextKey = modelIdentityKey(item)
      if (
        allModels.value.some(
          (m) => modelIdentityKey(m) === nextKey && modelIdentityKey(m) !== originalKey,
        )
      ) {
        feedback.duplicateId(`${item.provider}/${item.id}`)
        return
      }
      const nextList = originalKey
        ? allModels.value.map((m) => (modelIdentityKey(m) === originalKey ? item : m))
        : [item, ...allModels.value]
      await saveCatalogMutation.mutateAsync({ items: nextList })
      editDialogVisible.value = false
      feedback.saved(editingModel.value != null)
      editingModel.value = null
    } catch (caughtError) {
      feedback.saveFailed(caughtError)
    }
  }

  function copyModelId(id: string) {
    if (copyTimer) clearTimeout(copyTimer)
    copyTimer = null
    void copyText(id).then((ok) => {
      if (ok) {
        copiedModelId.value = id
        copyTimer = setTimeout(() => {
          if (copiedModelId.value === id) {
            copiedModelId.value = null
          }
        }, 2000)
      } else {
        feedback.copyFailed()
      }
    })
  }

  onUnmounted(() => {
    if (copyTimer) clearTimeout(copyTimer)
    copyTimer = null
  })

  return {
    // collection
    allModels,
    isLoading,
    isListError,
    isListStale,
    refetchCatalog,
    // observed (card stats, graceful empty on failure)
    observedByModel,
    isObservedLoading,
    isSaving,
    isRefreshing,
    // dialogs
    addDialogVisible,
    editDialogVisible,
    editingModel,
    gatewayDialogVisible,
    // filters
    searchKeyword,
    selectedMode,
    selectedProvider,
    selectedHosting,
    modeOptions,
    providerOptions,
    hostingOptions,
    hasActiveFilter,
    filteredModels,
    clearFilters,
    // actions
    handleRefresh,
    handleCreateModel,
    handleCreateManualModel,
    handleImportModels,
    handleEditModel,
    handleDeleteModel,
    handleSaveModel,
    copyModelId,
    copiedModelId,
  }
}
