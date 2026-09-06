import { ref } from 'vue'
import type { AdminProviderConfigFeedback } from './useAdminProviderConfigFeedback'
import type { AdminProviderConfigWorkspace } from './providerConfigWorkspace'
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
  workspace: AdminProviderConfigWorkspace
  feedback: AdminProviderConfigFeedback
}

export type ProviderEndpointEditor = ReturnType<typeof createProviderEndpointEditor>

export function createProviderEndpointEditor(options: ProviderEndpointEditorOptions) {
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
    ;(
      [
        'name',
        'apiType',
        'protocol',
        'capability',
        'baseUrl',
        'status',
      ] as ProviderEndpointFormField[]
    ).forEach((field) => {
      nextErrors[field] = resolveProviderEndpointFieldError(form.value, field)
    })
    nextErrors.apiKey = resolveProviderEndpointFieldError(form.value, 'apiKey')
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
    protocolRegistry: options.workspace.protocolRegistry,
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

function resolveProviderEndpointFieldError(
  form: ProviderEndpointFormData,
  field: ProviderEndpointFormField,
) {
  if (field === 'name' && !form.name.trim()) return '请输入供应商端点名称'
  if (field === 'apiType' && !form.apiType) return '请选择接口类型'
  if (field === 'protocol' && !form.protocol) return '请选择调用协议'
  if (field === 'capability' && isModellessProviderProtocol(form.protocol) && !form.capability) {
    return '请选择处理功能'
  }
  if (field === 'apiKey' && isModellessProviderProtocol(form.protocol) && !form.apiKey.trim()) {
    return '请输入 API Key'
  }
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
