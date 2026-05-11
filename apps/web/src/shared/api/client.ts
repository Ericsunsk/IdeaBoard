import axios, { type AxiosRequestConfig } from 'axios'
import { handleAuthFailure } from '../utils/auth'

export type ApiClientError = Error & {
  code?: number
  msg?: string
  status?: number
  data?: unknown
}

const api = axios.create({
  baseURL: '/api',
  withCredentials: true,
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json',
  },
})

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

api.interceptors.response.use(
  (response) => {
    const data = response.data
    if (data && typeof data.code === 'number' && data.code !== 0) {
      const requestUrl = response.config?.url || ''
      handleAuthFailure({ status: response.status, payload: data, requestUrl })
      return Promise.reject(createApiError(data, response.status))
    }
    return data
  },
  (error) => {
    if (error.response) {
      const requestUrl = error.config?.url || ''
      handleAuthFailure({ status: error.response.status, payload: error.response.data, requestUrl })
      return Promise.reject(createApiError(error.response.data, error.response.status))
    }
    return Promise.reject(createApiError(error))
  },
)

import type { ApiResponse } from '../types'

type AxiosConfig<D = unknown> = AxiosRequestConfig<D> | undefined

export function apiGet<T>(url: string, config?: AxiosConfig): Promise<ApiResponse<T>> {
  return api.get<unknown, ApiResponse<T>>(url, config)
}

export function apiPost<T, D = unknown>(
  url: string,
  data?: D,
  config?: AxiosConfig<D>,
): Promise<ApiResponse<T>> {
  return api.post<unknown, ApiResponse<T>, D>(url, data, config)
}

export function apiPut<T, D = unknown>(
  url: string,
  data?: D,
  config?: AxiosConfig<D>,
): Promise<ApiResponse<T>> {
  return api.put<unknown, ApiResponse<T>, D>(url, data, config)
}

export function apiDelete<T>(url: string, config?: AxiosConfig): Promise<ApiResponse<T>> {
  return api.delete<unknown, ApiResponse<T>>(url, config)
}

export { api }
export default api
