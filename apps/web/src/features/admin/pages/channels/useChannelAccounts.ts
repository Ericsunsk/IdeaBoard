import channelApi, { type Channel } from '../../../../shared/api/channel'
import adminAccountApi, {
  FALLBACK_ADMIN_ACCOUNT_LIMITS,
  normalizeAdminAccountLimits,
  type AdminAccount,
  type AdminAccountLimits,
  type AdminAccountUpdateRequest,
} from '../../shared/api/adminAccounts'
import {
  createMockAdminAccounts,
  mockAdminAccountLimits,
  useAdminMockMode,
} from '../../shared/lib/adminMockData'
import { extractApiErrorMessage, splitCommaValues } from './channelModelUtils'

const ACCOUNT_PAGE_SIZE = 100
const MAX_DETECTED_MODELS = 100
const DISABLED_MODELS_CONFIG_KEY = 'disabled_models'

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
  const accounts = ref<AdminAccount[]>([])
  const accountLoading = ref(false)
  const isMockMode = useAdminMockMode()
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
      api_key: '',
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

  async function loadAccountLimits() {
    if (isMockMode.value) {
      accountLimits.value = mockAdminAccountLimits
      return
    }

    try {
      const res = await adminAccountApi.getLimits()
      accountLimits.value = normalizeAdminAccountLimits(res.data?.limits)
    } catch {
      accountLimits.value = FALLBACK_ADMIN_ACCOUNT_LIMITS
    }
  }

  async function loadAllAccounts(): Promise<AdminAccount[]> {
    const all: AdminAccount[] = []
    let page = 1
    let total = Number.POSITIVE_INFINITY

    while (true) {
      const res = await adminAccountApi.list({
        page,
        page_size: ACCOUNT_PAGE_SIZE,
      })
      if (res.code !== 0) {
        throw new Error(res.msg || '加载账号失败')
      }
      const list = res.data?.list || []
      total = res.data?.total ?? total
      all.push(...list)
      if (all.length >= total || list.length === 0) break
      page += 1
    }

    return all
  }

  async function loadAccounts() {
    accountLoading.value = true
    try {
      if (isMockMode.value) {
        accounts.value = createMockAdminAccounts()
        accountLimits.value = mockAdminAccountLimits
        return
      }

      const [nextAccounts] = await Promise.all([loadAllAccounts(), loadAccountLimits()])
      accounts.value = nextAccounts
    } finally {
      accountLoading.value = false
    }
  }

  function getChannelAccounts(channelId: number) {
    return channelAccountsById.value.get(channelId) || []
  }

  function getAccountModelRows(account: AdminAccount): AccountModelRow[] {
    const disabledModels = readDisabledModelKeys(account)
    return splitCommaValues(account.model_set).map((model) => ({
      name: model,
      disabled: disabledModels.has(normalizeModelKey(model)),
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
      model_values: splitCommaValues(account.model_set),
      api_key: '',
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
    const apiKey = (accountForm.value.api_key || '').trim()
    const baseURL = (channel?.api_url || '').trim()

    if (!baseURL) return
    if (!apiKey) {
      if (!hasShownApiKeyHint.value) {
        hasShownApiKeyHint.value = true
        options.message.warning('请输入 API 密钥后再检测模型')
      }
      return
    }
    if (detectModelsLoading.value) return

    const apiType = channel?.type || 'openai-compatible'
    const sessionKey = `${apiType}::${baseURL}::${apiKey}`
    if (detectedModelSessionCache.value.has(sessionKey)) return
    detectedModelSessionCache.value.add(sessionKey)

    detectModelsLoading.value = true
    try {
      const res = await channelApi.detectModels({
        base_url: baseURL,
        api_key: apiKey,
        api_type: apiType,
      })
      const models = (res.data?.models || [])
        .map((m) => (m || '').trim())
        .filter(Boolean)
        .slice(0, MAX_DETECTED_MODELS)

      if (models.length > 0) {
        const merged = new Set([...detectedModelValues.value, ...models])
        detectedModelValues.value = Array.from(merged).slice(0, MAX_DETECTED_MODELS)
      }
    } catch (error) {
      options.message.warning(extractApiErrorMessage(error, '自动检测模型失败'))
    } finally {
      detectModelsLoading.value = false
    }
  }

  async function handleSaveAccount() {
    try {
      const selectedModels = accountForm.value.model_values
        .map((item) => item.trim())
        .filter(Boolean)
      accountForm.value.model_values = Array.from(new Set(selectedModels))
      if (accountForm.value.model_values.length === 0) {
        options.message.warning('请至少选择或输入一个模型')
        return
      }
      if (!isEditAccount.value && !accountForm.value.api_key) {
        options.message.warning('新增账号必须输入 API 密钥')
        return
      }

      accountSaveLoading.value = true
      const ch = selectedChannelForAccount.value
      const baseData = {
        channel_id: accountForm.value.channel_id,
        model_set: accountForm.value.model_values.join(','),
        max_concurrent: accountForm.value.max_concurrent,
        timeout: accountForm.value.timeout,
        name: ch?.name || 'Unknown',
        platform: ch?.type || 'openai-compatible',
        api_type: ch?.type || 'openai-compatible',
        proxy_url: ch?.api_url || '',
      }
      const existingAccount = isEditAccount.value
        ? accounts.value.find((account) => account.id === currentAccountId.value)
        : undefined
      const nextExtraConfig = reconcileDisabledModels(
        existingAccount,
        accountForm.value.model_values,
      )

      if (isMockMode.value) {
        const now = new Date().toISOString()
        const nextAccount: AdminAccount = {
          id:
            existingAccount?.id ??
            Math.max(900, ...accounts.value.map((account) => account.id)) + 1,
          name: baseData.name,
          channel_id: baseData.channel_id,
          platform: baseData.platform,
          api_type: baseData.api_type,
          proxy_url: baseData.proxy_url,
          model_set: baseData.model_set,
          max_concurrent: baseData.max_concurrent,
          current_concurrent: existingAccount?.current_concurrent ?? 0,
          wait_queue_count: existingAccount?.wait_queue_count ?? 0,
          running_status: existingAccount?.running_status ?? 'running',
          fail_count: existingAccount?.fail_count ?? 0,
          weight: existingAccount?.weight ?? 100,
          timeout: baseData.timeout,
          extra_config: nextExtraConfig,
          status: existingAccount?.status ?? 1,
          created_at: existingAccount?.created_at ?? now,
          updated_at: now,
        }
        accounts.value = isEditAccount.value
          ? accounts.value.map((account) =>
              account.id === currentAccountId.value ? nextAccount : account,
            )
          : [nextAccount, ...accounts.value]
        options.message.success(isEditAccount.value ? '账号已更新' : '账号已创建')
        showAccountModal.value = false
        return
      }

      if (isEditAccount.value) {
        const updateData = {
          ...baseData,
          weight: existingAccount?.weight ?? 100,
          status: existingAccount?.status ?? 1,
          extra_config: nextExtraConfig,
          ...(accountForm.value.api_key ? { api_key: accountForm.value.api_key } : {}),
        }
        await adminAccountApi.update(currentAccountId.value, updateData)
        options.message.success('账号已更新')
      } else {
        await adminAccountApi.create({
          ...baseData,
          api_key: accountForm.value.api_key,
        })
        options.message.success('账号已创建')
      }

      showAccountModal.value = false
      await options.onAfterMutation()
    } catch (error) {
      options.message.error(extractApiErrorMessage(error, '保存账号失败'))
    } finally {
      accountSaveLoading.value = false
    }
  }

  async function toggleAccountModelStatus(account: AdminAccount, model: string) {
    const currentDisabledModels = readDisabledModelNames(account)
    const modelKey = normalizeModelKey(model)
    const isDisabled = currentDisabledModels.some((item) => normalizeModelKey(item) === modelKey)
    const nextDisabledModels = isDisabled
      ? currentDisabledModels.filter((item) => normalizeModelKey(item) !== modelKey)
      : [...currentDisabledModels, model]

    try {
      await updateAccountModelConfig(account, {
        extra_config: writeDisabledModels(account, nextDisabledModels),
      })
      options.message.success(isDisabled ? '模型已启动' : '模型已停止')
    } catch (error) {
      options.message.error(extractApiErrorMessage(error, '模型状态修改失败'))
    }
  }

  async function updateAccountModelConfig(
    account: AdminAccount,
    patch: Partial<Pick<AdminAccountUpdateRequest, 'model_set' | 'extra_config'>>,
  ) {
    const updatedAt = new Date().toISOString()
    const nextAccount: AdminAccount = {
      ...account,
      ...patch,
      updated_at: updatedAt,
    }

    if (!isMockMode.value) {
      await adminAccountApi.update(account.id, createAccountUpdatePayload(nextAccount))
    }

    accounts.value = accounts.value.map((item) => (item.id === account.id ? nextAccount : item))
  }

  function createAccountUpdatePayload(account: AdminAccount): AdminAccountUpdateRequest {
    return {
      name: account.name,
      channel_id: account.channel_id,
      platform: account.platform,
      api_type: account.api_type,
      model_set: account.model_set,
      proxy_url: account.proxy_url,
      max_concurrent: account.max_concurrent,
      timeout: account.timeout,
      weight: account.weight,
      extra_config: account.extra_config || '{}',
      status: account.status,
    }
  }

  function handleDeleteAccount(account: AdminAccount) {
    options.dialog.warning({
      title: '确认删除 APIKey',
      content: '确定删除该 APIKey 吗？删除前会自动停用该账号。',
      onConfirm: async () => {
        try {
          if (isMockMode.value) {
            accounts.value = accounts.value.filter((item) => item.id !== account.id)
            options.message.success('账号已删除')
            return
          }

          if (account.status === 1) {
            await adminAccountApi.stop(account.id)
          }
          await adminAccountApi.delete(account.id)
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
    accountLoading,
    showAccountModal,
    isEditAccount,
    accountSaveLoading,
    accountLimits,
    selectedChannelForAccount,
    accountForm,
    detectModelsLoading,
    modelSuggestions,
    accountModelOptions,
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

function readAccountExtraConfig(account: AdminAccount): Record<string, unknown> {
  try {
    const payload: unknown = JSON.parse(account.extra_config || '{}')
    if (payload && typeof payload === 'object' && !Array.isArray(payload)) {
      return { ...(payload as Record<string, unknown>) }
    }
  } catch {
    return {}
  }
  return {}
}

function readDisabledModelNames(account: AdminAccount): string[] {
  const payload = readAccountExtraConfig(account)
  return readStringList(payload[DISABLED_MODELS_CONFIG_KEY])
}

function readDisabledModelKeys(account: AdminAccount) {
  return new Set(readDisabledModelNames(account).map(normalizeModelKey))
}

function writeDisabledModels(account: AdminAccount, disabledModels: string[]) {
  const payload = readAccountExtraConfig(account)
  const normalizedModels = uniqueTrimmedValues(disabledModels)
  if (normalizedModels.length > 0) {
    payload[DISABLED_MODELS_CONFIG_KEY] = normalizedModels
  } else {
    delete payload[DISABLED_MODELS_CONFIG_KEY]
  }
  return JSON.stringify(payload)
}

function reconcileDisabledModels(account: AdminAccount | undefined, modelValues: string[]) {
  if (!account) return '{}'
  const activeModelKeys = new Set(modelValues.map(normalizeModelKey))
  const disabledModels = readDisabledModelNames(account).filter((model) =>
    activeModelKeys.has(normalizeModelKey(model)),
  )
  return writeDisabledModels(account, disabledModels)
}

function readStringList(value: unknown): string[] {
  if (typeof value === 'string') return splitCommaValues(value)
  if (!Array.isArray(value)) return []
  return uniqueTrimmedValues(value.map((item) => (typeof item === 'string' ? item : '')))
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
