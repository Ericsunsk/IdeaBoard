import { computed, ref, watch } from 'vue'
import type { StorageProfile, StorageProvider, StorageProviderMeta } from '../../shared/api/storage'
import {
  getListAdminStorageProfilesQueryKey,
  useListAdminStorageProfiles,
  useListAdminStorageProviders,
} from '../../../../shared/api/generated-query/admin-storage/admin-storage'
import { formatDisplayErrorMessage } from '../../../../shared/utils/errorMessages'
import { useAppToast } from '../../../../shared/composables/useAppToast'
import {
  applyStorageProfile,
  deleteStorageProfile,
  fetchStorageBucketDomains,
  saveStorageProfile,
  testStorageProfile,
} from '../../shared/queries/adminActions'
import { buildVendorPresets, type VendorPresetOption } from './storageMeta'
import {
  buildPayload,
  canApply as canApplyProfile,
  canTest as canTestProfile,
  createEmptyProfile as createEmptyStorageProfile,
  normalizeProfile as normalizeStorageProfile,
  providerLabel as resolveProviderLabel,
  providerSupportsBucketInfo as providerHasBucketInfo,
  providerSupportsTest,
  statusClass as resolveStatusClass,
  statusText as resolveStatusText,
  storageFeatureChips as resolveStorageFeatureChips,
  storageMetaItems as resolveStorageMetaItems,
  storageSubtitle as resolveStorageSubtitle,
  toProviderMetaMap,
  type StorageCard,
} from './storageHelpers'

export function useStorageManager() {
  const appToast = useAppToast()
  const queryClient = useQueryClient()

  const providersQuery = useListAdminStorageProviders()
  const profilesQuery = useListAdminStorageProfiles()
  const saveProfileMutation = useMutation({
    mutationFn: saveStorageProfile,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: getListAdminStorageProfilesQueryKey() }),
  })
  const applyProfileMutation = useMutation({
    mutationFn: applyStorageProfile,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: getListAdminStorageProfilesQueryKey() }),
  })
  const testProfileMutation = useMutation({
    mutationFn: testStorageProfile,
  })
  const deleteProfileMutation = useMutation({
    mutationFn: deleteStorageProfile,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: getListAdminStorageProfilesQueryKey() }),
  })
  const bucketDomainsMutation = useMutation({
    mutationFn: fetchStorageBucketDomains,
  })

  const saving = ref(false)
  const savingAndApplying = ref(false)
  const fetching = ref(false)
  const applyingId = ref<string | null>(null)
  const testingId = ref<string | null>(null)
  const editorOpen = ref(false)
  const editorMode = ref<'create' | 'edit'>('create')
  const editingId = ref<string | null>(null)
  const selectedVendorKey = ref('aliyun-oss')
  const loading = computed(() => providersQuery.isLoading.value || profilesQuery.isLoading.value)

  const providers = computed<StorageProviderMeta[]>(
    () => providersQuery.data.value?.data?.providers ?? [],
  )
  const bucketDomains = ref<string[]>([])
  const providerMap = computed(() => toProviderMetaMap(providers.value))
  const creatableProviders = computed(() =>
    providers.value.filter((provider) => provider.create_allowed),
  )
  const vendorPresets = computed(() =>
    buildVendorPresets(providerMap.value, creatableProviders.value),
  )
  const storages = computed<StorageCard[]>(() =>
    (profilesQuery.data.value?.data?.profiles ?? []).map((profile) =>
      normalizeProfile({ ...profile, secret_configured: profile.secret_configured ?? false }),
    ),
  )
  const form = ref<StorageCard>(createEmptyStorageProfile('s3-compatible', providerMap.value))
  const selectedProviderMeta = computed(() => providerMap.value[form.value.provider])
  const textFields = computed(() =>
    (selectedProviderMeta.value?.fields || []).filter((field) => field.type !== 'switch'),
  )
  const switchFields = computed(() =>
    (selectedProviderMeta.value?.fields || []).filter((field) => field.type === 'switch'),
  )
  const activeVendorPreset = computed(
    () =>
      vendorPresets.value.find((vendor) => vendor.key === selectedVendorKey.value) ||
      vendorPresets.value[0],
  )
  const vendorSelectOptions = computed(() =>
    vendorPresets.value.map((vendor) => ({
      label: vendor.label,
      value: vendor.key,
      disabled: !vendor.supported,
      vendor,
    })),
  )

  watch(
    () => providersQuery.error.value || profilesQuery.error.value,
    (error) => {
      if (error) appToast.error(formatDisplayErrorMessage(error, '加载存储配置失败'))
    },
  )

  function configTextValue(key: string) {
    const value = form.value.config[key]
    return typeof value === 'string' ? value : value == null ? '' : String(value)
  }

  function configBoolValue(key: string) {
    return Boolean(form.value.config[key])
  }

  function updateConfigValue(key: string, value: string | boolean) {
    form.value.config = {
      ...form.value.config,
      [key]: value,
    }
  }

  function createEmptyProfile(provider: StorageProvider) {
    return createEmptyStorageProfile(provider, providerMap.value)
  }

  function profileFromVendorPreset(vendor: VendorPresetOption) {
    const profile = createEmptyProfile(vendor.provider)
    profile.name = vendor.defaultName
    profile.config = {
      ...profile.config,
      ...(vendor.defaultConfig || {}),
    }
    return profile
  }

  function normalizeProfile(profile: StorageProfile): StorageCard {
    return normalizeStorageProfile(profile, providerMap.value)
  }

  function providerLabel(provider: StorageProvider) {
    return resolveProviderLabel(provider, providerMap.value)
  }

  function providerSupportsBucketInfo(provider: StorageProvider) {
    return providerHasBucketInfo(provider, providerMap.value)
  }

  function supportsTest(storage: StorageCard) {
    return providerSupportsTest(storage.provider, providerMap.value)
  }

  function canTest(storage: StorageCard) {
    return canTestProfile(storage, providerMap.value)
  }

  function canApply(storage: StorageCard) {
    return canApplyProfile(storage, providerMap.value)
  }

  function statusText(storage: StorageCard) {
    return resolveStatusText(storage, providerMap.value)
  }

  function statusClass(storage: StorageCard) {
    return resolveStatusClass(storage, providerMap.value)
  }

  function storageSubtitle(storage: StorageCard) {
    return resolveStorageSubtitle(storage)
  }

  function storageMetaItems(storage: StorageCard) {
    return resolveStorageMetaItems(storage)
  }

  function storageFeatureChips(storage: StorageCard) {
    return resolveStorageFeatureChips(storage)
  }

  function handleVendorSelect(nextKey: string | undefined) {
    if (!nextKey) return
    const target = vendorPresets.value.find((item) => item.key === nextKey)
    if (!target) return
    if (!target.supported) {
      appToast.info(`${target.label} 暂未接入后端，当前先作为预设展示`)
      return
    }
    selectedVendorKey.value = target.key
    form.value = profileFromVendorPreset(target)
    bucketDomains.value = []
  }

  function openCreate() {
    const firstVendor = vendorPresets.value.find((vendor) => vendor.supported)
    if (!firstVendor) {
      appToast.warning('当前没有可新增的存储类型')
      return
    }
    openCreateWithVendor(firstVendor.key)
  }

  function openCreateWithVendor(vendorKey: string) {
    const vendor = vendorPresets.value.find((item) => item.key === vendorKey && item.supported)
    if (!vendor) return
    editorMode.value = 'create'
    editingId.value = null
    bucketDomains.value = []
    selectedVendorKey.value = vendor.key
    form.value = profileFromVendorPreset(vendor)
    editorOpen.value = true
  }

  function openEdit(id: string) {
    const target = storages.value.find((item) => item.id === id)
    if (!target) return
    editorMode.value = 'edit'
    editingId.value = id
    bucketDomains.value = []
    selectedVendorKey.value = 'aws-s3'
    form.value = {
      ...target,
      config: { ...target.config },
    }
    editorOpen.value = true
  }

  function closeEditor() {
    if (saving.value || savingAndApplying.value || fetching.value) return
    editorOpen.value = false
    editingId.value = null
    bucketDomains.value = []
  }

  async function persistForm(showSuccess = true) {
    const payload = buildPayload(form.value, providerMap.value)
    const editId = editorMode.value === 'edit' ? editingId.value : undefined
    if (editorMode.value === 'edit' && !editId) {
      appToast.error('未找到可编辑的存储配置')
      return null
    }

    const response = await saveProfileMutation.mutateAsync({ id: editId || undefined, payload })
    const saved = normalizeProfile(response.profile)
    editorMode.value = 'edit'
    editingId.value = saved.id
    form.value = {
      ...saved,
      config: { ...saved.config },
    }

    if (showSuccess) {
      appToast.success(response.message || '保存成功')
    }
    return saved
  }

  async function saveDraft() {
    saving.value = true
    try {
      const saved = await persistForm(true)
      if (!saved) return
      closeEditor()
    } catch (error) {
      appToast.error(formatDisplayErrorMessage(error, '保存失败'))
    } finally {
      saving.value = false
    }
  }

  async function saveAndApply() {
    savingAndApplying.value = true
    try {
      const saved = await persistForm(false)
      if (!saved) return

      const response = await applyProfileMutation.mutateAsync(saved.id)
      appToast.success(response.message || '已设为当前存储')
      closeEditor()
    } catch (error) {
      appToast.error(formatDisplayErrorMessage(error, '设为当前失败'))
    } finally {
      savingAndApplying.value = false
    }
  }

  async function applyStorage(id: string) {
    applyingId.value = id
    try {
      const response = await applyProfileMutation.mutateAsync(id)
      appToast.success(response.message || '已设为当前存储')
    } catch (error) {
      appToast.error(formatDisplayErrorMessage(error, '设为当前失败'))
    } finally {
      applyingId.value = null
    }
  }

  async function handleTest(storage: StorageCard) {
    testingId.value = storage.id
    try {
      const response = await testProfileMutation.mutateAsync(storage.id)
      appToast.success(response.message || '连接成功')
    } catch (error) {
      appToast.error(formatDisplayErrorMessage(error, '连接失败'))
    } finally {
      testingId.value = null
    }
  }

  async function fetchBucketDomains() {
    if (!providerSupportsBucketInfo(form.value.provider)) return

    fetching.value = true
    try {
      const saved = await persistForm(false)
      if (!saved) return

      const domains = await bucketDomainsMutation.mutateAsync(saved.id)
      if (domains.length > 0) {
        bucketDomains.value = domains
        appToast.success(`检测到 ${bucketDomains.value.length} 个域名`)
      } else {
        appToast.warning('未检测到绑定域名')
      }
    } catch (error) {
      appToast.error(formatDisplayErrorMessage(error, '获取失败'))
    } finally {
      fetching.value = false
    }
  }

  async function removeProfile(id: string) {
    try {
      const response = await deleteProfileMutation.mutateAsync(id)
      appToast.success(response.message || '删除成功')
      if (editingId.value === id) closeEditor()
    } catch (error) {
      appToast.error(formatDisplayErrorMessage(error, '删除失败'))
    }
  }

  async function loadData() {
    try {
      await Promise.all([providersQuery.refetch(), profilesQuery.refetch()])
    } catch (error) {
      appToast.error(formatDisplayErrorMessage(error, '刷新存储配置失败'))
    }
  }

  return {
    saving,
    savingAndApplying,
    fetching,
    loading,
    applyingId,
    testingId,
    editorOpen,
    editorMode,
    selectedVendorKey,
    storages,
    bucketDomains,
    form,
    creatableProviders,
    selectedProviderMeta,
    textFields,
    switchFields,
    activeVendorPreset,
    vendorSelectOptions,
    configTextValue,
    configBoolValue,
    updateConfigValue,
    providerLabel,
    supportsTest,
    canTest,
    canApply,
    statusText,
    statusClass,
    storageSubtitle,
    storageMetaItems,
    storageFeatureChips,
    handleVendorSelect,
    openCreate,
    openEdit,
    closeEditor,
    saveDraft,
    saveAndApply,
    applyStorage,
    handleTest,
    fetchBucketDomains,
    removeProfile,
    loadData,
  }
}
