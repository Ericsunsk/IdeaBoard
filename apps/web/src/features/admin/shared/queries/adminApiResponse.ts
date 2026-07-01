import type { ApiResponseBase } from '../../../../shared/api/generated-query/model'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'

type ApiResponseWithData<T> = ApiResponseBase & {
  data?: T
}

export function requireApiData<T>(res: ApiResponseWithData<T>, fallback: string): T {
  if (res.code !== 0 || res.data == null) {
    throw new Error(sanitizeApiErrorMessage(res.msg, fallback))
  }
  return res.data
}

export function assertApiOk(res: ApiResponseBase, fallback: string): void {
  if (res.code !== 0) {
    throw new Error(sanitizeApiErrorMessage(res.msg, fallback))
  }
}
