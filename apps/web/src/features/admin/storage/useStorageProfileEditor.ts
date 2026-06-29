import { computed, ref } from 'vue'
import { useAppToast } from '../../../shared/composables/useAppToast'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { buildVendorPresets, type VendorPresetOption } from './storageMeta'
import {
  buildPayload,
  canApply as canApplyProfile,
  canTest as canTestProfile,
  createEmptyProfile as createEmptyStorageProfile,
  normalizeProfile,
  providerLabel as resolveProviderLabel,
  providerSupportsTest,
  statusClass as resolveStatusClass,
  statusText as resolveStatusText,
  storageMetaItems as resolveStorageMetaItems,
  storageSubtitle as resolveStorageSubtitle,
  type ProviderMetaMap,
  type StorageCard,
  type StorageProfile,
  type StorageProfilePayload,
  type StorageProvider,
  type StorageProviderMeta,
} from './storageHelpers'

type SaveStorageProfile = (input: {
  id?: string
  payload: StorageProfilePayload
  apply?: boolean
}) => Promise<{ profile: StorageProfile; message: string }>

type StorageProfileCommand = (id: string) => Promise<{ profile: StorageProfile; message: string }>

export function useStorageProfileEditor(options: {
  creatableProviders: { value: StorageProviderMeta[] }
  providerMap: { value: ProviderMetaMap }
  storages: { value: StorageCard[] }
  saveProfile: SaveStorageProfile
  applyProfile: StorageProfileCommand
  testProfile: (id: string) => Promise<{ message: string }>
  deleteProfile: (id: string) => Promise<{ message: string }>
  fetchBucketDomains: (id: string) => Promise<string[]>
}) {
  const appToast = useAppToast()
  const { runAsyncAction } = useAsyncAction()
  const saving = ref(false)
  const savingAndApplying = ref(false)
  const fetching = ref(false)
  const applyingId = ref<string | null>(null)
  const testingId = ref<string | null>(null)
  const editorOpen = ref(false)
  const editorMode = ref<'create' | 'edit'>('create')
  const editingId = ref<string | null>(null)
  const selectedVendorKey = ref('aliyun-oss')
  const bucketDomains = ref<string[]>([])
  const form = ref<StorageCard>(
    createEmptyStorageProfile('s3-compatible', options.providerMap.value),
  )
  const vendorPresets = computed(() => buildVendorPresets(options.creatableProviders.value))
  const selectedProviderMeta = computed(() => options.providerMap.value[form.value.provider])
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
    return createEmptyStorageProfile(provider, options.providerMap.value)
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

  function providerLabel(provider: StorageProvider) {
    return resolveProviderLabel(provider, options.providerMap.value)
  }

  function supportsTest(storage: StorageCard) {
    return providerSupportsTest(storage.provider, options.providerMap.value)
  }

  function canTest(storage: StorageCard) {
    return canTestProfile(storage, options.providerMap.value)
  }

  function canApply(storage: StorageCard) {
    return canApplyProfile(storage, options.providerMap.value)
  }

  function statusText(storage: StorageCard) {
    return resolveStatusText(storage, options.providerMap.value)
  }

  function statusClass(storage: StorageCard) {
    return resolveStatusClass(storage, options.providerMap.value)
  }

  function storageSubtitle(storage: StorageCard) {
    return resolveStorageSubtitle(storage)
  }

  function storageMetaItems(storage: StorageCard) {
    return resolveStorageMetaItems(storage)
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
    const target = options.storages.value.find((item) => item.id === id)
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
    const payload = buildPayload(form.value, options.providerMap.value)
    const editId = editorMode.value === 'edit' ? editingId.value : undefined
    if (editorMode.value === 'edit' && !editId) {
      appToast.error('未找到可编辑的存储配置')
      return null
    }

    const response = await options.saveProfile({
      id: editId || undefined,
      payload,
      apply: apply || undefined,
    })
    const saved = normalizeProfile(response.profile, options.providerMap.value)
    editorMode.value = 'edit'
    editingId.value = saved.id
    form.value = {
      ...saved,
      config: { ...saved.config },
    }

    if (showSuccess) appToast.success(response.message || '保存成功')
    return saved
  }

  async function saveDraft() {
    await runAsyncAction(
      async () => {
        const saved = await persistForm(true)
        if (!saved) return
        closeEditor()
      },
      {
        loading: saving,
        errorMessage: '保存失败',
      },
    )
  }

  async function saveAndApply() {
    await runAsyncAction(
      async () => {
        const saved = await persistForm(false, true)
        if (!saved) return
        appToast.success('已保存并设为当前存储')
        closeEditor()
      },
      {
        loading: savingAndApplying,
        errorMessage: '设为当前失败',
      },
    )
  }

  async function applyStorage(id: string) {
    await runAsyncAction(
      async () => {
        const response = await options.applyProfile(id)
        appToast.success(response.message || '已设为当前存储')
      },
      {
        before: () => {
          applyingId.value = id
        },
        after: () => {
          applyingId.value = null
        },
        errorMessage: '设为当前失败',
      },
    )
  }

  async function handleTest(storage: StorageCard) {
    await runAsyncAction(
      async () => {
        const response = await options.testProfile(storage.id)
        appToast.success(response.message || '连接成功')
      },
      {
        before: () => {
          testingId.value = storage.id
        },
        after: () => {
          testingId.value = null
        },
        errorMessage: '连接失败',
      },
    )
  }

  async function fetchBucketDomains() {
    if (!selectedProviderMeta.value?.supports_bucket_info) return

    await runAsyncAction(
      async () => {
        const saved = await persistForm(false)
        if (!saved) return

        const domains = await options.fetchBucketDomains(saved.id)
        if (domains.length > 0) {
          bucketDomains.value = domains
          appToast.success(`检测到 ${bucketDomains.value.length} 个域名`)
        } else {
          appToast.warning('未检测到绑定域名')
        }
      },
      {
        loading: fetching,
        errorMessage: '获取失败',
      },
    )
  }

  async function removeProfile(id: string) {
    await runAsyncAction(
      async () => {
        const response = await options.deleteProfile(id)
        appToast.success(response.message || '删除成功')
        if (editingId.value === id) closeEditor()
      },
      {
        errorMessage: '删除失败',
      },
    )
  }

  return {
    saving,
    savingAndApplying,
    fetching,
    applyingId,
    testingId,
    editorOpen,
    editorMode,
    selectedVendorKey,
    bucketDomains,
    form,
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
  }
}
