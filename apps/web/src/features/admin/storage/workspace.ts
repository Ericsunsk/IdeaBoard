import { computed, ref } from 'vue'
import type { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import {
  ADMIN_PROBE_OPERATOR_REASON,
  confirmSettingsSave,
  requireDestructiveReason,
  type AdminConfirmRisk,
} from '../shared/lib/confirmSettingsSave'
import {
  createAdminStorageResource,
  type AdminStorageResource,
  type SaveStorageProfileInput,
  type StorageProfileCommandResult,
} from './resource'
import { storageVendorIcon, storageVendorLogo } from './meta'
import {
  buildPayload,
  canApply as canApplyProfile,
  canTest as canTestProfile,
  normalizeProfile,
  providerLabel as resolveProviderLabel,
  providerSupportsTest,
  statusClass as resolveStatusClass,
  statusText as resolveStatusText,
  storageMetaItems as resolveStorageMetaItems,
  storageStatusSeverity,
  storageDisableReason,
  storageSubtitle as resolveStorageSubtitle,
  toProviderMetaMap,
  type StorageCard,
  type StorageProviderMeta,
} from './profiles'
import type { AdminStorageFeedback } from './useStorageFeedback'

type RunAsyncAction = ReturnType<typeof useAsyncAction>['runAsyncAction']

export type AdminStorageWorkspaceOptions = {
  feedback: AdminStorageFeedback
  confirm: AdminConfirmPort
  runAsyncAction: RunAsyncAction
  resource?: AdminStorageResource | undefined
}

export type StorageProfileCommandTarget = Pick<StorageCard, 'id' | 'name'>

type DeleteStorageProfileCommandOptions = {
  afterDeleted?: (id: string) => void
}

export function createAdminStorageWorkspace(options: AdminStorageWorkspaceOptions) {
  const resource = options.resource ?? createAdminStorageResource()
  const loading = ref(false)
  const loadFailed = ref(false)
  const togglingId = ref<string | null>(null)
  const togglingPending = ref(false)
  const applyingId = ref<string | null>(null)
  const testingId = ref<string | null>(null)
  const applyingPending = ref(false)
  const testingPending = ref(false)
  const providers = ref<StorageProviderMeta[]>([])
  const storages = ref<StorageCard[]>([])
  const providerMap = computed(() => toProviderMetaMap(providers.value))
  const creatableProviders = computed(() =>
    providers.value.filter((provider) => provider.create_allowed),
  )

  const cardViews = computed(() => storages.value.map(createStorageCardView))

  async function refreshStorageData() {
    const [nextProviders, nextProfiles] = await Promise.all([
      resource.fetchProviders(),
      resource.fetchProfiles(),
    ])
    const nextProviderMap = toProviderMetaMap(nextProviders)
    providers.value = nextProviders
    storages.value = nextProfiles.map((profile) => normalizeProfile(profile, nextProviderMap))
  }

  async function loadData() {
    await options.runAsyncAction(refreshStorageData, {
      loading,
      before: () => {
        loadFailed.value = false
      },
      feedback: {
        error: (error) => {
          loadFailed.value = true
          options.feedback.loadProfilesFailed(error)
        },
      },
    })
  }

  async function refreshData() {
    await options.runAsyncAction(refreshStorageData, {
      loading,
      feedback: {
        error: options.feedback.refreshFailed,
      },
    })
  }

  async function refreshAfterCommand() {
    try {
      await refreshStorageData()
    } catch (error) {
      options.feedback.refreshFailed(error)
    }
  }

  // Workspace owns every storage mutation's confirmation, operator reason,
  // and refresh-after-command. The profile editor only supplies form payloads
  // through saveProfile; apply/test/delete/enable orchestration lives here.
  async function saveProfile(
    input: SaveStorageProfileInput,
  ): Promise<StorageProfileCommandResult | null> {
    const operatorReason = await confirmStorageWrite({
      title: input.id ? '保存存储配置' : '新建存储配置',
      message: '确定保存该存储配置吗？该操作将记录操作人与原因。',
      confirmLabel: '保存配置',
    })
    if (!operatorReason) return null
    const response = await resource.saveProfile(input, { operatorReason })
    await refreshAfterCommand()
    return response
  }

  async function applyStorage(target: StorageProfileCommandTarget) {
    const storage = resolveStorageTarget(target)
    if (!storage) return

    await options.runAsyncAction(
      async () => {
        const operatorReason = await confirmStorageWrite({
          title: '设为当前存储',
          message: '确定将该存储设为当前存储吗？该操作将记录操作人与原因。',
          confirmLabel: '设为当前',
        })
        if (!operatorReason) return
        const response = await resource.applyProfile(storage.id, { operatorReason })
        await refreshAfterCommand()
        options.feedback.applyCurrentSucceeded(response.message)
      },
      {
        loading: applyingPending,
        before: () => {
          applyingId.value = storage.id
        },
        after: () => {
          applyingId.value = null
        },
        feedback: {
          error: options.feedback.applyCurrentFailed,
        },
      },
    )
  }

  async function toggleEnabled(target: StorageProfileCommandTarget, enabled: boolean) {
    const storage = resolveStorageTarget(target)
    if (!storage || (!enabled && storageDisableReason(storage))) return

    await options.runAsyncAction(
      async () => {
        const response = await saveProfile({
          id: storage.id,
          payload: buildPayload({ ...storage, enabled }, providerMap.value),
        })
        if (response) options.feedback.saveDraftSucceeded(response.message)
      },
      {
        loading: togglingPending,
        before: () => {
          togglingId.value = storage.id
        },
        after: () => {
          togglingId.value = null
        },
        feedback: { error: options.feedback.saveDraftFailed },
      },
    )
  }

  async function testStorage(target: StorageProfileCommandTarget) {
    const storage = resolveStorageTarget(target)
    if (!storage) return

    await options.runAsyncAction(
      async () => {
        // Connectivity probes skip the confirm dialog (backend POST
        // read-guard: no reason ceremony); the fixed probe note still lands
        // on the audit record.
        const response = await resource.testProfile(storage.id, {
          operatorReason: ADMIN_PROBE_OPERATOR_REASON,
        })
        if (!response) return
        options.feedback.testConnectionSucceeded(response.message)
      },
      {
        loading: testingPending,
        before: () => {
          testingId.value = storage.id
        },
        after: () => {
          testingId.value = null
        },
        feedback: {
          error: options.feedback.testConnectionFailed,
        },
      },
    )
  }

  async function requestRemoveProfile(
    target: StorageProfileCommandTarget,
    commandOptions: DeleteStorageProfileCommandOptions = {},
  ) {
    const storage = resolveStorageTarget(target)
    if (!storage) return

    const confirmed = await options.confirm.confirm({
      title: '确认删除存储',
      message: `确定删除存储「${storage.name}」吗？此操作不可恢复，且将记录操作人与原因。`,
      confirmLabel: '确定删除',
      intent: 'danger',
      reason: { required: true },
    })
    if (!confirmed) return
    const operatorReason = requireDestructiveReason(options.confirm)
    if (!operatorReason) return
    try {
      const response = await resource.deleteProfile(storage.id, { operatorReason })
      options.feedback.profileDeleted(response.message)
      commandOptions.afterDeleted?.(storage.id)
      await refreshAfterCommand()
    } catch (error) {
      options.feedback.deleteProfileFailed(error)
    }
  }

  function fetchBucketDomains(id: string) {
    return resource.fetchBucketDomains(id)
  }

  // Control storage mutations require an operator reason (audit). Collect
  // it through the shared confirm dialog; a cancelled dialog aborts
  // silently (null) so no failure feedback fires. Reversible writes default
  // to the optional-note tier; destructive deletes pass risk explicitly.
  function confirmStorageWrite(
    dialog: {
      title: string
      message: string
      confirmLabel: string
    },
    risk: AdminConfirmRisk = 'reversible',
  ): Promise<string | null> {
    return confirmSettingsSave(options.confirm, { ...dialog, risk })
  }

  function providerLabel(provider: string) {
    return resolveProviderLabel(provider, providerMap.value)
  }

  function createStorageCardView(storage: StorageCard) {
    const vendorLogo = storageVendorLogo(storage)
    const vendorIcon = storageVendorIcon(storage)
    const providerName = providerLabel(storage.provider)
    const storageStatusClass = resolveStatusClass(storage, providerMap.value)
    const metaItems = resolveStorageMetaItems(storage)
    const storageStatusText = resolveStatusText(storage, providerMap.value)
    const subtitle = resolveStorageSubtitle(storage)

    return {
      storage,
      vendorLogo,
      vendorIcon,
      avatarLabel: vendorLogo || vendorIcon ? undefined : providerName.slice(0, 2).toUpperCase(),
      providerLabel: providerName,
      subtitle,
      statusText: storageStatusText,
      statusClass: storageStatusClass,
      statusSeverity: storageStatusSeverity(storageStatusClass),
      metaItems,
      supportsTest: providerSupportsTest(storage.provider, providerMap.value),
      canTest: canTestProfile(storage, providerMap.value),
      canApply: canApplyProfile(storage, providerMap.value),
    }
  }

  function resolveStorageTarget(target: StorageProfileCommandTarget) {
    const storage = findStorageByTarget(storages.value, target)
    if (!storage) {
      options.feedback.staleProfile()
      return null
    }
    return storage
  }

  return {
    loading,
    loadFailed,
    togglingId,
    applyingId,
    testingId,
    providers,
    providerMap,
    creatableProviders,
    storages,
    cards: {
      items: cardViews,
    },
    loadData,
    refreshData,
    saveProfile,
    applyStorage,
    toggleEnabled,
    testStorage,
    requestRemoveProfile,
    fetchBucketDomains,
  }
}

function findStorageByTarget(storages: StorageCard[], target: StorageProfileCommandTarget) {
  return storages.find((storage) => storage.id === target.id)
}
