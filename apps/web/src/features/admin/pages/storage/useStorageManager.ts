import { computed, ref, watch } from 'vue'
import type {
  StorageProfile,
  StorageProfilePayload,
  StorageProvider,
  StorageProviderMeta,
} from '../../shared/api/storage'
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
  mockStorageProfiles,
  mockStorageProviders,
  useAdminMockMode,
} from '../../shared/lib/adminMockData'
import {
  buildPayload,
  canApply as canApplyProfile,
  canTest as canTestProfile,
  createEmptyProfile as createEmptyStorageProfile,
  normalizeProfile as normalizeStorageProfile,
  providerLabel as resolveProviderLabel,
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
  const isMockMode = useAdminMockMode()
  const mockProfiles = ref<StorageProfile[]>(copyMockStorageProfiles())

  const providersQuery = useListAdminStorageProviders({
    query: {
      enabled: computed(() => !isMockMode.value),
    },
  })
  const profilesQuery = useListAdminStorageProfiles({
    query: {
      enabled: computed(() => !isMockMode.value),
    },
  })
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
  const loading = computed(
    () => !isMockMode.value && (providersQuery.isLoading.value || profilesQuery.isLoading.value),
  )

  const providers = computed<StorageProviderMeta[]>(() =>
    isMockMode.value ? mockStorageProviders : (providersQuery.data.value?.data?.providers ?? []),
  )
  const bucketDomains = ref<string[]>([])
  const providerMap = computed(() => toProviderMetaMap(providers.value))
  const creatableProviders = computed(() =>
    providers.value.filter((provider) => provider.create_allowed),
  )
  const vendorPresets = computed(() => buildVendorPresets(creatableProviders.value))
  const storages = computed<StorageCard[]>(() =>
    (isMockMode.value ? mockProfiles.value : (profilesQuery.data.value?.data?.profiles ?? [])).map(
      (profile) =>
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
      if (!isMockMode.value && error) {
        appToast.error(formatDisplayErrorMessage(error, '加载存储配置失败'))
      }
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

  function persistMockProfile(
    payload: StorageProfilePayload,
    editId: string | null | undefined,
    apply: boolean,
  ): StorageCard {
    const existing = editId
      ? mockProfiles.value.find((profile) => profile.id === editId)
      : undefined
    const id = editId || `mock-storage-${Date.now()}`
    const nextProfile: StorageProfile = {
      id,
      name: payload.name,
      provider: payload.provider,
      enabled: payload.enabled,
      is_current: apply || Boolean(existing?.is_current),
      builtin: existing?.builtin ?? false,
      secret_configured: true,
      config: { ...payload.config },
    }

    const nextProfiles = nextProfile.is_current
      ? mockProfiles.value.map((profile) => ({ ...profile, is_current: false }))
      : mockProfiles.value

    mockProfiles.value = existing
      ? nextProfiles.map((profile) => (profile.id === id ? nextProfile : profile))
      : [...nextProfiles, nextProfile]

    return normalizeProfile(nextProfile)
  }

  function providerLabel(provider: StorageProvider) {
    return resolveProviderLabel(provider, providerMap.value)
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
    editorMode.value = 'create'
    editingId.value = null
    bucketDomains.value = []
    selectedVendorKey.value = firstVendor.key
    form.value = profileFromVendorPreset(firstVendor)
    editorOpen.value = true
  }

  function openEdit(id: string) {
    const target = storages.value.find((item) => item.id === id)
    if (!target) return
    editorMode.value = 'edit'
    editingId.value = id
    bucketDomains.value = []
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

  async function persistForm(showSuccess = true, apply = false) {
    const payload = buildPayload(form.value, providerMap.value)
    const editId = editorMode.value === 'edit' ? editingId.value : undefined
    if (editorMode.value === 'edit' && !editId) {
      appToast.error('未找到可编辑的存储配置')
      return null
    }

    if (isMockMode.value) {
      const saved = persistMockProfile(payload, editId, apply)
      editorMode.value = 'edit'
      editingId.value = saved.id
      form.value = {
        ...saved,
        config: { ...saved.config },
      }
      if (showSuccess) {
        appToast.success('保存成功')
      }
      return saved
    }

    const response = await saveProfileMutation.mutateAsync({
      id: editId || undefined,
      payload,
      options: apply ? { apply: true } : undefined,
    })
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
      const saved = await persistForm(false, true)
      if (!saved) return

      appToast.success('已保存并设为当前存储')
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
      if (isMockMode.value) {
        mockProfiles.value = mockProfiles.value.map((profile) => ({
          ...profile,
          is_current: profile.id === id,
        }))
        appToast.success('已设为当前存储')
        return
      }
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
      if (isMockMode.value) {
        appToast.success('连接成功')
        return
      }
      const response = await testProfileMutation.mutateAsync(storage.id)
      appToast.success(response.message || '连接成功')
    } catch (error) {
      appToast.error(formatDisplayErrorMessage(error, '连接失败'))
    } finally {
      testingId.value = null
    }
  }

  async function fetchBucketDomains() {
    if (!selectedProviderMeta.value?.supports_bucket_info) return

    fetching.value = true
    try {
      const saved = await persistForm(false)
      if (!saved) return

      if (isMockMode.value) {
        bucketDomains.value = ['assets.example.com', 'cdn-assets.example.com']
        appToast.success(`检测到 ${bucketDomains.value.length} 个域名`)
        return
      }

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
      if (isMockMode.value) {
        const target = mockProfiles.value.find((profile) => profile.id === id)
        if (!target || target.builtin) {
          appToast.warning('内置存储不可删除')
          return
        }
        mockProfiles.value = mockProfiles.value.filter((profile) => profile.id !== id)
        appToast.success('删除成功')
        if (editingId.value === id) closeEditor()
        return
      }
      const response = await deleteProfileMutation.mutateAsync(id)
      appToast.success(response.message || '删除成功')
      if (editingId.value === id) closeEditor()
    } catch (error) {
      appToast.error(formatDisplayErrorMessage(error, '删除失败'))
    }
  }

  async function loadData() {
    if (isMockMode.value) return
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

function copyMockStorageProfiles(): StorageProfile[] {
  return mockStorageProfiles.map((profile) => ({
    ...profile,
    secret_configured: profile.secret_configured ?? false,
    config: { ...profile.config },
  }))
}
