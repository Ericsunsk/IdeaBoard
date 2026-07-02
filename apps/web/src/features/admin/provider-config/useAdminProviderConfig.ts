import { computed, ref, watch } from 'vue'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { extractSafeApiErrorMessage as extractApiErrorMessage } from '../../../shared/utils/errorMessages'
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
  createProviderChannelView,
  createProviderChannelCards,
} from './providerConfigCards'
import {
  ADMIN_CHANNEL_API_TYPE_OPTIONS,
  createAccountCreatePayload,
  createAccountUpdatePayload,
  createAccountUpdatePayloadFromAccount,
  createEmptyChannelForm,
  getAdminChannelApiTypeLabel,
  inferChannelApiTypeFromUrl,
  normalizeModelKey,
  uniqueTrimmedModelValues,
} from './providerConfigPayload'
import {
  createModelSuggestions,
  MAX_DETECTED_MODELS,
  mergeDetectedModelValues,
  normalizeDetectedModelValues,
} from './providerModelOptions'
import type {
  AdminProviderAccountLimits,
  ProviderAccountCommandTarget,
  ProviderAccountForm,
  ProviderAccountFormField,
  ProviderAccountRecord,
  ProviderAccountView,
  ProviderChannelCommandTarget,
  ProviderChannelFormData,
  ProviderChannelFormField,
  ProviderChannelRecord,
} from './providerConfigTypes'

type AdminProviderMessagePort = {
  success: (content: string) => void
  error: (content: string) => void
  warning: (content: string) => void
}

type AdminProviderConfirmPort = {
  danger: (options: {
    title: string
    content: string
    acceptLabel: string
    onConfirm: () => void | Promise<void>
  }) => void
}

export type UseAdminProviderConfigOptions = {
  message: AdminProviderMessagePort
  confirm: AdminProviderConfirmPort
}

export function useAdminProviderConfig(options: UseAdminProviderConfigOptions) {
  const { runAsyncAction } = useAsyncAction()
  const loading = ref(false)
  const channels = ref<ProviderChannelRecord[]>([])
  const accounts = ref<ProviderAccountRecord[]>([])
  const searchKeyword = ref('')

  const channelEditor = createChannelEditor({
    channels,
    message: options.message,
    reload: loadData,
    runAsyncAction,
  })
  const accountEditor = createAccountEditor({
    accounts,
    channels,
    message: options.message,
    reload: loadData,
    runAsyncAction,
  })

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
  const cards = {
    items: cardItems,
    searchKeyword,
    isEmpty: computed(() => filteredChannels.value.length === 0),
    isLoading: computed(() => loading.value && channels.value.length === 0),
    hasSearch: hasChannelSearch,
    totalChannelsLabel,
    emptyMessage: emptyChannelMessage,
    accountRunningLabel,
    activeModelCount,
  }

  async function loadAccountLimits() {
    try {
      accountEditor.accountLimits.value = await fetchAdminAccountLimits()
    } catch (error) {
      accountEditor.accountLimits.value = FALLBACK_ADMIN_ACCOUNT_LIMITS
      const message = extractApiErrorMessage(error, '账号限制加载失败，已使用默认限制')
      options.message.warning(message)
      console.warn('[provider-config] failed to load account limits, using fallback', error)
    }
  }

  async function loadAccounts() {
    const [nextAccounts] = await Promise.all([fetchAllAdminAccounts(), loadAccountLimits()])
    accounts.value = nextAccounts
  }

  async function loadData() {
    await runAsyncAction(
      async () => {
        channels.value = await fetchAdminChannels()
        await loadAccounts()
      },
      {
        loading,
        feedback: {
          error: (error) => showActionError(options.message, error, '加载数据失败'),
        },
      },
    )
  }

  function deleteChannel(target: ProviderChannelCommandTarget) {
    const channel = findChannelByTarget(channels.value, target)
    if (!channel) {
      options.message.warning('渠道商已刷新，请重新选择')
      return
    }

    options.confirm.danger({
      title: '确认删除',
      content: `确定删除渠道商「${channel.name}」吗？此操作不可恢复。`,
      acceptLabel: '确定',
      onConfirm: async () => {
        try {
          await deleteAdminChannel(channel.id)
          options.message.success('删除成功')
          await loadData()
        } catch (error) {
          options.message.error(extractApiErrorMessage(error, '删除渠道商失败'))
        }
      },
    })
  }

  function deleteAccount(target: ProviderAccountCommandTarget) {
    const account = findAccountByTarget(accounts.value, target)
    if (!account) {
      options.message.warning('账号已刷新，请重新选择')
      return
    }

    options.confirm.danger({
      title: '确认删除 APIKey',
      content: '确定删除该 APIKey 吗？关联的执行中任务会由后端统一结束。',
      acceptLabel: '确定',
      onConfirm: async () => {
        try {
          await deleteAdminAccountRecord(account.id)
          options.message.success('账号已删除')
          await loadData()
        } catch (error) {
          options.message.error(extractApiErrorMessage(error, '删除账号失败'))
        }
      },
    })
  }

  async function toggleAccountModelStatus(
    commandTarget: ProviderAccountCommandTarget,
    model: string,
  ) {
    const account = findAccountByTarget(accounts.value, commandTarget)
    if (!account) {
      options.message.warning('账号已刷新，请重新选择')
      return
    }

    const modelKey = normalizeModelKey(model)
    const targetModel = account.models.find((item) => normalizeModelKey(item.name) === modelKey)
    if (!targetModel) {
      options.message.warning('模型已刷新，请重新选择')
      return
    }
    const nextEnabled = !targetModel.enabled
    const nextModels = account.models.map((item) =>
      normalizeModelKey(item.name) === modelKey ? { ...item, enabled: nextEnabled } : item,
    )

    await runAsyncAction(
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
        options.message.success(nextEnabled ? '模型已启动' : '模型已停止')
      },
      {
        feedback: {
          error: (error) => showActionError(options.message, error, '模型状态修改失败'),
        },
      },
    )
  }

  return {
    cards,
    channelEditor,
    accountEditor,
    commands: {
      loadData,
      deleteChannel,
      deleteAccount,
      toggleAccountModelStatus,
      getApiTypeLabel: getAdminChannelApiTypeLabel,
      getAccountModelRows: createAccountModelRows,
      getAccountStatusDotClass,
      getAccountProgressPt,
    },
  }
}

function createChannelEditor(options: {
  channels: Readonly<{ value: ProviderChannelRecord[] }>
  message: AdminProviderMessagePort
  reload: () => Promise<void> | void
  runAsyncAction: ReturnType<typeof useAsyncAction>['runAsyncAction']
}) {
  const visible = ref(false)
  const isEditing = ref(false)
  const saveLoading = ref(false)
  const currentChannelId = ref(0)
  const form = ref<ProviderChannelFormData>(createEmptyChannelForm())
  const errors = ref<Record<ProviderChannelFormField, string>>(createEmptyChannelFormErrors())

  function openCreate() {
    isEditing.value = false
    currentChannelId.value = 0
    form.value = createEmptyChannelForm()
    resetErrors()
    visible.value = true
  }

  function openEdit(target: ProviderChannelCommandTarget) {
    const row = findChannelByTarget(options.channels.value, target)
    if (!row) {
      options.message.warning('渠道商已刷新，请重新选择')
      return
    }

    isEditing.value = true
    currentChannelId.value = row.id
    form.value = { apiType: row.apiType, name: row.name, baseUrl: row.baseUrl }
    resetErrors()
    visible.value = true
  }

  function close() {
    visible.value = false
    resetErrors()
  }

  function inferApiTypeFromUrl() {
    form.value.apiType = inferChannelApiTypeFromUrl(form.value.baseUrl, form.value.apiType)
  }

  function handleApiUrlBlur() {
    inferApiTypeFromUrl()
    validateField('baseUrl')
  }

  async function save() {
    if (!validate()) {
      options.message.warning('请修正渠道商表单错误')
      return
    }

    await options.runAsyncAction(
      async () => {
        await saveAdminChannel({
          id: isEditing.value ? currentChannelId.value : undefined,
          data: { ...form.value },
        })
        options.message.success(isEditing.value ? '渠道商已更新' : '渠道商已创建')
        visible.value = false
        resetErrors()
        await options.reload()
      },
      {
        loading: saveLoading,
        feedback: {
          error: (error) => showActionError(options.message, error, '保存渠道商失败'),
        },
      },
    )
  }

  function validate() {
    const nextErrors = createEmptyChannelFormErrors()
    ;(['name', 'apiType', 'baseUrl'] as ProviderChannelFormField[]).forEach((field) => {
      nextErrors[field] = resolveChannelFieldError(form.value, field)
    })
    errors.value = nextErrors
    return !Object.values(nextErrors).some(Boolean)
  }

  function validateField(field: ProviderChannelFormField) {
    errors.value[field] = resolveChannelFieldError(form.value, field)
    return !errors.value[field]
  }

  function clearError(field: ProviderChannelFormField) {
    if (errors.value[field]) {
      errors.value[field] = ''
    }
  }

  function resetErrors() {
    errors.value = createEmptyChannelFormErrors()
  }

  return {
    visible,
    isEditing,
    saveLoading,
    form,
    errors,
    apiTypeOptions: ADMIN_CHANNEL_API_TYPE_OPTIONS,
    openCreate,
    openEdit,
    close,
    save,
    clearError,
    handleApiUrlBlur,
  }
}

function createAccountEditor(options: {
  accounts: Readonly<{ value: ProviderAccountRecord[] }>
  channels: Readonly<{ value: ProviderChannelRecord[] }>
  message: AdminProviderMessagePort
  reload: () => Promise<void> | void
  runAsyncAction: ReturnType<typeof useAsyncAction>['runAsyncAction']
}) {
  const visible = ref(false)
  const isEditing = ref(false)
  const saveLoading = ref(false)
  const currentAccountId = ref(0)
  const selectedChannel = ref<ProviderChannelRecord | null>(null)
  const selectedChannelView = computed(() =>
    selectedChannel.value ? createProviderChannelView(selectedChannel.value) : null,
  )
  const accountLimits = ref<AdminProviderAccountLimits>(FALLBACK_ADMIN_ACCOUNT_LIMITS)
  const form = ref(createEmptyAccountForm(accountLimits.value, 0))
  const errors = ref<Record<ProviderAccountFormField, string>>(createEmptyAccountFormErrors())
  const detectModelsLoading = ref(false)
  const detectedModelValues = ref<string[]>([])
  const modelSuggestions = ref<string[]>([])
  const detectedModelSessionCache = ref(new Set<string>())
  const hasShownApiKeyHint = ref(false)

  function openCreate(target: ProviderChannelCommandTarget) {
    const channel = findChannelByTarget(options.channels.value, target)
    if (!channel) {
      options.message.warning('渠道商已刷新，请重新选择')
      return
    }

    isEditing.value = false
    currentAccountId.value = 0
    selectedChannel.value = channel
    form.value = createEmptyAccountForm(accountLimits.value, channel.id)
    detectedModelValues.value = []
    modelSuggestions.value = []
    detectedModelSessionCache.value = new Set()
    hasShownApiKeyHint.value = false
    resetErrors()
    visible.value = true
  }

  function openEdit(target: ProviderAccountCommandTarget) {
    const account = findAccountByTarget(options.accounts.value, target)
    if (!account) {
      options.message.warning('账号已刷新，请重新选择')
      return
    }

    isEditing.value = true
    currentAccountId.value = account.id
    selectedChannel.value =
      options.channels.value.find((channel) => channel.id === account.channelId) || null
    form.value = {
      channelId: account.channelId,
      modelValues: account.models.map((model) => model.name),
      secret: '',
      maxConcurrent: account.maxConcurrent,
      timeoutSeconds: account.timeoutSeconds,
    }
    detectedModelValues.value = []
    modelSuggestions.value = []
    detectedModelSessionCache.value = new Set()
    hasShownApiKeyHint.value = false
    resetErrors()
    visible.value = true
  }

  function close() {
    visible.value = false
    resetErrors()
  }

  function clearError(field: ProviderAccountFormField) {
    if (errors.value[field]) {
      errors.value[field] = ''
    }
  }

  function resetErrors() {
    errors.value = createEmptyAccountFormErrors()
  }

  function validate() {
    const nextErrors = createEmptyAccountFormErrors()
    const selectedModels = uniqueTrimmedModelValues(form.value.modelValues)
    if (selectedModels.length === 0) {
      nextErrors.modelValues = '请至少选择或输入一个模型'
    }
    if (!isEditing.value && !form.value.secret.trim()) {
      nextErrors.secret = '新增账号必须输入 API 密钥'
    }
    nextErrors.maxConcurrent = resolveNumberRangeError(
      form.value.maxConcurrent,
      accountLimits.value.maxConcurrent.min,
      accountLimits.value.maxConcurrent.max,
      '最大并发数',
    )
    nextErrors.timeoutSeconds = resolveNumberRangeError(
      form.value.timeoutSeconds,
      accountLimits.value.timeoutSeconds.min,
      accountLimits.value.timeoutSeconds.max,
      '超时时间',
    )
    errors.value = nextErrors
    return !Object.values(nextErrors).some(Boolean)
  }

  async function save() {
    if (!validate()) {
      options.message.warning('请修正 APIKey 表单错误')
      return
    }

    const modelValues = uniqueTrimmedModelValues(form.value.modelValues)
    form.value.modelValues = modelValues

    await options.runAsyncAction(
      async () => {
        const existingAccount = isEditing.value
          ? options.accounts.value.find((account) => account.id === currentAccountId.value)
          : undefined
        const secret = form.value.secret.trim()

        if (isEditing.value) {
          await updateAdminAccountRecord(
            currentAccountId.value,
            createAccountUpdatePayload({
              form: form.value,
              channel: selectedChannel.value,
              existingAccount,
              modelValues,
              secret,
            }),
          )
          options.message.success('账号已更新')
        } else {
          await createAdminAccountRecord(
            createAccountCreatePayload({
              form: form.value,
              channel: selectedChannel.value,
              modelValues,
              secret,
            }),
          )
          options.message.success('账号已创建')
        }

        visible.value = false
        resetErrors()
        await options.reload()
      },
      {
        loading: saveLoading,
        feedback: {
          error: (error) => showActionError(options.message, error, '保存账号失败'),
        },
      },
    )
  }

  function searchModelSuggestions(event: { query?: string }) {
    modelSuggestions.value = createModelSuggestions({
      detectedValues: detectedModelValues.value,
      selectedValues: form.value.modelValues,
      query: event.query,
    })
  }

  async function detectModelsOnSelectOpen() {
    const channel = selectedChannel.value
    const secret = form.value.secret.trim()
    const baseURL = (channel?.baseUrl || '').trim()

    if (!baseURL) return
    if (!secret) {
      if (!hasShownApiKeyHint.value) {
        hasShownApiKeyHint.value = true
        options.message.warning('请输入 API 密钥后再检测模型')
      }
      return
    }
    if (detectModelsLoading.value) return

    const apiType = channel?.apiType || DEFAULT_PROVIDER_API_TYPE
    const sessionKey = `${apiType}::${baseURL}::${secret}`
    if (detectedModelSessionCache.value.has(sessionKey)) return
    detectedModelSessionCache.value.add(sessionKey)

    await options.runAsyncAction(
      async () => {
        const detectedModels = await detectAdminChannelModels({
          baseUrl: baseURL,
          apiKey: secret,
          apiType,
        })
        const models = normalizeDetectedModelValues(detectedModels, MAX_DETECTED_MODELS)

        if (models.length > 0) {
          detectedModelValues.value = mergeDetectedModelValues(
            detectedModelValues.value,
            models,
            MAX_DETECTED_MODELS,
          )
        }
      },
      {
        loading: detectModelsLoading,
        feedback: {
          error: (error) => {
            options.message.warning(extractApiErrorMessage(error, '自动检测模型失败'))
          },
        },
      },
    )
  }

  function handleModelSelectShow(show: boolean) {
    if (!show) return
    void detectModelsOnSelectOpen()
  }

  watch(visible, (nextVisible) => {
    if (!nextVisible) resetErrors()
  })

  return {
    visible,
    isEditing,
    saveLoading,
    accountLimits,
    selectedChannel: selectedChannelView,
    form,
    errors,
    detectModelsLoading,
    modelSuggestions,
    openCreate,
    openEdit,
    close,
    save,
    clearError,
    searchModelSuggestions,
    handleModelSelectShow,
  }
}

const activeProgressPt = { value: { class: '!bg-admin-success' } } as const
const mutedProgressPt = { value: { class: '!bg-admin-text-3' } } as const
const DEFAULT_PROVIDER_API_TYPE = ADMIN_CHANNEL_API_TYPE_OPTIONS[0].value

function createEmptyChannelFormErrors(): Record<ProviderChannelFormField, string> {
  return {
    apiType: '',
    name: '',
    baseUrl: '',
  }
}

function createEmptyAccountForm(
  accountLimits: AdminProviderAccountLimits,
  channelId: number,
): ProviderAccountForm {
  return {
    channelId,
    modelValues: [],
    secret: '',
    maxConcurrent: accountLimits.maxConcurrent.default,
    timeoutSeconds: accountLimits.timeoutSeconds.default,
  }
}

function createEmptyAccountFormErrors(): Record<ProviderAccountFormField, string> {
  return {
    secret: '',
    modelValues: '',
    maxConcurrent: '',
    timeoutSeconds: '',
  }
}

function resolveChannelFieldError(form: ProviderChannelFormData, field: ProviderChannelFormField) {
  if (field === 'name' && !form.name.trim()) return '请输入渠道商名称'
  if (field === 'apiType' && !form.apiType) return '请选择接口类型'
  if (field === 'baseUrl') {
    const apiUrl = form.baseUrl.trim()
    if (!apiUrl) return '请输入 Base URL地址'
    try {
      const parsed = new URL(apiUrl)
      if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
        return 'Base URL 需使用 http 或 https'
      }
    } catch {
      return '请输入有效的 Base URL'
    }
  }
  return ''
}

function resolveNumberRangeError(value: number, min: number, max: number, label: string) {
  if (!Number.isFinite(value)) return `请输入${label}`
  if (value < min || value > max) return `${label}需在 ${min}-${max} 之间`
  return ''
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

function showActionError(message: AdminProviderMessagePort, error: unknown, fallback: string) {
  message.error(extractApiErrorMessage(error, fallback))
}

export type AdminProviderConfigContext = ReturnType<typeof useAdminProviderConfig>
