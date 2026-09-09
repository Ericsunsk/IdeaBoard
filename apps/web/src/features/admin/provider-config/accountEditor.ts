import { computed, ref, watch } from 'vue'
import type { AdminProviderConfigFeedback } from './useProviderConfigFeedback'
import type { ProviderAccountStore } from './workspace'
import { createModelSuggestions } from './providerModelOptions'
import { isModellessProviderEndpoint, uniqueTrimmedModelValues } from './payload'
import type {
  AdminProviderAccountLimits,
  ProviderAccountCommandTarget,
  ProviderAccountForm,
  ProviderAccountFormField,
  ProviderEndpointCommandTarget,
  ProviderEndpointRecord,
} from './types'

export type AccountEditorOptions = {
  store: ProviderAccountStore
  feedback: AdminProviderConfigFeedback
}

export type AccountEditor = ReturnType<typeof createAccountEditor>

export function createAccountEditor(options: AccountEditorOptions) {
  const visible = ref(false)
  const isEditing = ref(false)
  const saveLoading = options.store.accountSaveLoading
  const currentAccountId = ref(0)
  const selectedProviderEndpoint = ref<ProviderEndpointRecord | null>(null)
  const selectedProviderEndpointView = computed(() =>
    selectedProviderEndpoint.value
      ? options.store.getProviderEndpointView(selectedProviderEndpoint.value)
      : null,
  )
  const accountLimits = options.store.accountLimits
  const form = ref(createEmptyAccountForm(accountLimits.value, 0))
  const errors = ref<Record<ProviderAccountFormField, string>>(createEmptyAccountFormErrors())
  const detectModelsLoading = options.store.detectModelsLoading
  const detectedModelValues = ref<string[]>([])
  const modelSuggestions = ref<string[]>([])
  const modelSearchInput = ref('')
  const detectedModelSessionCache = ref(new Set<string>())
  const hasShownApiKeyHint = ref(false)

  function openCreate(target: ProviderEndpointCommandTarget) {
    const providerEndpoint = options.store.resolveProviderEndpointTarget(target)
    if (!providerEndpoint) return

    isEditing.value = false
    currentAccountId.value = 0
    selectedProviderEndpoint.value = providerEndpoint
    form.value = createEmptyAccountForm(
      accountLimits.value,
      providerEndpoint.id,
      createUniqueAccountName(
        `${providerEndpoint.name} APIKey`,
        options.store
          .getProviderEndpointAccounts(providerEndpoint.id)
          .map((account) => account.name),
      ),
    )
    resetModelEditorState()
    resetErrors()
    visible.value = true
  }

  function openEdit(target: ProviderAccountCommandTarget) {
    const account = options.store.resolveAccountTarget(target)
    if (!account) return

    isEditing.value = true
    currentAccountId.value = account.id
    selectedProviderEndpoint.value = options.store.findProviderEndpointById(
      account.providerEndpointId,
    )
    form.value = {
      providerEndpointId: account.providerEndpointId,
      name: account.name,
      modelValues: account.models.map((model) => model.name),
      modelCapabilities: Object.fromEntries(
        account.models.map((model) => [
          model.name.trim().toLowerCase(),
          { supports2K: model.supports2K, supports4K: model.supports4K },
        ]),
      ),
      secret: '',
      maxConcurrent: account.maxConcurrent,
      timeoutSeconds: account.timeoutSeconds,
    }
    resetModelEditorState()
    resetErrors()
    visible.value = true
  }

  function close() {
    visible.value = false
    resetModelEditorState()
    resetErrors()
  }

  function resetModelEditorState() {
    modelSearchInput.value = ''
    detectedModelValues.value = []
    modelSuggestions.value = []
    detectedModelSessionCache.value = new Set()
    hasShownApiKeyHint.value = false
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
    if (
      selectedModels.length === 0 &&
      !isModellessProviderEndpoint(selectedProviderEndpoint.value)
    ) {
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

    await options.store.saveAccount({
      isEditing: isEditing.value,
      accountId: currentAccountId.value,
      form: form.value,
      selectedProviderEndpoint: selectedProviderEndpoint.value,
      modelValues,
      modelCapabilities: form.value.modelCapabilities,
      secret: form.value.secret.trim(),
      afterSaved: () => {
        close()
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
    const result = await options.store.detectModels({
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
    return options.store
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

  const currentAccountTarget = computed<ProviderAccountCommandTarget | null>(() =>
    currentAccountId.value ? { kind: 'account', id: currentAccountId.value } : null,
  )

  return {
    visible,
    isEditing,
    saveLoading,
    currentAccountTarget,
    accountLimits,
    selectedProviderEndpoint: selectedProviderEndpointView,
    form,
    errors,
    detectModelsLoading,
    modelSuggestions,
    modelSearchInput,
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

function createEmptyAccountForm(
  accountLimits: AdminProviderAccountLimits,
  providerEndpointId: number,
  name = '',
): ProviderAccountForm {
  return {
    providerEndpointId,
    name,
    modelValues: [],
    modelCapabilities: {},
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

function resolveNumberRangeError(value: number, min: number, max: number, label: string) {
  if (!Number.isFinite(value)) return `请输入${label}`
  if (value < min || value > max) return `${label}需在 ${min}-${max} 之间`
  return ''
}
