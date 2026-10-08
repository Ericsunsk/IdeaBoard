import { computed, ref } from 'vue'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { useStorageFeedback } from './useStorageFeedback'
import { buildVendorPresets, type VendorPresetOption } from './meta'
import {
  buildPayload,
  createEmptyProfile as createEmptyStorageProfile,
  normalizeProfile,
  providerLabel as resolveProviderLabel,
  type ProviderMetaMap,
  type StorageCard,
  type StorageProfile,
  type StorageProfilePayload,
  type StorageProvider,
  type StorageProviderMeta,
} from './profiles'

type SaveStorageProfile = (input: {
  id?: string | undefined
  payload: StorageProfilePayload
}) => Promise<{ profile: StorageProfile; message: string } | null>

type StorageProfileEditorOptions = {
  creatableProviders: { value: StorageProviderMeta[] }
  providerMap: { value: ProviderMetaMap }
  storages: { value: StorageCard[] }
  saveProfile: SaveStorageProfile
  fetchBucketDomains: (id: string) => Promise<string[]>
}

export function useStorageProfileEditor(options: StorageProfileEditorOptions) {
  const storageFeedback = useStorageFeedback()
  const { runAsyncAction } = useAsyncAction()
  const saving = ref(false)
  const fetching = ref(false)
  const editorOpen = ref(false)
  const editorMode = ref<'create' | 'edit'>('create')
  const editingId = ref<string | null>(null)
  const bucketDomains = ref<string[]>([])
  const {
    view: formView,
    vendorPresets,
    profileFromVendorPreset,
  } = createStorageProfileForm(options)
  const { selectedVendorKey, form, selectedProviderMeta } = formView

  function handleVendorSelect(nextKey: string | undefined) {
    if (!nextKey) return
    const target = vendorPresets.value.find((item) => item.key === nextKey)
    if (!target) return
    if (!target.supported) {
      storageFeedback.unsupportedVendorPreset(target.label)
      return
    }
    selectedVendorKey.value = target.key
    form.value = profileFromVendorPreset(target)
    bucketDomains.value = []
  }

  function openCreate() {
    const firstVendor = vendorPresets.value.find((vendor) => vendor.supported)
    if (!firstVendor) {
      storageFeedback.noCreatableProviders()
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
    if (saving.value || fetching.value) return
    editorOpen.value = false
    editingId.value = null
    bucketDomains.value = []
  }

  async function persistForm(showSuccess = true) {
    const payload = buildPayload(form.value, options.providerMap.value)
    const editId = editorMode.value === 'edit' ? editingId.value : undefined
    if (editorMode.value === 'edit' && !editId) {
      storageFeedback.missingEditableTarget()
      return null
    }

    const response = await options.saveProfile({
      id: editId || undefined,
      payload,
    })
    if (!response) return null
    const saved = normalizeProfile(response.profile, options.providerMap.value)
    editorMode.value = 'edit'
    editingId.value = saved.id
    form.value = {
      ...saved,
      config: { ...saved.config },
    }

    if (showSuccess) storageFeedback.saveDraftSucceeded(response.message)
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
        feedback: {
          error: storageFeedback.saveDraftFailed,
        },
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
          storageFeedback.bucketDomainsDetected(bucketDomains.value.length)
        } else {
          storageFeedback.bucketDomainsEmpty()
        }
      },
      {
        loading: fetching,
        feedback: {
          error: storageFeedback.fetchBucketDomainsFailed,
        },
      },
    )
  }

  return {
    saving,
    fetching,
    editorOpen,
    editorMode,
    bucketDomains,
    ...formView,
    handleVendorSelect,
    openCreate,
    openEdit,
    closeEditor,
    saveDraft,
    fetchBucketDomains,
  }
}

function createStorageProfileForm(
  options: Pick<StorageProfileEditorOptions, 'creatableProviders' | 'providerMap'>,
) {
  const selectedVendorKey = ref('aliyun-oss')
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

  return {
    vendorPresets,
    profileFromVendorPreset,
    view: {
      selectedVendorKey,
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
    },
  }
}
