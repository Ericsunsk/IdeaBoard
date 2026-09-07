import { computed, ref } from 'vue'
import type { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminProviderConfigFeedback } from './useAdminProviderConfigFeedback'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import {
  countUniqueAccountModels,
  createAccountModelRows,
  createProviderEndpointCards,
  createProviderEndpointView,
} from './providerConfigCards'
import { FALLBACK_ADMIN_ACCOUNT_LIMITS } from './providerConfigLimits'
import {
  ADMIN_PROVIDER_ENDPOINT_API_TYPE_OPTIONS,
  DEFAULT_ADMIN_PROVIDER_ENDPOINT_API_TYPE,
  createAccountCreatePayload,
  createAccountUpdatePayload,
  getAdminProviderEndpointApiTypeLabel,
  isModellessProviderEndpoint,
  normalizeModelKey,
} from './providerConfigPayload'
import type { ProviderEndpointProtocol } from '../../../shared/api/generated-query/model'
import {
  createAdminProviderConfigResource,
  type AdminProviderConfigResource,
} from './providerConfigResource'
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
  resource?: AdminProviderConfigResource | undefined
}

export type SaveProviderEndpointCommand = {
  id?: number | undefined
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
  modelCapabilities: ProviderAccountForm['modelCapabilities']
  secret: string
  afterSaved?: () => void
}

export type DeleteProviderAccountCommand = {
  afterDeleted?: () => void
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
const DEFAULT_PROVIDER_API_TYPE =
  ADMIN_PROVIDER_ENDPOINT_API_TYPE_OPTIONS[0]?.value ?? DEFAULT_ADMIN_PROVIDER_ENDPOINT_API_TYPE

export function createAdminProviderConfigWorkspace(options: AdminProviderConfigWorkspaceOptions) {
  const resource = options.resource ?? createAdminProviderConfigResource()
  const loading = ref(false)
  const providerEndpointSaveLoading = ref(false)
  const accountSaveLoading = ref(false)
  const detectModelsLoading = ref(false)
  const modelStatusChangingKeys = ref<ReadonlySet<string>>(new Set())
  const providerEndpoints = ref<ProviderEndpointRecord[]>([])
  const protocolRegistry = ref<ProviderEndpointProtocol[]>([])
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
  const accountRunningLabel = computed(() => formatAccountRunningLabel(accounts.value))
  const activeModelCount = computed(() => countUniqueAccountModels(accounts.value))

  async function loadAccountLimits() {
    try {
      accountLimits.value = await resource.fetchAccountLimits()
    } catch (error) {
      accountLimits.value = FALLBACK_ADMIN_ACCOUNT_LIMITS
      options.feedback.accountLimitsFallback(error)
      console.warn('[provider-config] failed to load account limits, using fallback', error)
    }
  }

  async function loadAccounts() {
    const [nextAccounts] = await Promise.all([resource.fetchAccounts(), loadAccountLimits()])
    accounts.value = nextAccounts.filter((account) => account.protocol === 'model-api')
  }

  async function refreshProviderData() {
    const registryPromise = resource.fetchProviderEndpointRegistry?.()
    const [nextEndpoints, nextRegistry] = await Promise.all([
      resource.fetchProviderEndpoints(),
      registryPromise ?? Promise.resolve([]),
    ])
    providerEndpoints.value = nextEndpoints
    protocolRegistry.value = nextRegistry
    await loadAccounts()
  }

  async function loadData() {
    await options.runAsyncAction(refreshProviderData, {
      loading,
      feedback: {
        error: options.feedback.loadFailed,
      },
    })
  }

  async function refreshAfterCommand() {
    try {
      await refreshProviderData()
    } catch (error) {
      options.feedback.loadFailed(error)
    }
  }

  async function saveProviderEndpoint(command: SaveProviderEndpointCommand) {
    const saved = await options.runAsyncAction(
      async () => {
        await resource.saveProviderEndpoint({
          id: command.id,
          data: { ...command.data },
        })
        return true
      },
      {
        loading: providerEndpointSaveLoading,
        feedback: {
          success: () => {
            options.feedback.providerEndpointSaved(command.isEditing)
            command.afterSaved?.()
          },
          error: options.feedback.providerEndpointSaveFailed,
        },
      },
    )
    if (saved) await refreshAfterCommand()
  }

  async function saveAccount(command: SaveProviderAccountCommand) {
    const saved = await options.runAsyncAction(
      async () => {
        const existingAccount = command.isEditing
          ? accounts.value.find((account) => account.id === command.accountId)
          : undefined

        if (command.isEditing) {
          await resource.updateAccountRecord(
            command.accountId,
            createAccountUpdatePayload({
              form: command.form,
              providerEndpoint: command.selectedProviderEndpoint,
              existingAccount,
              modelValues: command.modelValues,
              modelCapabilities: command.modelCapabilities,
              secret: command.secret,
            }),
          )
        } else {
          await resource.createAccountRecord(
            createAccountCreatePayload({
              form: command.form,
              providerEndpoint: command.selectedProviderEndpoint,
              modelValues: command.modelValues,
              modelCapabilities: command.modelCapabilities,
              secret: command.secret,
            }),
          )
        }
        return true
      },
      {
        loading: accountSaveLoading,
        feedback: {
          success: () => {
            options.feedback.accountSaved(command.isEditing)
            command.afterSaved?.()
          },
          error: options.feedback.accountSaveFailed,
        },
      },
    )
    if (saved) await refreshAfterCommand()
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
          await resource.deleteProviderEndpoint(providerEndpoint.id)
          options.feedback.providerEndpointDeleted()
          await loadData()
        } catch (error) {
          options.feedback.providerEndpointDeleteFailed(error)
        }
      },
    })
  }

  function deleteAccount(
    target: ProviderAccountCommandTarget,
    command: DeleteProviderAccountCommand = {},
  ) {
    const account = resolveAccountTarget(target)
    if (!account) return

    options.confirm.request({
      title: '确认删除 APIKey',
      message: '确定删除该 APIKey 吗？关联的执行中任务会由后端统一结束。',
      confirmLabel: '确定',
      intent: 'danger',
      onConfirm: async () => {
        try {
          await resource.deleteAccountRecord(account.id)
          command.afterDeleted?.()
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
    if (detectModelsLoading.value || isModellessProviderEndpoint(command.providerEndpoint))
      return result

    const apiType = command.providerEndpoint?.apiType || DEFAULT_PROVIDER_API_TYPE
    const sessionKey = `${apiType}::${baseURL}::${secret}`
    if (command.sessionCache.has(sessionKey)) return result

    let nextDetectedValues = command.detectedValues
    let nextSessionCache = command.sessionCache

    await options.runAsyncAction(
      async () => {
        const detectedModels = await resource.detectProviderEndpointModels({
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
        nextSessionCache = new Set(command.sessionCache)
        nextSessionCache.add(sessionKey)
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
    const pending = createModelStatusPendingRef(commandTarget, targetModel.name)

    await options.runAsyncAction(
      async () => {
        const nextAccount = await resource.patchAccountModelRecord(account.id, {
          name: targetModel.name,
          enabled: nextEnabled,
        })
        accounts.value = accounts.value.map((item) => (item.id === account.id ? nextAccount : item))
      },
      {
        loading: pending,
        feedback: {
          success: () => options.feedback.modelStatusChanged(nextEnabled),
          error: options.feedback.modelStatusChangeFailed,
        },
      },
    )
  }

  function isAccountModelStatusChanging(
    commandTarget: ProviderAccountCommandTarget,
    model: string,
  ) {
    return modelStatusChangingKeys.value.has(createModelStatusPendingKey(commandTarget, model))
  }

  function createModelStatusPendingRef(commandTarget: ProviderAccountCommandTarget, model: string) {
    const key = createModelStatusPendingKey(commandTarget, model)
    return computed({
      get: () => modelStatusChangingKeys.value.has(key),
      set: (isPending) => {
        const nextKeys = new Set(modelStatusChangingKeys.value)
        if (isPending) nextKeys.add(key)
        else nextKeys.delete(key)
        modelStatusChangingKeys.value = nextKeys
      },
    })
  }

  function createModelStatusPendingKey(commandTarget: ProviderAccountCommandTarget, model: string) {
    return `${commandTarget.id}:${normalizeModelKey(model)}`
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
    return (
      providerEndpoints.value.find(
        (providerEndpoint) => providerEndpoint.id === providerEndpointId,
      ) || null
    )
  }

  function getProviderEndpointAccounts(providerEndpointId: number) {
    return accounts.value.filter((account) => account.providerEndpointId === providerEndpointId)
  }

  async function toggleProviderEndpointStatus(
    target: ProviderEndpointCommandTarget | { id: number },
  ) {
    const targetPayload: ProviderEndpointCommandTarget =
      'kind' in target ? target : { kind: 'providerEndpoint', id: target.id }
    const providerEndpoint = resolveProviderEndpointTarget(targetPayload)
    if (!providerEndpoint) return

    const nextStatus = providerEndpoint.status === 1 ? 0 : 1
    const saved = await options.runAsyncAction(
      async () => {
        await resource.setProviderEndpointStatus({
          id: providerEndpoint.id,
          status: nextStatus,
        })
        return true
      },
      {
        feedback: {
          error: options.feedback.providerEndpointSaveFailed,
        },
      },
    )
    if (saved) await refreshAfterCommand()
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
    protocolRegistry,
    loadData,
    saveProviderEndpoint,
    saveAccount,
    deleteProviderEndpoint,
    deleteAccount,
    detectModels,
    toggleProviderEndpointStatus,
    toggleAccountModelStatus,
    isAccountModelStatusChanging,
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

function formatAccountRunningLabel(accounts: readonly ProviderAccountRecord[]): string {
  if (accounts.length === 0) return '0 APIKey'
  const runningCount = accounts.filter(
    (account) => account.status === 1 && account.failCount < 5,
  ).length
  return `${runningCount}/${accounts.length} 运行中`
}

function getAccountStatusDotClass(account: ProviderAccountView) {
  const allDisabled = account.models.length > 0 && account.models.every((m) => !m.enabled)
  if (!account.isEnabled || allDisabled || account.isDegraded) return 'bg-admin-danger'
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

export type AdminProviderConfigWorkspace = ReturnType<typeof createAdminProviderConfigWorkspace>
