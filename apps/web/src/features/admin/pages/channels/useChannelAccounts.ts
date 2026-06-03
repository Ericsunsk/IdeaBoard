import type { FormRules } from 'naive-ui'
import accountApi, {
  FALLBACK_ACCOUNT_LIMITS,
  normalizeAccountLimits,
  type AccountLimits,
  type ApiAccount,
} from '../../../../shared/api/account'
import channelApi, { type Channel } from '../../../../shared/api/channel'
import { extractApiErrorMessage, splitCommaValues } from '../../shared/lib/assetLibraryShared'

const ACCOUNT_PAGE_SIZE = 100
const MAX_DETECTED_MODELS = 100

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
      positiveText: string
      negativeText: string
      onPositiveClick: () => void | Promise<void>
    }) => void
  }
  onAfterMutation: () => Promise<void> | void
}) {
  const accounts = ref<ApiAccount[]>([])
  const accountLoading = ref(false)
  const showAccountModal = ref(false)
  const isEditAccount = ref(false)
  const accountFormRef = ref()
  const accountSaveLoading = ref(false)
  const currentAccountId = ref(0)
  const selectedChannelForAccount = ref<Channel | null>(null)
  const accountLimits = ref<AccountLimits>(FALLBACK_ACCOUNT_LIMITS)

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
  const detectedModelSessionCache = ref(new Set<string>())
  const hasShownApiKeyHint = ref(false)

  async function loadAccountLimits() {
    try {
      const res = await accountApi.getAccountLimits()
      accountLimits.value = normalizeAccountLimits(res.data?.limits)
    } catch {
      accountLimits.value = FALLBACK_ACCOUNT_LIMITS
    }
  }

  async function loadAllAccounts(): Promise<ApiAccount[]> {
    const all: ApiAccount[] = []
    let page = 1
    let total = Number.POSITIVE_INFINITY

    while (true) {
      const res = await accountApi.listAccounts({ page, page_size: ACCOUNT_PAGE_SIZE })
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
      const [nextAccounts] = await Promise.all([loadAllAccounts(), loadAccountLimits()])
      accounts.value = nextAccounts
    } finally {
      accountLoading.value = false
    }
  }

  function getChannelAccounts(channelId: number) {
    return accounts.value.filter((a) => a.channel_id === channelId)
  }

  function getChannelActiveModels(channelId: number) {
    const accs = getChannelAccounts(channelId)
    const models = new Set<string>()
    accs.forEach((acc) => {
      if (acc.model_set) {
        splitCommaValues(acc.model_set).forEach((m) => models.add(m))
      }
    })
    return Array.from(models)
  }

  function getAccStatusClass(acc: ApiAccount) {
    if (acc.status === 0) return 'acc-warning'
    if (acc.fail_count >= 5) return 'acc-error'
    return 'acc-success'
  }

  const accountModelOptions = computed(() => {
    const detectedModels = detectedModelValues.value
    const currentValues = accountForm.value.model_values || []
    const allModels = new Set([...detectedModels, ...currentValues])
    return Array.from(allModels).map((m) => ({ label: m, value: m }))
  })

  const accountRules: FormRules = {
    model_values: [
      {
        required: true,
        type: 'array',
        min: 1,
        message: '请至少选择一个模型',
        trigger: ['change', 'blur'],
      },
    ],
  }

  function openAddAccountModal(channel: Channel) {
    isEditAccount.value = false
    selectedChannelForAccount.value = channel
    accountForm.value = createEmptyAccountForm(channel.id)
    detectedModelValues.value = []
    detectedModelSessionCache.value = new Set()
    hasShownApiKeyHint.value = false
    showAccountModal.value = true
  }

  function openEditAccountModal(account: ApiAccount) {
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
      await accountFormRef.value?.validate()
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

      if (isEditAccount.value) {
        const updateData = {
          ...baseData,
          ...(accountForm.value.api_key ? { api_key: accountForm.value.api_key } : {}),
        }
        await accountApi.updateAccount(currentAccountId.value, updateData)
        options.message.success('账号已更新')
      } else {
        await accountApi.createAccount({ ...baseData, api_key: accountForm.value.api_key })
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

  async function toggleAccountStatus(account: ApiAccount) {
    try {
      if (account.status === 1) {
        await accountApi.stopAccount(account.id)
        options.message.success('账号已停止')
      } else {
        await accountApi.startAccount(account.id)
        options.message.success('账号已启动')
      }
      await options.onAfterMutation()
    } catch (error) {
      options.message.error(extractApiErrorMessage(error, '状态修改失败'))
    }
  }

  function handleDeleteAccount(account: ApiAccount) {
    options.dialog.warning({
      title: '确认删除',
      content: '确定删除该账号吗？',
      positiveText: '确定',
      negativeText: '取消',
      onPositiveClick: async () => {
        try {
          await accountApi.deleteAccount(account.id)
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
    accountFormRef,
    accountSaveLoading,
    accountLimits,
    selectedChannelForAccount,
    accountForm,
    detectModelsLoading,
    accountModelOptions,
    accountRules,
    loadAccounts,
    getChannelAccounts,
    getChannelActiveModels,
    getAccStatusClass,
    openAddAccountModal,
    openEditAccountModal,
    handleDetectModelsOnSelectOpen,
    handleSaveAccount,
    toggleAccountStatus,
    handleDeleteAccount,
  }
}
