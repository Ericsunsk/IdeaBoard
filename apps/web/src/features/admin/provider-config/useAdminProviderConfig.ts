import { computed, ref, watch } from 'vue'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminProviderConfigFeedback } from '../feedback/useAdminFeedback'
import {
  createEmptyChannelForm,
  inferChannelApiTypeFromUrl,
  uniqueTrimmedModelValues,
} from './providerConfigPayload'
import { createModelSuggestions } from './providerModelOptions'
import {
  createAdminProviderConfigWorkspace,
  type AdminProviderConfirmPort,
  type AdminProviderConfigWorkspace,
} from './providerConfigWorkspace'
import type {
  AdminProviderAccountLimits,
  ProviderAccountCommandTarget,
  ProviderAccountForm,
  ProviderAccountFormField,
  ProviderChannelCommandTarget,
  ProviderChannelFormData,
  ProviderChannelFormField,
  ProviderChannelRecord,
} from './providerConfigTypes'

export type UseAdminProviderConfigOptions = {
  feedback: AdminProviderConfigFeedback
  confirm: AdminProviderConfirmPort
}

export function useAdminProviderConfig(options: UseAdminProviderConfigOptions) {
  const { runAsyncAction } = useAsyncAction()
  const workspace = createAdminProviderConfigWorkspace({
    feedback: options.feedback,
    confirm: options.confirm,
    runAsyncAction,
  })

  const channelEditor = createChannelEditor({
    workspace,
    feedback: options.feedback,
  })
  const accountEditor = createAccountEditor({
    workspace,
    feedback: options.feedback,
  })

  return {
    cards: workspace.cards,
    channelEditor,
    accountEditor,
    commands: {
      loadData: workspace.loadData,
      deleteChannel: workspace.deleteChannel,
      deleteAccount: workspace.deleteAccount,
      toggleAccountModelStatus: workspace.toggleAccountModelStatus,
      getApiTypeLabel: workspace.getApiTypeLabel,
      getAccountModelRows: workspace.getAccountModelRows,
      getAccountStatusDotClass: workspace.getAccountStatusDotClass,
      getAccountProgressPt: workspace.getAccountProgressPt,
    },
  }
}

function createChannelEditor(options: {
  workspace: AdminProviderConfigWorkspace
  feedback: AdminProviderConfigFeedback
}) {
  const visible = ref(false)
  const isEditing = ref(false)
  const saveLoading = options.workspace.channelSaveLoading
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
    const row = options.workspace.resolveChannelTarget(target)
    if (!row) return

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
      options.feedback.channelFormInvalid()
      return
    }

    await options.workspace.saveChannel({
      id: isEditing.value ? currentChannelId.value : undefined,
      data: { ...form.value },
      isEditing: isEditing.value,
      afterSaved: () => {
        visible.value = false
        resetErrors()
      },
    })
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
    apiTypeOptions: options.workspace.apiTypeOptions,
    openCreate,
    openEdit,
    close,
    save,
    clearError,
    handleApiUrlBlur,
  }
}

function createAccountEditor(options: {
  workspace: AdminProviderConfigWorkspace
  feedback: AdminProviderConfigFeedback
}) {
  const visible = ref(false)
  const isEditing = ref(false)
  const saveLoading = options.workspace.accountSaveLoading
  const currentAccountId = ref(0)
  const selectedChannel = ref<ProviderChannelRecord | null>(null)
  const selectedChannelView = computed(() =>
    selectedChannel.value ? options.workspace.getChannelView(selectedChannel.value) : null,
  )
  const accountLimits = options.workspace.accountLimits
  const form = ref(createEmptyAccountForm(accountLimits.value, 0))
  const errors = ref<Record<ProviderAccountFormField, string>>(createEmptyAccountFormErrors())
  const detectModelsLoading = options.workspace.detectModelsLoading
  const detectedModelValues = ref<string[]>([])
  const modelSuggestions = ref<string[]>([])
  const detectedModelSessionCache = ref(new Set<string>())
  const hasShownApiKeyHint = ref(false)

  function openCreate(target: ProviderChannelCommandTarget) {
    const channel = options.workspace.resolveChannelTarget(target)
    if (!channel) return

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
    const account = options.workspace.resolveAccountTarget(target)
    if (!account) return

    isEditing.value = true
    currentAccountId.value = account.id
    selectedChannel.value = options.workspace.findChannelById(account.channelId)
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
      options.feedback.accountFormInvalid()
      return
    }

    const modelValues = uniqueTrimmedModelValues(form.value.modelValues)
    form.value.modelValues = modelValues

    await options.workspace.saveAccount({
      isEditing: isEditing.value,
      accountId: currentAccountId.value,
      form: form.value,
      selectedChannel: selectedChannel.value,
      modelValues,
      secret: form.value.secret.trim(),
      afterSaved: () => {
        visible.value = false
        resetErrors()
      },
    })
  }

  function searchModelSuggestions(event: { query?: string }) {
    modelSuggestions.value = createModelSuggestions({
      detectedValues: detectedModelValues.value,
      selectedValues: form.value.modelValues,
      query: event.query,
    })
  }

  async function detectModelsOnSelectOpen() {
    const result = await options.workspace.detectModels({
      channel: selectedChannel.value,
      secret: form.value.secret,
      detectedValues: detectedModelValues.value,
      sessionCache: detectedModelSessionCache.value,
      hasShownApiKeyHint: hasShownApiKeyHint.value,
    })

    detectedModelValues.value = result.detectedValues
    detectedModelSessionCache.value = result.sessionCache
    hasShownApiKeyHint.value = result.hasShownApiKeyHint
  }

  function handleModelSelectShow(show: boolean) {
    if (!show) return Promise.resolve()
    return detectModelsOnSelectOpen()
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
    detectModelsOnSelectOpen,
    handleModelSelectShow,
  }
}

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

export type AdminProviderConfigContext = ReturnType<typeof useAdminProviderConfig>
