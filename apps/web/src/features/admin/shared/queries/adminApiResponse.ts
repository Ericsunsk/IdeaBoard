import type { ApiResponse } from '../../../../shared/types'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'

export function requireApiData<T>(res: ApiResponse<T>, fallback: string): T {
  if (res.code !== 0 || res.data == null) {
    throw new Error(sanitizeApiErrorMessage(res.msg, fallback))
  }
  return res.data
}

export function assertApiOk(res: ApiResponse, fallback: string): void {
  if (res.code !== 0) {
    throw new Error(sanitizeApiErrorMessage(res.msg, fallback))
  }
}
