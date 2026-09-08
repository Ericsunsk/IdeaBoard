import { ref } from 'vue'
import type { AdminProviderConfigFeedback } from './useAdminProviderConfigFeedback'
import type { ProviderEndpointStore } from './providerConfigWorkspace'
import {
  createEmptyProviderEndpointForm,
  inferProviderEndpointApiTypeFromUrl,
  isModellessProviderCapability,
  isModellessProviderProtocol,
} from './providerConfigPayload'
import type {
  ProviderEndpointCommandTarget,
  ProviderEndpointFormData,
  ProviderEndpointFormField,
} from './providerConfigTypes'

export type ProviderEndpointEditorOptions = {
  store: ProviderEndpointStore
  feedback: AdminProviderConfigFeedback
}

export type ProviderEndpointEditor = ReturnType<typeof createProviderEndpointEditor>

export function createProviderEndpointEditor(options: ProviderEndpointEditorOptions) {
  const visible = ref(false)
  const isEditing = ref(false)
  const saveLoading = options.store.providerEndpointSaveLoading
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
    const row = options.store.resolveProviderEndpointTarget(target)
    if (!row) return

    isEditing.value = true
    currentProviderEndpointId.value = row.id
    form.value = {
      apiType: row.apiType,
      protocol: row.protocol,
      capability:
        isModellessProviderProtocol(row.protocol) &&
        isModellessProviderCapability(row.capabilities[0])
          ? row.capabilities[0]
          : '',
      name: row.name,
      baseUrl: row.baseUrl,
      apiKey: '',
      status: row.status,
    }
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

    await options.store.saveProviderEndpoint({
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
    for (const field of Object.keys(nextErrors) as ProviderEndpointFormField[]) {
      nextErrors[field] = PROVIDER_ENDPOINT_FIELD_VALIDATORS[field](form.value)
    }
    errors.value = nextErrors
    return !Object.values(nextErrors).some(Boolean)
  }

  function validateField(field: ProviderEndpointFormField) {
    errors.value[field] = PROVIDER_ENDPOINT_FIELD_VALIDATORS[field](form.value)
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
    apiTypeOptions: options.store.apiTypeOptions,
    protocolRegistry: options.store.protocolRegistry,
    openCreate,
    openEdit,
    close,
    save,
    clearError,
    handleApiUrlBlur,
  }
}

function createEmptyProviderEndpointFormErrors(): Record<ProviderEndpointFormField, string> {
  return {
    apiType: '',
    protocol: '',
    capability: '',
    name: '',
    baseUrl: '',
    apiKey: '',
    status: '',
  }
}

type ProviderEndpointFieldValidator = (form: ProviderEndpointFormData) => string

const PROVIDER_ENDPOINT_FIELD_VALIDATORS: Record<
  ProviderEndpointFormField,
  ProviderEndpointFieldValidator
> = {
  name: (form) => (!form.name.trim() ? '请输入供应商端点名称' : ''),
  apiType: (form) => (!form.apiType ? '请选择接口类型' : ''),
  protocol: (form) => (!form.protocol ? '请选择调用协议' : ''),
  capability: (form) =>
    isModellessProviderProtocol(form.protocol) && !form.capability ? '请选择处理功能' : '',
  apiKey: (form) =>
    isModellessProviderProtocol(form.protocol) && !form.apiKey.trim() ? '请输入 API Key' : '',
  baseUrl: (form) => resolveBaseUrlFieldError(form.baseUrl),
  status: (form) => (form.status !== 0 && form.status !== 1 ? '请选择启用状态' : ''),
}

function resolveBaseUrlFieldError(rawBaseUrl: string): string {
  const apiUrl = rawBaseUrl.trim()
  if (!apiUrl) return '请输入 Base URL地址'
  try {
    const parsed = new URL(apiUrl)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      return 'Base URL 需使用 http 或 https'
    }
  } catch {
    return '请输入有效的 Base URL'
  }
  return ''
}
