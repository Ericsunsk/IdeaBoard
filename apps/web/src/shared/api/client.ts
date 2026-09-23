import { ofetch, type FetchError, type FetchOptions } from 'ofetch'
import { handleAuthFailure } from '../utils/auth'
import { API_BASE_URL, normalizeRuntimeMediaRefs } from '../config/runtimeUrls'
import { i18n } from '../i18n'

export type ApiClientError = Error & {
  code?: number | string | undefined
  hasSafeDisplayMessage?: boolean | undefined
  msg?: string | undefined
  status?: number | undefined
  data?: unknown
  // Contract envelope passthrough (issue #803): v1 error bodies carry
  // `{ error: { code, message, details, request_id } }`. Additive only;
  // legacy `{ code, msg, data }` handling below is untouched.
  requestId?: string | undefined
  details?: unknown
}

export type ApiRequestConfig = Omit<FetchOptions<'json'>, 'baseURL' | 'params' | 'signal'> & {
  // Callers forward optional AbortSignals; undefined is equivalent to absent
  // (ofetch/fetch ignore it) and is never read back as defined here.
  signal?: AbortSignal | null | undefined
}

function toObject(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : null
}

function getPayloadMessage(payload: unknown, data: Record<string, unknown> | null): string {
  if (data?.message_safe !== true) return ''
  if (typeof data?.msg === 'string' && data.msg) return data.msg
  if (typeof data?.message === 'string' && data.message) return data.message
  if (payload instanceof Error && payload.message) return payload.message
  if (typeof payload === 'string' && payload.trim()) return payload.trim()
  return ''
}

type ContractErrorFields = {
  code?: string | undefined
  message?: string | undefined
  details?: unknown
  requestId?: string | undefined
}

// getContractErrorFields reads the v1 `{ error: { code, message, details,
// request_id } }` envelope (issue #803). It returns undefined when the
// payload carries no contract error. Contract messages are user-facing by
// kernel design, hence safe to display.
function getContractErrorFields(data: Record<string, unknown> | null): ContractErrorFields | undefined {
  const nested = toObject(data?.error)
  if (typeof nested?.code !== 'string' || !nested.code) return undefined
  const fields: ContractErrorFields = { code: nested.code }
  if (typeof nested.message === 'string' && nested.message) fields.message = nested.message
  fields.details = nested.details
  if (typeof nested.request_id === 'string' && nested.request_id) {
    fields.requestId = nested.request_id
  }
  return fields
}

export function createApiError(payload?: unknown, status?: number): ApiClientError {
  const data = toObject(payload)
  const safeMessage = getPayloadMessage(payload, data)
  // The contract branch only applies when the legacy top-level `code`
  // is absent, so existing `{ code, msg, data }` payloads keep their
  // exact behavior.
  const legacyCode =
    typeof data?.code === 'number' || typeof data?.code === 'string' ? data.code : undefined
  const contractError = legacyCode == null ? getContractErrorFields(data) : undefined
  return buildApiError(data, status, safeMessage, legacyCode, contractError)
}

function buildApiError(
  data: Record<string, unknown> | null,
  status: number | undefined,
  safeMessage: string,
  legacyCode: number | string | undefined,
  contractError: ContractErrorFields | undefined,
): ApiClientError {
  const displayMessage = safeMessage || contractError?.message || ''
  const message = displayMessage || i18n.global.t('common.errors.requestIncomplete')
  const error = new Error(message) as ApiClientError
  error.code = legacyCode ?? contractError?.code ?? status
  error.hasSafeDisplayMessage = Boolean(displayMessage)
  error.msg = displayMessage
  error.status = status
  error.data = data?.data
  if (contractError != null) {
    error.details = contractError.details
    error.requestId = contractError.requestId
  }
  return error
}

function requestURL(request: unknown): string {
  if (typeof request === 'string') return request
  if (request instanceof URL) return request.toString()
  if (request instanceof Request) return request.url
  return ''
}

const apiFetch = ofetch.create({
  baseURL: API_BASE_URL,
  credentials: 'include',
  retry: false,
  timeout: 10000,
  onResponse({ response }) {
    response._data = normalizeRuntimeMediaRefs(response._data)
  },
  onResponseError({ request, response }) {
    const payload = response._data
    const requestUrl = response.url || requestURL(request)
    handleAuthFailure({ status: response.status, payload, requestUrl })
    throw createApiError(payload, response.status)
  },
})

async function apiRequest<T>(url: string, config?: ApiRequestConfig): Promise<T> {
  // Normalize here so every caller can forward optional signals: fetch
  // treats undefined identically to absent.
  const { signal, ...rest } = config ?? {}
  const normalizedConfig = {
    ...rest,
    ...(signal == null ? {} : { signal }),
  }
  try {
    return await apiFetch<T>(url, normalizedConfig)
  } catch (error) {
    if (error && typeof error === 'object' && 'status' in error && 'msg' in error) {
      throw error
    }
    const fetchError = error as FetchError<unknown>
    if (fetchError?.response) {
      const payload = fetchError.data ?? fetchError.response._data
      const requestUrl = fetchError.response.url || requestURL(fetchError.request)
      handleAuthFailure({ status: fetchError.response.status, payload, requestUrl })
      throw createApiError(payload, fetchError.response.status)
    }
    throw createApiError(error)
  }
}

export const api = {
  request: apiRequest,
}

export default api
