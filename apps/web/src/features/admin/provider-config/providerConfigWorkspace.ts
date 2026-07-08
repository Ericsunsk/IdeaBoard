import { computed, ref } from 'vue'
import type { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminProviderConfigFeedback } from '../feedback/useAdminFeedback'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import {
  createAdminAccountRecord,
  deleteAdminAccountRecord,
  deleteAdminProviderEndpoint,
  detectAdminProviderEndpointModels,
  FALLBACK_ADMIN_ACCOUNT_LIMITS,
  fetchAdminAccountLimits,
  fetchAllAdminAccounts,
  fetchAdminProviderEndpoints,
  patchAdminAccountModelRecord,
  saveAdminProviderEndpoint,
  updateAdminAccountRecord,
} from './providerConfigApi'
import {
  countUniqueAccountModels,
  createAccountModelRows,
  createProviderEndpointCards,
  createProviderEndpointView,
} from './providerConfigCards'
import {
  ADMIN_PROVIDER_ENDPOINT_API_TYPE_OPTIONS,
  createAccountCreatePayload,
  createAccountUpdatePayload,
  getAdminProviderEndpointApiTypeLabel,
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
  ProviderEndpointCommandTarget,
  ProviderEndpointFormData,
  ProviderEndpointRecord,
} from './providerConfigTypes'

type RunAsyncAction = ReturnType<typeof useAsyncAction>['runAsyncAction']

export type AdminProviderConfigWorkspaceOptions = {
  feedback: AdminProviderConfigFeedback
  confirm: AdminConfirmPort
  runAsyncAction: RunAsyncAction
}

export type SaveProviderEndpointCommand = {
  id?: number
  data: ProviderEndpointFormData
  isEditing: boolean
  afterSaved?: () => void
}

export type SaveProviderAccountCommand = {
  isEditing: boolean
  accountId: number
  form: ProviderAccountForm
  selectedProviderEndpoint: ProviderEndpointRecord | null
  modelValues: string[]
  secret: string
  afterSaved?: () => void
}

export type DetectProviderModelsCommand = {
  providerEndpoint: ProviderEndpointRecord | null
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
const DEFAULT_PROVIDER_API_TYPE = ADMIN_PROVIDER_ENDPOINT_API_TYPE_OPTIONS[0].value

export function createAdminProviderConfigWorkspace(
  options: AdminProviderConfigWorkspaceOptions,
) {
  const loading = ref(false)
  const providerEndpointSaveLoading = ref(false)
  const accountSaveLoading = ref(false)
  const detectModelsLoading = ref(false)
  const providerEndpoints = ref<ProviderEndpointRecord[]>([])
  const accounts = ref<ProviderAccountRecord[]>([])
  const accountLimits = ref<AdminProviderAccountLimits>(FALLBACK_ADMIN_ACCOUNT_LIMITS)
  const searchKeyword = ref('')

  const filteredProviderEndpoints = computed(() => {
    const keyword = searchKeyword.value.trim().toLowerCase()
    if (!keyword) return providerEndpoints.value
    return providerEndpoints.value.filter((item) => item.name.toLowerCase().includes(keyword))
  })

  const cardItems = computed(() =>
    createProviderEndpointCards(filteredProviderEndpoints.value, accounts.value),
  )
  const hasProviderEndpointSearch = computed(() => Boolean(searchKeyword.value.trim()))
  const totalProviderEndpointsLabel = computed(() =>
    hasProviderEndpointSearch.value
      ? `${filteredProviderEndpoints.value.length} 匹配端点`
      : `${providerEndpoints.value.length} 端点`,
  )
  const emptyProviderEndpointMessage = computed(() =>
    hasProviderEndpointSearch.value ? '无匹配供应商端点' : '暂无供应商端点',
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
        providerEndpoints.value = await fetchAdminProviderEndpoints()
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

  async function saveProviderEndpoint(command: SaveProviderEndpointCommand) {
    await options.runAsyncAction(
      async () => {
        await saveAdminProviderEndpoint({
          id: command.id,
          data: { ...command.data },
        })
        options.feedback.providerEndpointSaved(command.isEditing)
        command.afterSaved?.()
        await loadData()
      },
      {
        loading: providerEndpointSaveLoading,
        feedback: {
          error: options.feedback.providerEndpointSaveFailed,
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
              providerEndpoint: command.selectedProviderEndpoint,
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
              providerEndpoint: command.selectedProviderEndpoint,
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

  function deleteProviderEndpoint(target: ProviderEndpointCommandTarget) {
    const providerEndpoint = resolveProviderEndpointTarget(target)
    if (!providerEndpoint) return

    options.confirm.request({
      title: '确认删除',
      message: `确定删除供应商端点「${providerEndpoint.name}」吗？此操作不可恢复。`,
      confirmLabel: '确定',
      intent: 'danger',
      onConfirm: async () => {
        try {
          await deleteAdminProviderEndpoint(providerEndpoint.id)
          options.feedback.providerEndpointDeleted()
          await loadData()
        } catch (error) {
          options.feedback.providerEndpointDeleteFailed(error)
        }
      },
    })
  }

  function deleteAccount(target: ProviderAccountCommandTarget) {
    const account = resolveAccountTarget(target)
    if (!account) return

    options.confirm.request({
      title: '确认删除 APIKey',
      message: '确定删除该 APIKey 吗？关联的执行中任务会由后端统一结束。',
      confirmLabel: '确定',
      intent: 'danger',
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
    const baseURL = (command.providerEndpoint?.baseUrl || '').trim()
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

    const apiType = command.providerEndpoint?.apiType || DEFAULT_PROVIDER_API_TYPE
    const sessionKey = `${apiType}::${baseURL}::${secret}`
    if (command.sessionCache.has(sessionKey)) return result

    const nextSessionCache = new Set(command.sessionCache)
    nextSessionCache.add(sessionKey)
    let nextDetectedValues = command.detectedValues

    await options.runAsyncAction(
      async () => {
        const detectedModels = await detectAdminProviderEndpointModels({
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

    await options.runAsyncAction(
      async () => {
        const nextAccount = await patchAdminAccountModelRecord(account.id, {
          name: targetModel.name,
          enabled: nextEnabled,
        })
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

  function resolveProviderEndpointTarget(target: ProviderEndpointCommandTarget) {
    const providerEndpoint = findProviderEndpointByTarget(providerEndpoints.value, target)
    if (!providerEndpoint) {
      options.feedback.staleProviderEndpoint()
      return null
    }
    return providerEndpoint
  }

  function resolveAccountTarget(target: ProviderAccountCommandTarget) {
    const account = findAccountByTarget(accounts.value, target)
    if (!account) {
      options.feedback.staleAccount()
      return null
    }
    return account
  }

  function findProviderEndpointById(providerEndpointId: number) {
    return providerEndpoints.value.find((providerEndpoint) => providerEndpoint.id === providerEndpointId) || null
  }

  function getProviderEndpointAccounts(providerEndpointId: number) {
    return accounts.value.filter((account) => account.providerEndpointId === providerEndpointId)
  }

  return {
    cards: {
      items: cardItems,
      searchKeyword,
      isEmpty: computed(() => filteredProviderEndpoints.value.length === 0),
      isLoading: computed(() => loading.value && providerEndpoints.value.length === 0),
      hasSearch: hasProviderEndpointSearch,
      totalProviderEndpointsLabel,
      emptyMessage: emptyProviderEndpointMessage,
      accountRunningLabel,
      activeModelCount,
    },
    providerEndpointSaveLoading,
    accountSaveLoading,
    detectModelsLoading,
    accountLimits,
    apiTypeOptions: ADMIN_PROVIDER_ENDPOINT_API_TYPE_OPTIONS,
    loadData,
    saveProviderEndpoint,
    saveAccount,
    deleteProviderEndpoint,
    deleteAccount,
    detectModels,
    toggleAccountModelStatus,
    resolveProviderEndpointTarget,
    resolveAccountTarget,
    findProviderEndpointById,
    getProviderEndpointAccounts,
    getApiTypeLabel: getAdminProviderEndpointApiTypeLabel,
    getAccountModelRows: createAccountModelRows,
    getProviderEndpointView: createProviderEndpointView,
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

function findProviderEndpointByTarget(
  providerEndpoints: ProviderEndpointRecord[],
  target: ProviderEndpointCommandTarget,
) {
  return providerEndpoints.find((providerEndpoint) => providerEndpoint.id === target.id)
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
