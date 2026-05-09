import { computed, h, ref, watch } from 'vue'
import { useMutation, useQuery } from '@tanstack/vue-query'
import { useMessage } from 'naive-ui'
import type { StorageProfile, StorageProvider, StorageProviderMeta } from '../../../shared/api/storage'
import { extractSafeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { queryKeys } from '../../../shared/query/keys'
import {
  applyStorageProfile,
  deleteStorageProfile,
  fetchStorageBucketDomains,
  fetchStorageProfiles,
  fetchStorageProviders,
  invalidateStorageProfiles,
  saveStorageProfile,
  testStorageProfile,
} from '../../../shared/query/adminQueries'
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

type VendorSelectOption = {
  label: string
  value: string
  disabled: boolean
  vendor: VendorPresetOption
}

function vendorAccessLabel(vendor: VendorPresetOption) {
  return vendor.provider === 'kodo' ? '原生接入' : 'S3-compatible'
}

function safeErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : extractSafeApiErrorMessage(error, fallback)
}

export function useStorageManager() {
  const message = useMessage()

  const providersQuery = useQuery({
    queryKey: queryKeys.admin.storage.providers(),
    queryFn: fetchStorageProviders,
  })
  const profilesQuery = useQuery({
    queryKey: queryKeys.admin.storage.profiles(),
    queryFn: fetchStorageProfiles,
  })
  const saveProfileMutation = useMutation({
    mutationFn: saveStorageProfile,
    onSuccess: () => invalidateStorageProfiles(),
  })
  const applyProfileMutation = useMutation({
    mutationFn: applyStorageProfile,
    onSuccess: () => invalidateStorageProfiles(),
  })
  const testProfileMutation = useMutation({
    mutationFn: testStorageProfile,
  })
  const deleteProfileMutation = useMutation({
    mutationFn: deleteStorageProfile,
    onSuccess: () => invalidateStorageProfiles(),
  })
  const bucketDomainsMutation = useMutation({
    mutationFn: fetchStorageBucketDomains,
  })

  const loading = computed(() =>
    providersQuery.isLoading.value ||
    providersQuery.isFetching.value ||
    profilesQuery.isLoading.value ||
    profilesQuery.isFetching.value,
  )
  const saving = ref(false)
  const savingAndApplying = ref(false)
  const fetching = ref(false)
  const applyingId = ref<string | null>(null)
  const testingId = ref<string | null>(null)
  const editorOpen = ref(false)
  const editorMode = ref<'create' | 'edit'>('create')
  const editingId = ref<string | null>(null)
  const selectedVendorKey = ref('kodo')

  const providers = computed<StorageProviderMeta[]>(() => providersQuery.data.value ?? [])
  const bucketDomains = ref<string[]>([])
  const providerMap = computed(() => toProviderMetaMap(providers.value))
  const creatableProviders = computed(() => providers.value.filter((provider) => provider.create_allowed))
  const vendorPresets = computed(() => buildVendorPresets(providerMap.value, creatableProviders.value))
  const storages = computed<StorageCard[]>(() => (profilesQuery.data.value ?? []).map(normalizeProfile))
  const form = ref<StorageCard>(createEmptyStorageProfile('kodo', providerMap.value))
  const selectedProviderMeta = computed(() => providerMap.value[form.value.provider])
  const textFields = computed(() => (selectedProviderMeta.value?.fields || []).filter((field) => field.type !== 'switch'))
  const switchFields = computed(() => (selectedProviderMeta.value?.fields || []).filter((field) => field.type === 'switch'))
  const activeVendorPreset = computed(() => vendorPresets.value.find((vendor) => vendor.key === selectedVendorKey.value) || vendorPresets.value[0])
  const vendorSelectOptions = computed(() =>
    vendorPresets.value.map((vendor) => ({
      label: vendor.label,
      value: vendor.key,
      disabled: !vendor.supported,
      vendor,
    }))
  )

  watch(
    () => providersQuery.error.value || profilesQuery.error.value,
    (error) => {
      if (error) message.error(safeErrorMessage(error, '加载存储配置失败'))
    },
  )

  function toChipVariant(status: string): 'neutral' | 'current' | 'enabled' | 'empty' {
    switch (status) {
      case 'is-current':
        return 'current'
      case 'is-enabled':
        return 'enabled'
      case 'is-empty':
        return 'empty'
      default:
        return 'neutral'
    }
  }

  function asVendorSelectOption(input: unknown): VendorSelectOption | null {
    if (!input || typeof input !== 'object') return null
    const candidate = input as Partial<VendorSelectOption>
    if (typeof candidate.label !== 'string' || typeof candidate.value !== 'string' || !candidate.vendor) {
      return null
    }
    return candidate as VendorSelectOption
  }

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

  function handleVendorSelect(nextKey: string) {
    const target = vendorPresets.value.find((item) => item.key === nextKey)
    if (!target) return
    if (!target.supported) {
      message.info(`${target.label} 暂未接入后端，当前先作为预设展示`)
      return
    }
    selectedVendorKey.value = target.key
    form.value = profileFromVendorPreset(target)
    bucketDomains.value = []
  }

  function renderVendorLabel(option: unknown) {
    const parsed = asVendorSelectOption(option)
    if (!parsed) return ''
    const vendor = parsed.vendor
    return h('div', { class: 'vendor-select-entry' }, [
      h('div', { class: 'vendor-select-entry-brand' }, [
        h('img', {
          src: vendor.logoSrc,
          alt: vendor.label,
          class: 'vendor-select-entry-logo',
        }),
      ]),
      h('div', { class: 'vendor-select-entry-copy' }, [
        h('div', { class: 'vendor-select-entry-top' }, [
          h('span', { class: 'vendor-select-entry-name' }, vendor.label),
          h('span', { class: 'vendor-select-entry-family' }, vendorAccessLabel(vendor)),
        ]),
      ]),
    ])
  }

  function openCreate() {
    const firstVendor = vendorPresets.value.find((vendor) => vendor.supported)
    if (!firstVendor) {
      message.warning('当前没有可新增的存储类型')
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
    selectedVendorKey.value = target.provider === 'kodo' ? 'kodo' : 'aws-s3'
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
      message.error('未找到可编辑的存储配置')
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
      message.success(response.message || '保存成功')
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
      message.error(safeErrorMessage(error, '保存失败'))
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
      message.success(response.message || '已设为当前存储')
      closeEditor()
    } catch (error) {
      message.error(safeErrorMessage(error, '设为当前失败'))
    } finally {
      savingAndApplying.value = false
    }
  }

  async function applyStorage(id: string) {
    applyingId.value = id
    try {
      const response = await applyProfileMutation.mutateAsync(id)
      message.success(response.message || '已设为当前存储')
    } catch (error) {
      message.error(safeErrorMessage(error, '设为当前失败'))
    } finally {
      applyingId.value = null
    }
  }

  async function handleTest(storage: StorageCard) {
    testingId.value = storage.id
    try {
      const response = await testProfileMutation.mutateAsync(storage.id)
      message.success(response.message || '连接成功')
    } catch (error) {
      message.error(safeErrorMessage(error, '连接失败'))
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
        message.success(`检测到 ${bucketDomains.value.length} 个域名`)
      } else {
        message.warning('未检测到绑定域名')
      }
    } catch (error) {
      message.error(safeErrorMessage(error, '获取失败'))
    } finally {
      fetching.value = false
    }
  }

  async function removeProfile(id: string) {
    try {
      const response = await deleteProfileMutation.mutateAsync(id)
      message.success(response.message || '删除成功')
      if (editingId.value === id) closeEditor()
    } catch (error) {
      message.error(safeErrorMessage(error, '删除失败'))
    }
  }

  return {
    loading,
    saving,
    savingAndApplying,
    fetching,
    applyingId,
    testingId,
    editorOpen,
    editorMode,
    editingId,
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
    toChipVariant,
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
    renderVendorLabel,
    openCreate,
    openEdit,
    closeEditor,
    saveDraft,
    saveAndApply,
    applyStorage,
    handleTest,
    fetchBucketDomains,
    removeProfile,
  }
}
