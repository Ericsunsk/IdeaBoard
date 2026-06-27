import { computed, ref } from 'vue'
import { useAsyncAction } from '../../../../shared/composables/useAsyncAction'
import {
  detectAdminChannelModels,
  type Channel,
} from '../../shared/queries/adminChannelActions'
import {
  createAdminAccountRecord,
  deleteAdminAccountRecord,
  FALLBACK_ADMIN_ACCOUNT_LIMITS,
  fetchAdminAccountLimits,
  fetchAllAdminAccounts,
  type AdminAccount,
  type AdminAccountLimits,
  type AdminAccountModel,
  type AdminAccountModelInput,
  type AdminAccountUpdateRequest,
  updateAdminAccountRecord,
} from '../../shared/queries/adminAccountActions'
import { DEFAULT_ADMIN_CHANNEL_API_TYPE } from './channelApiTypes'
import { extractApiErrorMessage } from './channelModelUtils'

const MAX_DETECTED_MODELS = 100

export type AccountModelRow = {
  name: string
  disabled: boolean
}

export function useChannelAccounts(options: {
  channels: Readonly<{ value: Channel[] }>
  message: {
    success: (content: string) => void
    error: (content: string) => void
    warning: (content: string) => void
  }
  dialog: {
    warning: (options: {
      title: string
      content: string
      onConfirm: () => void | Promise<void>
    }) => void
  }
  onAfterMutation: () => Promise<void> | void
}) {
  const { runAsyncAction } = useAsyncAction()
  const accounts = ref<AdminAccount[]>([])
  const showAccountModal = ref(false)
  const isEditAccount = ref(false)
  const accountSaveLoading = ref(false)
  const currentAccountId = ref(0)
  const selectedChannelForAccount = ref<Channel | null>(null)
  const accountLimits = ref<AdminAccountLimits>(FALLBACK_ADMIN_ACCOUNT_LIMITS)

  function createEmptyAccountForm(channelId = 0) {
    return {
      channel_id: channelId,
      model_values: [] as string[],
      secret: '',
      max_concurrent: accountLimits.value.max_concurrent.default,
      timeout: accountLimits.value.timeout_seconds.default,
    }
  }

  const accountForm = ref(createEmptyAccountForm())
  const detectModelsLoading = ref(false)
  const detectedModelValues = ref<string[]>([])
  const modelSuggestions = ref<string[]>([])
  const detectedModelSessionCache = ref(new Set<string>())
  const hasShownApiKeyHint = ref(false)
  const channelAccountsById = computed(() => {
    const grouped = new Map<number, AdminAccount[]>()
    accounts.value.forEach((account) => {
      const group = grouped.get(account.channel_id)
      if (group) {
        group.push(account)
        return
      }
      grouped.set(account.channel_id, [account])
    })
    return grouped
  })

  function showAccountActionError(error: unknown, fallback: string) {
    options.message.error(extractApiErrorMessage(error, fallback))
  }

  function showAccountActionWarning(error: unknown, fallback: string) {
    options.message.warning(extractApiErrorMessage(error, fallback))
  }

  async function loadAccountLimits() {
    try {
      accountLimits.value = await fetchAdminAccountLimits()
    } catch {
      accountLimits.value = FALLBACK_ADMIN_ACCOUNT_LIMITS
    }
  }

  async function loadAccounts() {
    const [nextAccounts] = await Promise.all([
      fetchAllAdminAccounts(),
      loadAccountLimits(),
    ])
    accounts.value = nextAccounts
  }

  function getChannelAccounts(channelId: number) {
    return channelAccountsById.value.get(channelId) || []
  }

  function getAccountModelRows(account: AdminAccount): AccountModelRow[] {
    return account.models.map((model) => ({
      name: model.name,
      disabled: !model.enabled,
    }))
  }

  const accountModelOptions = computed(() => {
    const detectedModels = detectedModelValues.value
    const currentValues = accountForm.value.model_values || []
    const allModels = new Set([...detectedModels, ...currentValues])
    return Array.from(allModels).map((m) => ({ label: m, value: m }))
  })

  function openAddAccountModal(channel: Channel) {
    isEditAccount.value = false
    selectedChannelForAccount.value = channel
    accountForm.value = createEmptyAccountForm(channel.id)
    detectedModelValues.value = []
    detectedModelSessionCache.value = new Set()
    hasShownApiKeyHint.value = false
    showAccountModal.value = true
  }

  function searchModelSuggestions(event: { query?: string }) {
    const query = (event.query || '').trim().toLowerCase()
    const options = accountModelOptions.value.map((item) => item.value)
    modelSuggestions.value = query
      ? options.filter((model) => model.toLowerCase().includes(query))
      : options
  }

  function openEditAccountModal(account: AdminAccount) {
    isEditAccount.value = true
    currentAccountId.value = account.id
    const channel = options.channels.value.find((c) => c.id === account.channel_id)
    selectedChannelForAccount.value = channel || null
    accountForm.value = {
      channel_id: account.channel_id,
      model_values: account.models.map((model) => model.name),
      secret: '',
      max_concurrent: account.max_concurrent,
      timeout: account.timeout,
    }
    detectedModelValues.value = []
    detectedModelSessionCache.value = new Set()
    hasShownApiKeyHint.value = false
    showAccountModal.value = true
  }

  async function handleDetectModelsOnSelectOpen() {
    const channel = selectedChannelForAccount.value
    const secret = (accountForm.value.secret || '').trim()
    const baseURL = (channel?.api_url || '').trim()

    if (!baseURL) return
    if (!secret) {
      if (!hasShownApiKeyHint.value) {
        hasShownApiKeyHint.value = true
        options.message.warning('请输入 API 密钥后再检测模型')
      }
      return
    }
    if (detectModelsLoading.value) return

    const apiType = channel?.type || DEFAULT_ADMIN_CHANNEL_API_TYPE
    const sessionKey = `${apiType}::${baseURL}::${secret}`
    if (detectedModelSessionCache.value.has(sessionKey)) return
    detectedModelSessionCache.value.add(sessionKey)

    await runAsyncAction(
      async () => {
        const detectedModels = await detectAdminChannelModels({
          base_url: baseURL,
          api_key: secret,
          api_type: apiType,
        })
        const models = detectedModels
          .map((m) => (m || '').trim())
          .filter(Boolean)
          .slice(0, MAX_DETECTED_MODELS)

        if (models.length > 0) {
          const merged = new Set([...detectedModelValues.value, ...models])
          detectedModelValues.value = Array.from(merged).slice(0, MAX_DETECTED_MODELS)
        }
      },
      {
        loading: detectModelsLoading,
        onError: (error) => showAccountActionWarning(error, '自动检测模型失败'),
      },
    )
  }

  async function handleSaveAccount() {
    const selectedModels = accountForm.value.model_values.map((item) => item.trim()).filter(Boolean)
    accountForm.value.model_values = Array.from(new Set(selectedModels))
    if (accountForm.value.model_values.length === 0) {
      options.message.warning('请至少选择或输入一个模型')
      return
    }
    if (!isEditAccount.value && !accountForm.value.secret) {
      options.message.warning('新增账号必须输入 API 密钥')
      return
    }

    await runAsyncAction(
      async () => {
        const ch = selectedChannelForAccount.value
        const existingAccount = isEditAccount.value
          ? accounts.value.find((account) => account.id === currentAccountId.value)
          : undefined
        const modelInputs = accountModelInputsFromValues(
          accountForm.value.model_values,
          existingAccount?.models,
        )
        const channelId =
          isEditAccount.value && existingAccount
            ? existingAccount.channel_id
            : accountForm.value.channel_id
        const baseData = {
          models: modelInputs,
          max_concurrent: accountForm.value.max_concurrent,
          timeout: accountForm.value.timeout,
          name: ch?.name || 'Unknown',
          platform: ch?.type || DEFAULT_ADMIN_CHANNEL_API_TYPE,
          api_type: ch?.type || DEFAULT_ADMIN_CHANNEL_API_TYPE,
          proxy_url: ch?.api_url || '',
        }

        if (isEditAccount.value) {
          const updateData: AdminAccountUpdateRequest = {
            ...baseData,
            weight: existingAccount?.weight ?? 100,
            extra_config: existingAccount?.extra_config ?? '{}',
            ...(accountForm.value.secret ? { secret: accountForm.value.secret } : {}),
          }
          await updateAdminAccountRecord(currentAccountId.value, updateData)
          options.message.success('账号已更新')
        } else {
          await createAdminAccountRecord({
            ...baseData,
            channel_id: channelId,
            secret: accountForm.value.secret,
            extra_config: '{}',
          })
          options.message.success('账号已创建')
        }

        showAccountModal.value = false
        await options.onAfterMutation()
      },
      {
        loading: accountSaveLoading,
        onError: (error) => showAccountActionError(error, '保存账号失败'),
      },
    )
  }

  async function toggleAccountModelStatus(account: AdminAccount, model: string) {
    const modelKey = normalizeModelKey(model)
    const target = account.models.find((item) => normalizeModelKey(item.name) === modelKey)
    if (!target) return
    const nextEnabled = !target.enabled
    const nextModels = account.models.map((item) =>
      normalizeModelKey(item.name) === modelKey ? { ...item, enabled: nextEnabled } : item,
    )

    await runAsyncAction(
      async () => {
        await updateAccountModelConfig(account, {
          models: nextModels,
        })
        options.message.success(nextEnabled ? '模型已启动' : '模型已停止')
      },
      {
        onError: (error) => showAccountActionError(error, '模型状态修改失败'),
      },
    )
  }

  async function updateAccountModelConfig(
    account: AdminAccount,
    patch: Partial<Pick<AdminAccount, 'models' | 'extra_config'>>,
  ) {
    const updatedAt = new Date().toISOString()
    const nextAccount: AdminAccount = {
      ...account,
      ...patch,
      updated_at: updatedAt,
    }

    await updateAdminAccountRecord(account.id, createAccountUpdatePayload(nextAccount))

    accounts.value = accounts.value.map((item) => (item.id === account.id ? nextAccount : item))
  }

  function createAccountUpdatePayload(account: AdminAccount): AdminAccountUpdateRequest {
    return {
      name: account.name,
      platform: account.platform,
      api_type: account.api_type,
      models: accountModelInputsFromModels(account.models),
      proxy_url: account.proxy_url,
      max_concurrent: account.max_concurrent,
      timeout: account.timeout,
      weight: account.weight,
      extra_config: account.extra_config || '{}',
    }
  }

  function handleDeleteAccount(account: AdminAccount) {
    options.dialog.warning({
      title: '确认删除 APIKey',
      content: '确定删除该 APIKey 吗？关联的执行中任务会由后端统一结束。',
      onConfirm: async () => {
        try {
          await deleteAdminAccountRecord(account.id)
          options.message.success('账号已删除')
          await options.onAfterMutation()
        } catch (error) {
          options.message.error(extractApiErrorMessage(error, '删除账号失败'))
        }
      },
    })
  }

  return {
    accounts,
    showAccountModal,
    isEditAccount,
    accountSaveLoading,
    accountLimits,
    selectedChannelForAccount,
    accountForm,
    detectModelsLoading,
    modelSuggestions,
    loadAccounts,
    getChannelAccounts,
    getAccountModelRows,
    openAddAccountModal,
    openEditAccountModal,
    handleDetectModelsOnSelectOpen,
    searchModelSuggestions,
    handleSaveAccount,
    toggleAccountModelStatus,
    handleDeleteAccount,
  }
}

function accountModelInputsFromValues(
  values: string[],
  existingModels: AdminAccountModel[] = [],
): AdminAccountModelInput[] {
  const existingByKey = new Map(
    existingModels.map((model) => [normalizeModelKey(model.name), model]),
  )
  return uniqueTrimmedValues(values).map((name) => {
    const existing = existingByKey.get(normalizeModelKey(name))
    return {
      name,
      enabled: existing?.enabled ?? true,
      weight: existing?.weight ?? 1,
      max_concurrent: existing?.max_concurrent,
      feature_slugs: existing?.feature_slugs,
    }
  })
}

function accountModelInputsFromModels(models: AdminAccountModel[]): AdminAccountModelInput[] {
  return models.map((model) => ({
    name: model.name,
    enabled: model.enabled,
    weight: model.weight,
    max_concurrent: model.max_concurrent,
    feature_slugs: model.feature_slugs,
  }))
}

function uniqueTrimmedValues(values: string[]) {
  const seen = new Set<string>()
  const result: string[] = []
  values.forEach((value) => {
    const trimmed = value.trim()
    if (!trimmed) return
    const key = normalizeModelKey(trimmed)
    if (seen.has(key)) return
    seen.add(key)
    result.push(trimmed)
  })
  return result
}

function normalizeModelKey(model: string) {
  return model.trim().toLowerCase()
}
