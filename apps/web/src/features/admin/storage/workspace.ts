import { computed, ref } from 'vue'
import type { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import { readConfirmReason } from '../shared/composables/useAdminConfirm'
import { confirmSettingsSave } from '../shared/lib/confirmSettingsSave'
import {
  createAdminStorageResource,
  type AdminStorageResource,
  type SaveStorageProfileInput,
  type StorageProfileCommandResult,
} from './resource'
import { storageVendorIcon, storageVendorLogo } from './meta'
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
  const applyingId = ref<string | null>(null)
  const testingId = ref<string | null>(null)
  const applyingPending = ref(false)
  const testingPending = ref(false)
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

  async function applyProfile(id: string): Promise<StorageProfileCommandResult | null> {
    const operatorReason = await confirmStorageWrite({
      title: '设为当前存储',
      message: '确定将该存储设为当前存储吗？该操作将记录操作人与原因。',
      confirmLabel: '设为当前',
    })
    if (!operatorReason) return null
    const response = await resource.applyProfile(id, { operatorReason })
    await refreshAfterCommand()
    return response
  }

  function testProfile(id: string) {
    return testStorageById(id)
  }

  async function testStorageById(id: string) {
    const operatorReason = await confirmStorageWrite({
      title: '测试存储连接',
      message: '将使用已保存的配置发起一次真实连接测试。该操作将记录操作人与原因。',
      confirmLabel: '测试连接',
    })
    if (!operatorReason) return null
    return resource.testProfile(id, { operatorReason })
  }

  async function deleteProfile(id: string) {
    const operatorReason = await confirmStorageWrite({
      title: '确认删除存储',
      message: '确定删除该存储配置吗？此操作不可恢复，且将记录操作人与原因。',
      confirmLabel: '确定删除',
    })
    if (!operatorReason) return null
    const response = await resource.deleteProfile(id, { operatorReason })
    await refreshAfterCommand()
    return response
  }

  async function applyStorage(target: StorageProfileCommandTarget) {
    const storage = resolveStorageTarget(target)
    if (!storage) return

    await options.runAsyncAction(
      async () => {
        const response = await applyProfile(storage.id)
        if (!response) return
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

  async function testStorage(target: StorageProfileCommandTarget) {
    const storage = resolveStorageTarget(target)
    if (!storage) return

    await options.runAsyncAction(
      async () => {
        const response = await testStorageById(storage.id)
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
    const operatorReason = (options.confirm.readReason?.() ?? readConfirmReason()).trim()
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
  // silently (null) so no failure feedback fires.
  function confirmStorageWrite(dialog: {
    title: string
    message: string
    confirmLabel: string
  }): Promise<string | null> {
    return confirmSettingsSave(options.confirm, dialog)
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
