import {
  ADMIN_UPLOAD_ENDPOINT,
  UPLOAD_FAILED_MESSAGE,
} from '../../../shared/constants/media'
import { handleAuthFailure } from '../../../shared/utils/auth'

type UploadResponse = {
  code?: number
  msg?: string
  data?: { url?: string }
}

export function parseAdminUploadResponse(event?: ProgressEvent): UploadResponse {
  const target = event?.target as XMLHttpRequest | undefined
  const status = target?.status
  if (!target?.response) {
    handleAuthFailure({ status, requestUrl: ADMIN_UPLOAD_ENDPOINT })
    return { msg: UPLOAD_FAILED_MESSAGE }
  }
  try {
    const payload = JSON.parse(target.response) as UploadResponse
    handleAuthFailure({ status, payload, requestUrl: ADMIN_UPLOAD_ENDPOINT })
    return payload
  } catch {
    handleAuthFailure({ status, requestUrl: ADMIN_UPLOAD_ENDPOINT })
    return { msg: UPLOAD_FAILED_MESSAGE }
  }
}
