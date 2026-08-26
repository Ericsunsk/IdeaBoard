import { ofetch, type FetchError, type FetchOptions } from 'ofetch'
import { handleAuthFailure } from '../utils/auth'
import { API_BASE_URL, normalizeRuntimeMediaRefs } from '../config/runtimeUrls'
import { i18n } from '../i18n'

export type ApiClientError = Error & {
  code?: number | string
  hasSafeDisplayMessage?: boolean
  msg?: string
  status?: number
  data?: unknown
}

export type ApiRequestConfig = Omit<FetchOptions<'json'>, 'baseURL' | 'params'>

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

export function createApiError(payload?: unknown, status?: number): ApiClientError {
  const data = toObject(payload)
  const safeMessage = getPayloadMessage(payload, data)
  const message = safeMessage || i18n.global.t('common.errors.requestIncomplete')
  const error = new Error(message) as ApiClientError
  error.code =
    (typeof data?.code === 'number' || typeof data?.code === 'string' ? data.code : undefined) ??
    status
  error.hasSafeDisplayMessage = Boolean(safeMessage)
  error.msg = safeMessage
  error.status = status
  error.data = data?.data
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
  try {
    return await apiFetch<T>(url, config)
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
