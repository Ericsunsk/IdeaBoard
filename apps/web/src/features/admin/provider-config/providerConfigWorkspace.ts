import { computed, ref } from 'vue'
import type { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminProviderConfigFeedback } from '../feedback/useAdminFeedback'
import {
  createAdminAccountRecord,
  deleteAdminAccountRecord,
  deleteAdminChannel,
  detectAdminChannelModels,
  FALLBACK_ADMIN_ACCOUNT_LIMITS,
  fetchAdminAccountLimits,
  fetchAllAdminAccounts,
  fetchAdminChannels,
  saveAdminChannel,
  updateAdminAccountRecord,
} from './providerConfigApi'
import {
  countUniqueAccountModels,
  createAccountModelRows,
  createProviderChannelCards,
  createProviderChannelView,
} from './providerConfigCards'
import {
  ADMIN_CHANNEL_API_TYPE_OPTIONS,
  createAccountCreatePayload,
  createAccountUpdatePayload,
  createAccountUpdatePayloadFromAccount,
  getAdminChannelApiTypeLabel,
  normalizeModelKey,
} from './providerConfigPayload'
import {
  MAX_DETECTED_MODELS,
  mergeDetectedModelValues,
  normalizeDetectedModelValues,
} from './providerModelOptions'
import type {
  AdminProviderAccountLimits,
  ProviderAccountCommandTarget,
  ProviderAccountForm,
  ProviderAccountRecord,
  ProviderAccountView,
  ProviderChannelCommandTarget,
  ProviderChannelFormData,
  ProviderChannelRecord,
} from './providerConfigTypes'

export type AdminProviderConfirmPort = {
  danger: (options: {
    title: string
    content: string
    acceptLabel: string
    onConfirm: () => void | Promise<void>
  }) => void
}

type RunAsyncAction = ReturnType<typeof useAsyncAction>['runAsyncAction']

export type AdminProviderConfigWorkspaceOptions = {
  feedback: AdminProviderConfigFeedback
  confirm: AdminProviderConfirmPort
  runAsyncAction: RunAsyncAction
}

export type SaveProviderChannelCommand = {
  id?: number
  data: ProviderChannelFormData
  isEditing: boolean
  afterSaved?: () => void
}

export type SaveProviderAccountCommand = {
  isEditing: boolean
  accountId: number
  form: ProviderAccountForm
  selectedChannel: ProviderChannelRecord | null
  modelValues: string[]
  secret: string
  afterSaved?: () => void
}

export type DetectProviderModelsCommand = {
  channel: ProviderChannelRecord | null
  secret: string
  detectedValues: string[]
  sessionCache: Set<string>
  hasShownApiKeyHint: boolean
}

export type DetectProviderModelsResult = {
  detectedValues: string[]
  sessionCache: Set<string>
  hasShownApiKeyHint: boolean
}

const activeProgressPt = { value: { class: '!bg-admin-success' } } as const
const mutedProgressPt = { value: { class: '!bg-admin-text-3' } } as const
const DEFAULT_PROVIDER_API_TYPE = ADMIN_CHANNEL_API_TYPE_OPTIONS[0].value

export function createAdminProviderConfigWorkspace(
  options: AdminProviderConfigWorkspaceOptions,
) {
  const loading = ref(false)
  const channelSaveLoading = ref(false)
  const accountSaveLoading = ref(false)
  const detectModelsLoading = ref(false)
  const channels = ref<ProviderChannelRecord[]>([])
  const accounts = ref<ProviderAccountRecord[]>([])
  const accountLimits = ref<AdminProviderAccountLimits>(FALLBACK_ADMIN_ACCOUNT_LIMITS)
  const searchKeyword = ref('')

  const filteredChannels = computed(() => {
    const keyword = searchKeyword.value.trim().toLowerCase()
    if (!keyword) return channels.value
    return channels.value.filter((item) => item.name.toLowerCase().includes(keyword))
  })

  const cardItems = computed(() =>
    createProviderChannelCards(filteredChannels.value, accounts.value),
  )
  const hasChannelSearch = computed(() => Boolean(searchKeyword.value.trim()))
  const totalChannelsLabel = computed(() =>
    hasChannelSearch.value
      ? `${filteredChannels.value.length} 匹配渠道`
      : `${channels.value.length} 渠道`,
  )
  const emptyChannelMessage = computed(() =>
    hasChannelSearch.value ? '无匹配渠道商' : '暂无渠道商',
  )
  const totalAccountCount = computed(() => accounts.value.length)
  const runningAccountCount = computed(
    () => accounts.value.filter((account) => account.status === 1 && account.failCount < 5).length,
  )
  const accountRunningLabel = computed(() =>
    totalAccountCount.value > 0
      ? `${runningAccountCount.value}/${totalAccountCount.value} 运行中`
      : '0 APIKey',
  )
  const activeModelCount = computed(() => countUniqueAccountModels(accounts.value))

  async function loadAccountLimits() {
    try {
      accountLimits.value = await fetchAdminAccountLimits()
    } catch (error) {
      accountLimits.value = FALLBACK_ADMIN_ACCOUNT_LIMITS
      options.feedback.accountLimitsFallback(error)
      console.warn('[provider-config] failed to load account limits, using fallback', error)
    }
  }

  async function loadAccounts() {
    const [nextAccounts] = await Promise.all([fetchAllAdminAccounts(), loadAccountLimits()])
    accounts.value = nextAccounts
  }

  async function loadData() {
    await options.runAsyncAction(
      async () => {
        channels.value = await fetchAdminChannels()
        await loadAccounts()
      },
      {
        loading,
        feedback: {
          error: options.feedback.loadFailed,
        },
      },
    )
  }

  async function saveChannel(command: SaveProviderChannelCommand) {
    await options.runAsyncAction(
      async () => {
        await saveAdminChannel({
          id: command.id,
          data: { ...command.data },
        })
        options.feedback.channelSaved(command.isEditing)
        command.afterSaved?.()
        await loadData()
      },
      {
        loading: channelSaveLoading,
        feedback: {
          error: options.feedback.channelSaveFailed,
        },
      },
    )
  }

  async function saveAccount(command: SaveProviderAccountCommand) {
    await options.runAsyncAction(
      async () => {
        const existingAccount = command.isEditing
          ? accounts.value.find((account) => account.id === command.accountId)
          : undefined

        if (command.isEditing) {
          await updateAdminAccountRecord(
            command.accountId,
            createAccountUpdatePayload({
              form: command.form,
              channel: command.selectedChannel,
              existingAccount,
              modelValues: command.modelValues,
              secret: command.secret,
            }),
          )
          options.feedback.accountSaved(true)
        } else {
          await createAdminAccountRecord(
            createAccountCreatePayload({
              form: command.form,
              channel: command.selectedChannel,
              modelValues: command.modelValues,
              secret: command.secret,
            }),
          )
          options.feedback.accountSaved(false)
        }

        command.afterSaved?.()
        await loadData()
      },
      {
        loading: accountSaveLoading,
        feedback: {
          error: options.feedback.accountSaveFailed,
        },
      },
    )
  }

  function deleteChannel(target: ProviderChannelCommandTarget) {
    const channel = resolveChannelTarget(target)
    if (!channel) return

    options.confirm.danger({
      title: '确认删除',
      content: `确定删除渠道商「${channel.name}」吗？此操作不可恢复。`,
      acceptLabel: '确定',
      onConfirm: async () => {
        try {
          await deleteAdminChannel(channel.id)
          options.feedback.channelDeleted()
          await loadData()
        } catch (error) {
          options.feedback.channelDeleteFailed(error)
        }
      },
    })
  }

  function deleteAccount(target: ProviderAccountCommandTarget) {
    const account = resolveAccountTarget(target)
    if (!account) return

    options.confirm.danger({
      title: '确认删除 APIKey',
      content: '确定删除该 APIKey 吗？关联的执行中任务会由后端统一结束。',
      acceptLabel: '确定',
      onConfirm: async () => {
        try {
          await deleteAdminAccountRecord(account.id)
          options.feedback.accountDeleted()
          await loadData()
        } catch (error) {
          options.feedback.accountDeleteFailed(error)
        }
      },
    })
  }

  async function detectModels(
    command: DetectProviderModelsCommand,
  ): Promise<DetectProviderModelsResult> {
    const baseURL = (command.channel?.baseUrl || '').trim()
    const secret = command.secret.trim()
    const result = {
      detectedValues: command.detectedValues,
      sessionCache: command.sessionCache,
      hasShownApiKeyHint: command.hasShownApiKeyHint,
    }

    if (!baseURL) return result
    if (!secret) {
      if (!command.hasShownApiKeyHint) {
        options.feedback.missingApiKeyForModelDetection()
      }
      return {
        ...result,
        hasShownApiKeyHint: true,
      }
    }
    if (detectModelsLoading.value) return result

    const apiType = command.channel?.apiType || DEFAULT_PROVIDER_API_TYPE
    const sessionKey = `${apiType}::${baseURL}::${secret}`
    if (command.sessionCache.has(sessionKey)) return result

    const nextSessionCache = new Set(command.sessionCache)
    nextSessionCache.add(sessionKey)
    let nextDetectedValues = command.detectedValues

    await options.runAsyncAction(
      async () => {
        const detectedModels = await detectAdminChannelModels({
          baseUrl: baseURL,
          apiKey: secret,
          apiType,
        })
        const models = normalizeDetectedModelValues(detectedModels, MAX_DETECTED_MODELS)

        if (models.length > 0) {
          nextDetectedValues = mergeDetectedModelValues(
            command.detectedValues,
            models,
            MAX_DETECTED_MODELS,
          )
        }
      },
      {
        loading: detectModelsLoading,
        feedback: {
          error: options.feedback.modelDetectionFailed,
        },
      },
    )

    return {
      detectedValues: nextDetectedValues,
      sessionCache: nextSessionCache,
      hasShownApiKeyHint: command.hasShownApiKeyHint,
    }
  }

  async function toggleAccountModelStatus(
    commandTarget: ProviderAccountCommandTarget,
    model: string,
  ) {
    const account = resolveAccountTarget(commandTarget)
    if (!account) return

    const modelKey = normalizeModelKey(model)
    const targetModel = account.models.find((item) => normalizeModelKey(item.name) === modelKey)
    if (!targetModel) {
      options.feedback.staleModel()
      return
    }
    const nextEnabled = !targetModel.enabled
    const nextModels = account.models.map((item) =>
      normalizeModelKey(item.name) === modelKey ? { ...item, enabled: nextEnabled } : item,
    )

    await options.runAsyncAction(
      async () => {
        const updatedAt = new Date().toISOString()
        const nextAccount: ProviderAccountRecord = {
          ...account,
          models: nextModels,
          updatedAt,
        }

        await updateAdminAccountRecord(
          account.id,
          createAccountUpdatePayloadFromAccount(nextAccount),
        )
        accounts.value = accounts.value.map((item) => (item.id === account.id ? nextAccount : item))
        options.feedback.modelStatusChanged(nextEnabled)
      },
      {
        feedback: {
          error: options.feedback.modelStatusChangeFailed,
        },
      },
    )
  }

  function resolveChannelTarget(target: ProviderChannelCommandTarget) {
    const channel = findChannelByTarget(channels.value, target)
    if (!channel) {
      options.feedback.staleChannel()
      return null
    }
    return channel
  }

  function resolveAccountTarget(target: ProviderAccountCommandTarget) {
    const account = findAccountByTarget(accounts.value, target)
    if (!account) {
      options.feedback.staleAccount()
      return null
    }
    return account
  }

  function findChannelById(channelId: number) {
    return channels.value.find((channel) => channel.id === channelId) || null
  }

  return {
    cards: {
      items: cardItems,
      searchKeyword,
      isEmpty: computed(() => filteredChannels.value.length === 0),
      isLoading: computed(() => loading.value && channels.value.length === 0),
      hasSearch: hasChannelSearch,
      totalChannelsLabel,
      emptyMessage: emptyChannelMessage,
      accountRunningLabel,
      activeModelCount,
    },
    channelSaveLoading,
    accountSaveLoading,
    detectModelsLoading,
    accountLimits,
    apiTypeOptions: ADMIN_CHANNEL_API_TYPE_OPTIONS,
    loadData,
    saveChannel,
    saveAccount,
    deleteChannel,
    deleteAccount,
    detectModels,
    toggleAccountModelStatus,
    resolveChannelTarget,
    resolveAccountTarget,
    findChannelById,
    getApiTypeLabel: getAdminChannelApiTypeLabel,
    getAccountModelRows: createAccountModelRows,
    getChannelView: createProviderChannelView,
    getAccountStatusDotClass,
    getAccountProgressPt,
  }
}

function getAccountStatusDotClass(account: ProviderAccountView) {
  if (!account.isEnabled) return 'bg-admin-warning'
  if (account.isDegraded) return 'bg-admin-danger'
  return 'bg-admin-success'
}

function getAccountProgressPt(account: ProviderAccountView) {
  return account.isEnabled ? activeProgressPt : mutedProgressPt
}

function findChannelByTarget(
  channels: ProviderChannelRecord[],
  target: ProviderChannelCommandTarget,
) {
  return channels.find((channel) => channel.id === target.id)
}

function findAccountByTarget(
  accounts: ProviderAccountRecord[],
  target: ProviderAccountCommandTarget,
) {
  return accounts.find((account) => account.id === target.id)
}

export type AdminProviderConfigWorkspace = ReturnType<
  typeof createAdminProviderConfigWorkspace
>
