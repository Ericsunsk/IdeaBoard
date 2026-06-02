import { ofetch, type FetchError, type FetchOptions } from 'ofetch'
import { handleAuthFailure } from '../utils/auth'
import { API_BASE_URL, normalizeRuntimeMediaRefs } from '../config/runtimeUrls'
import type { ApiResponse } from '../types'

export type ApiClientError = Error & {
  code?: number
  msg?: string
  status?: number
  data?: unknown
}

export type ApiRequestConfig = Omit<FetchOptions<'json'>, 'params'>

function toObject(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : null
}

export function createApiError(payload?: unknown, status?: number): ApiClientError {
  const data = toObject(payload)
  const message =
    (typeof data?.msg === 'string' && data.msg) ||
    (typeof data?.message === 'string' && data.message) ||
    '请求失败'
  const error = new Error(message) as ApiClientError
  error.code = (typeof data?.code === 'number' ? data.code : undefined) ?? status
  error.msg = (typeof data?.msg === 'string' && data.msg) || message
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
    const data = normalizeRuntimeMediaRefs(response._data)
    response._data = data
    if (data && typeof data.code === 'number' && data.code !== 0) {
      const requestUrl = response.url || ''
      handleAuthFailure({ status: response.status, payload: data, requestUrl })
      throw createApiError(data, response.status)
    }
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

export function apiGet<T>(url: string, config?: ApiRequestConfig): Promise<ApiResponse<T>> {
  return apiRequest<ApiResponse<T>>(url, { ...config, method: 'GET' })
}

export function apiPost<T>(url: string, config?: ApiRequestConfig): Promise<ApiResponse<T>> {
  return apiRequest<ApiResponse<T>>(url, { ...config, method: 'POST' })
}

export function apiPut<T>(url: string, config?: ApiRequestConfig): Promise<ApiResponse<T>> {
  return apiRequest<ApiResponse<T>>(url, { ...config, method: 'PUT' })
}

export function apiDelete<T>(url: string, config?: ApiRequestConfig): Promise<ApiResponse<T>> {
  return apiRequest<ApiResponse<T>>(url, { ...config, method: 'DELETE' })
}

export const api = {
  request: apiRequest,
  get: apiGet,
  post: apiPost,
  put: apiPut,
  delete: apiDelete,
}

export default api
