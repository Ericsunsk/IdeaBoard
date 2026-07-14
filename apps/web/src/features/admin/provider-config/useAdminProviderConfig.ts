import { computed, ref, watch } from 'vue'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminProviderConfigFeedback } from './useAdminProviderConfigFeedback'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import {
  createEmptyProviderEndpointForm,
  inferProviderEndpointApiTypeFromUrl,
  uniqueTrimmedModelValues,
} from './providerConfigPayload'
import { createModelSuggestions } from './providerModelOptions'
import {
  createAdminProviderConfigWorkspace,
  type AdminProviderConfigWorkspace,
} from './providerConfigWorkspace'
import type { AdminProviderConfigResource } from './providerConfigResource'
import type {
  AdminProviderAccountLimits,
  ProviderAccountCommandTarget,
  ProviderAccountForm,
  ProviderAccountFormField,
  ProviderEndpointCommandTarget,
  ProviderEndpointFormData,
  ProviderEndpointFormField,
  ProviderEndpointRecord,
} from './providerConfigTypes'

export type UseAdminProviderConfigOptions = {
  feedback: AdminProviderConfigFeedback
  confirm: AdminConfirmPort
  resource?: AdminProviderConfigResource
}

export function useAdminProviderConfig(options: UseAdminProviderConfigOptions) {
  const { runAsyncAction } = useAsyncAction()
  const workspace = createAdminProviderConfigWorkspace({
    feedback: options.feedback,
    confirm: options.confirm,
    runAsyncAction,
    resource: options.resource,
  })

  const providerEndpointEditor = createProviderEndpointEditor({
    workspace,
    feedback: options.feedback,
  })
  const accountEditor = createAccountEditor({
    workspace,
    feedback: options.feedback,
  })

  return {
    cards: workspace.cards,
    providerEndpointEditor,
    accountEditor,
    commands: {
      loadData: workspace.loadData,
      deleteProviderEndpoint: workspace.deleteProviderEndpoint,
      deleteAccount: workspace.deleteAccount,
      toggleAccountModelStatus: workspace.toggleAccountModelStatus,
      getApiTypeLabel: workspace.getApiTypeLabel,
      getAccountModelRows: workspace.getAccountModelRows,
      getAccountStatusDotClass: workspace.getAccountStatusDotClass,
      getAccountProgressPt: workspace.getAccountProgressPt,
    },
  }
}

function createProviderEndpointEditor(options: {
  workspace: AdminProviderConfigWorkspace
  feedback: AdminProviderConfigFeedback
}) {
  const visible = ref(false)
  const isEditing = ref(false)
  const saveLoading = options.workspace.providerEndpointSaveLoading
  const currentProviderEndpointId = ref(0)
  const form = ref<ProviderEndpointFormData>(createEmptyProviderEndpointForm())
  const errors = ref<Record<ProviderEndpointFormField, string>>(
    createEmptyProviderEndpointFormErrors(),
  )

  function openCreate() {
    isEditing.value = false
    currentProviderEndpointId.value = 0
    form.value = createEmptyProviderEndpointForm()
    resetErrors()
    visible.value = true
  }

  function openEdit(target: ProviderEndpointCommandTarget) {
    const row = options.workspace.resolveProviderEndpointTarget(target)
    if (!row) return

    isEditing.value = true
    currentProviderEndpointId.value = row.id
    form.value = { apiType: row.apiType, name: row.name, baseUrl: row.baseUrl, status: row.status }
    resetErrors()
    visible.value = true
  }

  function close() {
    visible.value = false
    resetErrors()
  }

  function inferApiTypeFromUrl() {
    form.value.apiType = inferProviderEndpointApiTypeFromUrl(form.value.baseUrl, form.value.apiType)
  }

  function handleApiUrlBlur() {
    inferApiTypeFromUrl()
    validateField('baseUrl')
  }

  async function save() {
    if (!validate()) {
      options.feedback.providerEndpointFormInvalid()
      return
    }

    await options.workspace.saveProviderEndpoint({
      id: isEditing.value ? currentProviderEndpointId.value : undefined,
      data: { ...form.value },
      isEditing: isEditing.value,
      afterSaved: () => {
        visible.value = false
        resetErrors()
      },
    })
  }

  function validate() {
    const nextErrors = createEmptyProviderEndpointFormErrors()
    ;(['name', 'apiType', 'baseUrl', 'status'] as ProviderEndpointFormField[]).forEach((field) => {
      nextErrors[field] = resolveProviderEndpointFieldError(form.value, field)
    })
    errors.value = nextErrors
    return !Object.values(nextErrors).some(Boolean)
  }

  function validateField(field: ProviderEndpointFormField) {
    errors.value[field] = resolveProviderEndpointFieldError(form.value, field)
    return !errors.value[field]
  }

  function clearError(field: ProviderEndpointFormField) {
    if (errors.value[field]) {
      errors.value[field] = ''
    }
  }

  function resetErrors() {
    errors.value = createEmptyProviderEndpointFormErrors()
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
  const selectedProviderEndpoint = ref<ProviderEndpointRecord | null>(null)
  const selectedProviderEndpointView = computed(() =>
    selectedProviderEndpoint.value
      ? options.workspace.getProviderEndpointView(selectedProviderEndpoint.value)
      : null,
  )
  const accountLimits = options.workspace.accountLimits
  const form = ref(createEmptyAccountForm(accountLimits.value, 0))
  const errors = ref<Record<ProviderAccountFormField, string>>(createEmptyAccountFormErrors())
  const detectModelsLoading = options.workspace.detectModelsLoading
  const detectedModelValues = ref<string[]>([])
  const modelSuggestions = ref<string[]>([])
  const detectedModelSessionCache = ref(new Set<string>())
  const hasShownApiKeyHint = ref(false)

  function openCreate(target: ProviderEndpointCommandTarget) {
    const providerEndpoint = options.workspace.resolveProviderEndpointTarget(target)
    if (!providerEndpoint) return

    isEditing.value = false
    currentAccountId.value = 0
    selectedProviderEndpoint.value = providerEndpoint
    form.value = createEmptyAccountForm(
      accountLimits.value,
      providerEndpoint.id,
      createUniqueAccountName(
        `${providerEndpoint.name} APIKey`,
        options.workspace
          .getProviderEndpointAccounts(providerEndpoint.id)
          .map((account) => account.name),
      ),
    )
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
    selectedProviderEndpoint.value = options.workspace.findProviderEndpointById(
      account.providerEndpointId,
    )
    form.value = {
      providerEndpointId: account.providerEndpointId,
      name: account.name,
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
    const accountName = form.value.name.trim()
    const selectedModels = uniqueTrimmedModelValues(form.value.modelValues)
    if (!accountName) {
      nextErrors.name = '请输入 APIKey 名称'
    } else if (hasDuplicateAccountName(accountName)) {
      nextErrors.name = '同一端点下已存在同名 APIKey'
    }
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
    form.value.name = form.value.name.trim()
    form.value.modelValues = modelValues

    await options.workspace.saveAccount({
      isEditing: isEditing.value,
      accountId: currentAccountId.value,
      form: form.value,
      selectedProviderEndpoint: selectedProviderEndpoint.value,
      modelValues,
      secret: form.value.secret.trim(),
      afterSaved: () => {
        visible.value = false
        resetErrors()
      },
    })
  }

  function refreshModelSuggestions(query?: string) {
    modelSuggestions.value = createModelSuggestions({
      detectedValues: detectedModelValues.value,
      selectedValues: form.value.modelValues,
      query,
    })
  }

  function searchModelSuggestions(event: { query?: string }) {
    refreshModelSuggestions(event.query)
  }

  async function detectModelsOnSelectOpen() {
    const result = await options.workspace.detectModels({
      providerEndpoint: selectedProviderEndpoint.value,
      secret: form.value.secret,
      detectedValues: detectedModelValues.value,
      sessionCache: detectedModelSessionCache.value,
      hasShownApiKeyHint: hasShownApiKeyHint.value,
    })

    detectedModelValues.value = result.detectedValues
    detectedModelSessionCache.value = result.sessionCache
    hasShownApiKeyHint.value = result.hasShownApiKeyHint
    refreshModelSuggestions()
  }

  function handleModelSelectShow(show: boolean) {
    if (!show) return Promise.resolve()
    return detectModelsOnSelectOpen()
  }

  function hasDuplicateAccountName(accountName: string) {
    const normalizedName = accountName.toLowerCase()
    return options.workspace
      .getProviderEndpointAccounts(form.value.providerEndpointId)
      .some(
        (account) =>
          account.id !== currentAccountId.value &&
          account.name.trim().toLowerCase() === normalizedName,
      )
  }

  watch(visible, (nextVisible) => {
    if (!nextVisible) resetErrors()
  })

  return {
    visible,
    isEditing,
    saveLoading,
    accountLimits,
    selectedProviderEndpoint: selectedProviderEndpointView,
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

function createEmptyProviderEndpointFormErrors(): Record<ProviderEndpointFormField, string> {
  return {
    apiType: '',
    name: '',
    baseUrl: '',
    status: '',
  }
}

function createEmptyAccountForm(
  accountLimits: AdminProviderAccountLimits,
  providerEndpointId: number,
  name = '',
): ProviderAccountForm {
  return {
    providerEndpointId,
    name,
    modelValues: [],
    secret: '',
    maxConcurrent: accountLimits.maxConcurrent.default,
    timeoutSeconds: accountLimits.timeoutSeconds.default,
  }
}

function createEmptyAccountFormErrors(): Record<ProviderAccountFormField, string> {
  return {
    name: '',
    secret: '',
    modelValues: '',
    maxConcurrent: '',
    timeoutSeconds: '',
  }
}

function createUniqueAccountName(baseName: string, existingNames: string[]) {
  const normalizedExistingNames = new Set(
    existingNames.map((name) => name.trim().toLowerCase()).filter(Boolean),
  )
  const trimmedBaseName = baseName.trim() || 'APIKey'
  if (!normalizedExistingNames.has(trimmedBaseName.toLowerCase())) return trimmedBaseName

  let suffix = 2
  while (normalizedExistingNames.has(`${trimmedBaseName} ${suffix}`.toLowerCase())) {
    suffix += 1
  }
  return `${trimmedBaseName} ${suffix}`
}

function resolveProviderEndpointFieldError(
  form: ProviderEndpointFormData,
  field: ProviderEndpointFormField,
) {
  if (field === 'name' && !form.name.trim()) return '请输入供应商端点名称'
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
  if (field === 'status' && form.status !== 0 && form.status !== 1) {
    return '请选择启用状态'
  }
  return ''
}

function resolveNumberRangeError(value: number, min: number, max: number, label: string) {
  if (!Number.isFinite(value)) return `请输入${label}`
  if (value < min || value > max) return `${label}需在 ${min}-${max} 之间`
  return ''
}
