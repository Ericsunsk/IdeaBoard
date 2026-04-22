import { computed, h, onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { storageApi } from '../../api/index'
import type { StorageProfile, StorageProvider, StorageProviderMeta } from '../../api/index'
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

export function useStorageManager() {
  const message = useMessage()

  const loading = ref(false)
  const saving = ref(false)
  const savingAndApplying = ref(false)
  const fetching = ref(false)
  const applyingId = ref<string | null>(null)
  const testingId = ref<string | null>(null)
  const editorOpen = ref(false)
  const editorMode = ref<'create' | 'edit'>('create')
  const editingId = ref<string | null>(null)
  const selectedVendorKey = ref('kodo')

  const providers = ref<StorageProviderMeta[]>([])
  const storages = ref<StorageCard[]>([])
  const bucketDomains = ref<string[]>([])
  const providerMap = computed(() => toProviderMetaMap(providers.value))
  const creatableProviders = computed(() => providers.value.filter((provider) => provider.create_allowed))
  const vendorPresets = computed(() => buildVendorPresets(providerMap.value, creatableProviders.value))
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
    return h('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
        minWidth: '0',
        width: '100%',
      },
    }, [
      h('img', {
        src: vendor.logoSrc,
        alt: vendor.label,
        style: {
          width: '24px',
          height: '24px',
          flex: '0 0 24px',
          objectFit: 'contain',
          borderRadius: '7px',
          background: '#ffffff',
          padding: '3px',
        },
      }),
      h('span', {
        style: {
          minWidth: '0',
          color: '#f4f4f5',
          fontSize: '14px',
          fontWeight: 600,
          lineHeight: 1.2,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        },
      }, vendor.label),
      h('span', {
        style: {
          marginLeft: 'auto',
          minHeight: '20px',
          padding: '0 6px',
          display: 'inline-flex',
          alignItems: 'center',
          borderRadius: '999px',
          background: vendor.supported ? 'rgba(16, 185, 129, 0.12)' : '#1b1c1f',
          border: vendor.supported ? '1px solid rgba(16, 185, 129, 0.24)' : '1px solid #232326',
          color: vendor.supported ? '#d1fae5' : '#6b6b71',
          fontSize: '10px',
          lineHeight: 1,
          whiteSpace: 'nowrap',
          flex: '0 0 auto',
        },
      }, vendor.supported ? '已接入' : '即将支持'),
    ])
  }

  function renderVendorOption(payload: { option: unknown }) {
    const parsed = asVendorSelectOption(payload.option)
    if (!parsed) return ''
    const vendor = parsed.vendor
    const supported = vendor.supported
    return h('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
        minWidth: '0',
        width: '100%',
        padding: '2px 0',
      },
    }, [
      h('img', {
        src: vendor.logoSrc,
        alt: vendor.label,
        style: {
          width: '28px',
          height: '28px',
          flex: '0 0 28px',
          objectFit: 'contain',
          borderRadius: '8px',
          background: '#ffffff',
          padding: '4px',
        },
      }),
      h('span', {
        style: {
          minWidth: '0',
          flex: '1',
          color: '#f4f4f5',
          fontSize: '13px',
          fontWeight: 600,
          lineHeight: 1.2,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        },
      }, vendor.label),
      h('span', {
        style: {
          minHeight: '20px',
          padding: '0 6px',
          display: 'inline-flex',
          alignItems: 'center',
          borderRadius: '999px',
          background: supported ? 'rgba(16, 185, 129, 0.12)' : '#1b1c1f',
          border: supported ? '1px solid rgba(16, 185, 129, 0.24)' : '1px solid #232326',
          color: supported ? '#d1fae5' : '#6b6b71',
          fontSize: '10px',
          lineHeight: 1,
          whiteSpace: 'nowrap',
          flex: '0 0 auto',
        },
      }, supported ? '已接入' : '即将支持'),
    ])
  }

  function errorMessage(error: unknown, fallback: string) {
    if (typeof error === 'object' && error && 'msg' in error && typeof (error as { msg?: unknown }).msg === 'string') {
      return (error as { msg: string }).msg
    }
    return fallback
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

  async function loadStorageData() {
    loading.value = true
    try {
      const [providersRes, profilesRes] = await Promise.all([
        storageApi.getProviders(),
        storageApi.getProfiles(),
      ])

      if (providersRes.code === 0 && providersRes.data?.providers) {
        providers.value = providersRes.data.providers
      }

      if (profilesRes.code === 0 && profilesRes.data?.profiles) {
        storages.value = profilesRes.data.profiles.map(normalizeProfile)
      }
    } catch (error) {
      console.error('Failed to load storage profiles', error)
      message.error('加载存储配置失败')
    } finally {
      loading.value = false
    }
  }

  async function reloadProfiles() {
    const response = await storageApi.getProfiles()
    if (response.code === 0 && response.data?.profiles) {
      storages.value = response.data.profiles.map(normalizeProfile)
    }
  }

  async function persistForm(showSuccess = true) {
    const payload = buildPayload(form.value, providerMap.value)
    let response
    if (editorMode.value === 'create') {
      response = await storageApi.createProfile(payload)
    } else {
      const editId = editingId.value
      if (!editId) {
        message.error('未找到可编辑的存储配置')
        return null
      }
      response = await storageApi.updateProfile(editId, payload)
    }

    if (response.code !== 0 || !response.data?.profile) {
      message.error(response.msg || '保存失败')
      return null
    }

    const saved = normalizeProfile(response.data.profile)
    editorMode.value = 'edit'
    editingId.value = saved.id
    form.value = {
      ...saved,
      config: { ...saved.config },
    }
    await reloadProfiles()

    if (showSuccess) {
      message.success(response.data?.message || '保存成功')
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
      message.error(errorMessage(error, '保存失败'))
    } finally {
      saving.value = false
    }
  }

  async function saveAndApply() {
    savingAndApplying.value = true
    try {
      const saved = await persistForm(false)
      if (!saved) return

      const response = await storageApi.applyProfile(saved.id)
      if (response.code !== 0) {
        message.error(response.msg || '设为当前失败')
        return
      }

      await reloadProfiles()
      message.success(response.data?.message || '已设为当前存储')
      closeEditor()
    } catch (error) {
      message.error(errorMessage(error, '设为当前失败'))
    } finally {
      savingAndApplying.value = false
    }
  }

  async function applyStorage(id: string) {
    applyingId.value = id
    try {
      const response = await storageApi.applyProfile(id)
      if (response.code === 0) {
        await reloadProfiles()
        message.success(response.data?.message || '已设为当前存储')
      } else {
        message.error(response.msg || '设为当前失败')
      }
    } catch (error) {
      message.error(errorMessage(error, '设为当前失败'))
    } finally {
      applyingId.value = null
    }
  }

  async function handleTest(storage: StorageCard) {
    testingId.value = storage.id
    try {
      const response = await storageApi.testProfile(storage.id)
      if (response.code === 0) {
        message.success(response.data?.message || '连接成功')
      } else {
        message.error(response.msg || '连接失败')
      }
    } catch (error) {
      message.error(errorMessage(error, '连接失败'))
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

      const response = await storageApi.getBucketInfo(saved.id)
      if (response.code === 0 && response.data?.info?.domains?.length) {
        bucketDomains.value = response.data.info.domains
        message.success(`检测到 ${bucketDomains.value.length} 个域名`)
      } else {
        message.warning(response.msg || '未检测到绑定域名')
      }
    } catch (error) {
      message.error(errorMessage(error, '获取失败'))
    } finally {
      fetching.value = false
    }
  }

  async function removeProfile(id: string) {
    try {
      const response = await storageApi.deleteProfile(id)
      if (response.code === 0) {
        storages.value = storages.value.filter((item) => item.id !== id)
        message.success(response.data?.message || '删除成功')
        if (editingId.value === id) closeEditor()
      } else {
        message.error(response.msg || '删除失败')
      }
    } catch (error) {
      message.error(errorMessage(error, '删除失败'))
    }
  }

  onMounted(loadStorageData)

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
    renderVendorOption,
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
