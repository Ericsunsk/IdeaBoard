import { computed, onUnmounted, ref } from 'vue'
import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { useAdminCollectionQuery } from '../../shared/composables/useAdminCollectionQuery'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/toast'
import {
  createAdminGatewayResource,
  type AdminGatewayCatalog,
} from '../infrastructure/adminGatewayResource'
import type { AdminModelItem } from '../domain/catalogData'
import { matchesModelKeyword, MODEL_HOSTING_OPTIONS } from '../domain/catalogData'

// Query key 归 workspace 所有（Page 不得自建 key）：定价 adapter 同 key 共享目录缓存。
export function getAdminModelCatalogQueryKey() {
  return ['admin', 'models', 'catalog'] as const
}

// 模型目录页的用例编排：collection query、筛选、refresh 富化、CRUD、复制。
// Page 只做布局与绑定。resource 可注入，测试传 fake 即脱离 MSW 整页。
export function useAdminModelCatalogWorkspace(
  resource: AdminGatewayCatalog = createAdminGatewayResource().catalog,
) {
  const adminConfirm = useAdminConfirm()
  const toast = useAdminToast()
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

  function invalidateCatalog() {
    return queryClient.invalidateQueries({ queryKey })
  }

  const saveCatalogMutation = useMutation({
    mutationFn: (items: AdminModelItem[]) => resource.save(items),
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
          await resource.save(currentList)
        }
      } catch {
        // 探测富化是 best-effort：网络异常不阻塞刷新，列表仍按服务端当前数据展示。
      }

      await invalidateCatalog()
      toast.success('模型库已更新至最新')
    } catch (err) {
      toast.error(formatAdminDisplayError(err, '同步模型库失败'))
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
        toast.info('所选模型已全部存在于模型库中')
        addDialogVisible.value = false
        return
      }
      await saveCatalogMutation.mutateAsync([...toAdd, ...allModels.value])
      addDialogVisible.value = false
      toast.success(`已成功导入 ${toAdd.length} 个模型到模型库`)
    } catch (err) {
      toast.error(formatAdminDisplayError(err, '添加模型失败'))
    }
  }

  function handleEditModel(model: AdminModelItem) {
    editingModel.value = model
    editDialogVisible.value = true
  }

  async function handleDeleteModel(model: AdminModelItem) {
    const confirmed = await adminConfirm.confirm({
      title: '删除模型',
      message: `确定要从模型库中移除模型「${model.name} (${model.id})」吗？引用该模型的功能定价映射将一并移除，需重新指定模型。`,
      confirmLabel: '确认删除',
      intent: 'danger',
    })
    if (!confirmed) return
    const nextList = allModels.value.filter((m) => m.id !== model.id)
    try {
      await saveCatalogMutation.mutateAsync(nextList)
      toast.success(`已删除模型「${model.name}」`)
    } catch (err) {
      toast.error(formatAdminDisplayError(err, '删除模型失败'))
    }
  }

  async function handleSaveModel(item: AdminModelItem) {
    try {
      let nextList: AdminModelItem[]
      if (editingModel.value) {
        nextList = allModels.value.map((m) => (m.id === item.id ? item : m))
      } else {
        if (allModels.value.some((m) => m.id === item.id)) {
          toast.error(`模型标识 ${item.id} 已存在，不能重复添加`)
          return
        }
        nextList = [item, ...allModels.value]
      }
      await saveCatalogMutation.mutateAsync(nextList)
      editDialogVisible.value = false
      toast.success(editingModel.value ? '模型修改已保存' : '已成功新增模型')
      editingModel.value = null
    } catch (err) {
      toast.error(formatAdminDisplayError(err, '保存模型失败'))
    }
  }

  function copyModelId(id: string) {
    if (copyTimer) clearTimeout(copyTimer)
    copyTimer = null
    if (!navigator?.clipboard?.writeText) {
      toast.error('当前环境不支持剪贴板复制')
      return
    }
    void navigator.clipboard.writeText(id).then(
      () => {
        copiedModelId.value = id
        copyTimer = setTimeout(() => {
          if (copiedModelId.value === id) {
            copiedModelId.value = null
          }
        }, 2000)
      },
      () => {
        toast.error('复制失败，请手动复制模型标识')
      },
    )
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
