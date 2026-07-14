import type { ApiResponseBase } from './generated-query/model'
import { sanitizeApiErrorMessage } from '../utils/errorMessages'

type ApiResponseWithData<T> = ApiResponseBase & {
  data?: T | null
}

export function requireApiData<T>(response: ApiResponseWithData<T>, fallback: string): T {
  if (response.code !== 0) {
    throw new Error(sanitizeApiErrorMessage(response.msg, fallback))
  }
  if (response.data == null) throw new Error(fallback)
  return response.data
}

export function assertApiOk(response: ApiResponseBase, fallback: string): void {
  if (response.code !== 0) {
    throw new Error(sanitizeApiErrorMessage(response.msg, fallback))
  }
}
