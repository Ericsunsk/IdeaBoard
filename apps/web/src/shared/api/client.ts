import axios from 'axios'
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

export function createApiError(payload?: any, status?: number): ApiClientError {
  const message = payload?.msg || payload?.message || '请求失败'
  const error = new Error(message) as ApiClientError
  error.code = payload?.code ?? status
  error.msg = payload?.msg || message
  error.status = status
  error.data = payload?.data
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

export default api
