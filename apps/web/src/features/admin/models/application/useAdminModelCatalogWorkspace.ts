import { computed, onUnmounted, ref } from 'vue'
import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { useAdminCollectionQuery } from '../../shared/composables/useAdminCollectionQuery'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { confirmSettingsSave, requireDestructiveReason } from '../../shared/lib/confirmSettingsSave'
import { useAdminModelCatalogFeedback } from './useAdminModelCatalogFeedback'
import { useAppClipboard } from '../../../../shared/composables/useAppClipboard'
import {
  createAdminGatewayResource,
  type AdminGatewayCatalog,
} from '../infrastructure/adminGatewayResource'
import type { AdminModelItem, ModelObservedSnapshot } from '../domain/catalogData'
import { matchesModelKeyword, MODEL_HOSTING_OPTIONS } from '../domain/catalogData'

// Query key 归 workspace 所有（Page 不得自建 key）：定价 adapter 同 key 共享目录缓存。
export function getAdminModelCatalogQueryKey() {
  return ['admin', 'models', 'catalog'] as const
}

// 观测 query key 同样归 workspace 所有：卡片观测区与目录列表共享同一失效节奏。
function getAdminModelObservedQueryKey() {
  return ['admin', 'models', 'observed'] as const
}

// 模型目录页的用例编排：collection query、筛选、refresh 富化、CRUD、复制。
// Page 只做布局与绑定。resource 可注入，测试传 fake 即脱离 MSW 整页。
export function useAdminModelCatalogWorkspace(
  resource: AdminGatewayCatalog = createAdminGatewayResource().catalog,
) {
  const adminConfirm = useAdminConfirm()
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
  } = useAdminCollectionQuery({
    queryKey,
    queryFn: () => resource.list(),
  })
  const allModels = computed<AdminModelItem[]>(() => catalogData.value ?? [])

  // 卡片观测区：7 天聚合只读透传，失败即空（卡片按暂无数据缺省，不抛错误横幅）。
  const { data: observedData, loading: isObservedLoading } = useAdminCollectionQuery({
    queryKey: getAdminModelObservedQueryKey(),
    queryFn: () => resource.listObserved(),
  })
  const observedByModel = computed(() => {
    const map = new Map<string, ModelObservedSnapshot>()
    for (const item of observedData.value ?? []) map.set(item.model, item)
    return map
  })

  function invalidateCatalog() {
    return queryClient.invalidateQueries({ queryKey })
  }

  const saveCatalogMutation = useMutation({
    mutationFn: (input: { items: AdminModelItem[]; operatorReason: string }) =>
      resource.save(input.items, { operatorReason: input.operatorReason }),
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
      // 刷新 = 读当前库 + 探测富化缺失简介/上下文 + 按需保存 + 失效重取。
      // 无种子合并：模型库唯一事实源是服务端 catalog，不做本地 seed 回写。
      let currentList = await resource.list()

      // 智能增量检测：自动补全已有模型中缺失的官方简介和上下文长度
      try {
        const detection = await resource.detect()
        const detectedMap = new Map(detection.models.map((d) => [d.id, d]))
        let enrichedCount = 0
        currentList = currentList.map((model) => {
          const d = detectedMap.get(model.id)
          if (!d) return model
          let changed = false
          let nextDesc = model.description
          let nextContext = model.contextOrResolution
          // 富化位为准：导入文本（含占位）可覆盖，运营手写永不触碰。
          // 空简介一律可补（手工留空也视为缺失）。
          if ((model.descriptionEnriched || !nextDesc) && d.description) {
            nextDesc = d.description
            changed = true
          }
          if (!nextContext && d.context_or_resolution) {
            nextContext = d.context_or_resolution
            changed = true
          }
          if (changed) {
            enrichedCount++
            return {
              ...model,
              description: nextDesc,
              contextOrResolution: nextContext,
            }
          }
          return model
        })
        if (enrichedCount > 0) {
          // Best-effort enrichment still writes through control, so it
          // needs an operator reason like any other catalog save.
          const operatorReason = await confirmSettingsSave(adminConfirm, {
            title: '保存模型富化结果',
            message: `探测补全了 ${enrichedCount} 个模型的官方简介或上下文信息，确定保存吗？该操作将记录操作人与原因。`,
            confirmLabel: '保存',
          })
          if (operatorReason) {
            await resource.save(currentList, { operatorReason })
          }
        }
      } catch {
        // 探测富化是 best-effort：网络异常不阻塞刷新，列表仍按服务端当前数据展示。
      }

      await invalidateCatalog()
      feedback.catalogSynced()
    } catch (err) {
      feedback.syncFailed(err)
    } finally {
      isRefreshing.value = false
    }
  }

  function handleCreateModel() {
    addDialogVisible.value = true
  }

  // 手动新增：探测不到的 Provider（Vertex AI 等）走 EditDialog 空表单入库。
  // 激活 EditDialog 的新增分支与 handleSaveModel 的 prepend 分支（此前仅组件单测覆盖）。
  function handleCreateManualModel() {
    addDialogVisible.value = false
    editingModel.value = null
    editDialogVisible.value = true
  }

  async function handleImportModels(items: AdminModelItem[]) {
    try {
      const existingIds = new Set(allModels.value.map((m) => m.id))
      const toAdd = items.filter((item) => !existingIds.has(item.id))
      if (toAdd.length === 0) {
        feedback.importSkippedAsExisting()
        addDialogVisible.value = false
        return
      }
      const operatorReason = await confirmSettingsSave(adminConfirm, {
        title: '导入模型',
        message: `确定导入 ${toAdd.length} 个模型吗？该操作将记录操作人与原因。`,
        confirmLabel: '导入',
      })
      if (!operatorReason) return
      await saveCatalogMutation.mutateAsync({ items: [...toAdd, ...allModels.value], operatorReason })
      addDialogVisible.value = false
      feedback.imported(toAdd.length)
    } catch (err) {
      feedback.addFailed(err)
    }
  }

  function handleEditModel(model: AdminModelItem) {
    editingModel.value = model
    editDialogVisible.value = true
  }

  async function handleDeleteModel(model: AdminModelItem) {
    const confirmed = await adminConfirm.confirm({
      title: '删除模型',
      message: `确定要从模型库中移除模型「${model.name} (${model.id})」吗？引用该模型的功能定价映射将一并移除，需重新指定模型。该操作将记录操作人与原因。`,
      confirmLabel: '确认删除',
      intent: 'danger',
      reason: { required: true },
    })
    if (!confirmed) return
    const operatorReason = requireDestructiveReason(adminConfirm)
    if (!operatorReason) return
    const nextList = allModels.value.filter((m) => m.id !== model.id)
    try {
      await saveCatalogMutation.mutateAsync({ items: nextList, operatorReason })
      feedback.deleted(model.name)
    } catch (err) {
      feedback.deleteFailed(err)
    }
  }

  async function handleSaveModel(item: AdminModelItem) {
    try {
      let nextList: AdminModelItem[]
      if (editingModel.value) {
        nextList = allModels.value.map((m) => (m.id === item.id ? item : m))
      } else {
        if (allModels.value.some((m) => m.id === item.id)) {
          feedback.duplicateId(item.id)
          return
        }
        nextList = [item, ...allModels.value]
      }
      const operatorReason = await confirmSettingsSave(adminConfirm, {
        title: editingModel.value ? '保存模型' : '新增模型',
        message: `确定保存模型「${item.name} (${item.id})」吗？该操作将记录操作人与原因。`,
        confirmLabel: '保存',
      })
      if (!operatorReason) return
      await saveCatalogMutation.mutateAsync({ items: nextList, operatorReason })
      editDialogVisible.value = false
      feedback.saved(editingModel.value != null)
      editingModel.value = null
    } catch (err) {
      feedback.saveFailed(err)
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
