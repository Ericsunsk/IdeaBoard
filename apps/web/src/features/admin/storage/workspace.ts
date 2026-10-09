import { computed, ref } from 'vue'
import type { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import { useOptimisticToggle } from '../shared/composables/useOptimisticToggle'
import { requireDestructiveReason } from '../shared/lib/confirmSettingsSave'
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
  storageMetaItems as resolveStorageMetaItems,
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
  // 行开关乐观收口到共享 useOptimisticToggle：intent 同 tick 翻转，
  // pending 按 id 独立；成功后等回填 reconcile，失败回滚。toast 语义不变。
  const toggle = useOptimisticToggle<string>()
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
      feedback: {
        error: (error) => {
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

  // Workspace owns storage mutation orchestration and refresh-after-command.
  // Save/apply are routine config writes (backend ReasonOptional): no
  // confirm dialog, the audit records an empty reason. The profile editor
  // only supplies form payloads through saveProfile; apply/test/delete/
  // enable orchestration lives here. Delete stays a destructive confirm
  // with a required reason.
  async function saveProfile(
    input: SaveStorageProfileInput,
  ): Promise<StorageProfileCommandResult | null> {
    const response = await resource.saveProfile(input, { operatorReason: '' })
    await refreshAfterCommand()
    return response
  }

  async function applyStorage(target: StorageProfileCommandTarget) {
    const storage = resolveStorageTarget(target)
    if (!storage) return

    await options.runAsyncAction(
      async () => {
        const response = await resource.applyProfile(storage.id, { operatorReason: '' })
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

    // Routine config write (backend ReasonOptional): no confirm dialog,
    // the audit records an empty reason. 乐观翻转与 pending/回滚收口到共享
    // composable，toast 仍走 saveDraft 通道。
    await toggle.toggle(
      storage.id,
      enabled,
      async (key, next) => {
        const response = await resource.saveProfile(
          {
            id: key,
            payload: buildPayload({ ...storage, enabled: next }, providerMap.value),
          },
          { operatorReason: '' },
        )
        const savedStorage = normalizeProfile(response.profile, providerMap.value)
        storages.value = storages.value.map((candidate) =>
          candidate.id === key ? savedStorage : candidate,
        )
        toggle.reconcile(key, response.profile.enabled)
        await refreshAfterCommand()
        const current = storages.value.find((candidate) => candidate.id === key)
        if (current) toggle.reconcile(key, current.enabled)
        if (response) options.feedback.saveDraftSucceeded(response.message)
      },
      {
        onFailure: (_, __, error) => {
          options.feedback.saveDraftFailed(error)
        },
      },
    )
  }

  async function testStorage(target: StorageProfileCommandTarget) {
    const storage = resolveStorageTarget(target)
    if (!storage) return

    await options.runAsyncAction(
      async () => {
        // Connectivity probe is a POST read-guard (backend non-Mutation):
        // no confirm dialog, the audit records an empty reason (routine
        // 空 reason，与 mail/搜索探针对齐）。
        const response = await resource.testProfile(storage.id, {
          operatorReason: '',
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

  function providerLabel(provider: string) {
    return resolveProviderLabel(provider, providerMap.value)
  }

  function createStorageCardView(storage: StorageCard) {
    const vendorLogo = storageVendorLogo(storage)
    const vendorIcon = storageVendorIcon(storage)
    const providerName = providerLabel(storage.provider)
    const metaItems = resolveStorageMetaItems(storage)
    const subtitle = resolveStorageSubtitle(storage)

    return {
      storage,
      vendorLogo,
      vendorIcon,
      avatarLabel: vendorLogo || vendorIcon ? undefined : providerName.slice(0, 2).toUpperCase(),
      providerLabel: providerName,
      subtitle,
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
    toggle,
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
