import { computed, ref } from 'vue'
import type { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import {
  createAdminStorageResource,
  type AdminStorageResource,
  type SaveStorageProfileInput,
  type StorageProfileCommandResult,
} from './storageResource'
import { storageVendorIcon, storageVendorLogo } from './storageMeta'
import {
  canApply as canApplyProfile,
  canTest as canTestProfile,
  normalizeProfile,
  providerLabel as resolveProviderLabel,
  providerSupportsTest,
  statusClass as resolveStatusClass,
  statusText as resolveStatusText,
  storageMetaItems as resolveStorageMetaItems,
  storageStatusSeverity,
  storageSubtitle as resolveStorageSubtitle,
  toProviderMetaMap,
  type StorageCard,
  type StorageProviderMeta,
} from './storageHelpers'
import type { AdminStorageFeedback } from './useAdminStorageFeedback'

type RunAsyncAction = ReturnType<typeof useAsyncAction>['runAsyncAction']

export type AdminStorageWorkspaceOptions = {
  feedback: AdminStorageFeedback
  confirm: AdminConfirmPort
  runAsyncAction: RunAsyncAction
  resource?: AdminStorageResource
}

export type StorageProfileCommandTarget = Pick<StorageCard, 'id' | 'name'>

type DeleteStorageProfileCommandOptions = {
  afterDeleted?: (id: string) => void
}

export function createAdminStorageWorkspace(options: AdminStorageWorkspaceOptions) {
  const resource = options.resource ?? createAdminStorageResource()
  const loading = ref(false)
  const applyingId = ref<string | null>(null)
  const testingId = ref<string | null>(null)
  const providers = ref<StorageProviderMeta[]>([])
  const storages = ref<StorageCard[]>([])
  const searchKeyword = ref('')
  const providerMap = computed(() => toProviderMetaMap(providers.value))
  const creatableProviders = computed(() =>
    providers.value.filter((provider) => provider.create_allowed),
  )

  const cardViews = computed(() => storages.value.map(createStorageCardView))
  const hasStorageSearch = computed(() => Boolean(searchKeyword.value.trim()))
  const filteredStorageCards = computed(() => {
    const keyword = searchKeyword.value.trim().toLowerCase()
    if (!keyword) return cardViews.value
    return cardViews.value.filter((card) => card.searchableText.includes(keyword))
  })
  const currentStorage = computed(
    () => cardViews.value.find((card) => card.storage.isCurrent)?.storage || null,
  )
  const configuredStorageCount = computed(
    () => cardViews.value.filter((card) => card.statusClass !== 'is-empty').length,
  )
  const storageCountLabel = computed(() =>
    hasStorageSearch.value
      ? `${filteredStorageCards.value.length} 匹配存储`
      : `${storages.value.length} 存储`,
  )
  const emptyStorageMessage = computed(() =>
    hasStorageSearch.value ? '无匹配存储' : '暂无存储配置',
  )

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
        error: options.feedback.loadProfilesFailed,
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

  async function saveProfile(input: SaveStorageProfileInput): Promise<StorageProfileCommandResult> {
    const response = await resource.saveProfile(input)
    await refreshAfterCommand()
    return response
  }

  async function applyProfile(id: string): Promise<StorageProfileCommandResult> {
    const response = await resource.applyProfile(id)
    await refreshAfterCommand()
    return response
  }

  function testProfile(id: string) {
    return resource.testProfile(id)
  }

  async function deleteProfile(id: string) {
    const response = await resource.deleteProfile(id)
    await refreshAfterCommand()
    return response
  }

  async function applyStorage(target: StorageProfileCommandTarget) {
    const storage = resolveStorageTarget(target)
    if (!storage) return

    await options.runAsyncAction(
      async () => {
        const response = await applyProfile(storage.id)
        options.feedback.applyCurrentSucceeded(response.message)
      },
      {
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

  async function testStorage(target: StorageProfileCommandTarget) {
    const storage = resolveStorageTarget(target)
    if (!storage) return

    await options.runAsyncAction(
      async () => {
        const response = await resource.testProfile(storage.id)
        options.feedback.testConnectionSucceeded(response.message)
      },
      {
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

  function requestRemoveProfile(
    target: StorageProfileCommandTarget,
    commandOptions: DeleteStorageProfileCommandOptions = {},
  ) {
    const storage = resolveStorageTarget(target)
    if (!storage) return

    options.confirm.request({
      title: '确认删除存储',
      message: `确定删除存储「${storage.name}」吗？此操作不可恢复。`,
      confirmLabel: '确定删除',
      intent: 'danger',
      onConfirm: async () => {
        try {
          const response = await resource.deleteProfile(storage.id)
          options.feedback.profileDeleted(response.message)
          commandOptions.afterDeleted?.(storage.id)
          await refreshAfterCommand()
        } catch (error) {
          options.feedback.deleteProfileFailed(error)
        }
      },
    })
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
      searchableText: [
        storage.name,
        providerName,
        subtitle,
        storageStatusText,
        ...metaItems.flatMap((item) => [item.label, item.value]),
      ]
        .join(' ')
        .toLowerCase(),
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
    applyingId,
    testingId,
    providers,
    providerMap,
    creatableProviders,
    storages,
    cards: {
      items: filteredStorageCards,
      searchKeyword,
      isEmpty: computed(() => filteredStorageCards.value.length === 0),
      isLoading: computed(() => loading.value && storages.value.length === 0),
      hasSearch: hasStorageSearch,
      storageCountLabel,
      emptyMessage: emptyStorageMessage,
      currentStorage,
      configuredStorageCount,
      totalStorageCount: computed(() => storages.value.length),
    },
    loadData,
    refreshData,
    saveProfile,
    applyProfile,
    testProfile,
    deleteProfile,
    applyStorage,
    testStorage,
    requestRemoveProfile,
    fetchBucketDomains,
  }
}

function findStorageByTarget(storages: StorageCard[], target: StorageProfileCommandTarget) {
  return storages.find((storage) => storage.id === target.id)
}

export type AdminStorageWorkspace = ReturnType<typeof createAdminStorageWorkspace>
