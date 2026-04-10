import { computed } from 'vue'

import {
  ADMIN_UPLOAD_ENDPOINT,
  UPLOAD_FAILED_MESSAGE,
} from '../constants/media'

type UploadResponse = {
  code?: number
  msg?: string
  data?: { url?: string }
}

export const adminUploadAction = ADMIN_UPLOAD_ENDPOINT

export function useAdminUploadHeaders() {
  return computed(() => ({ Authorization: `Bearer ${localStorage.getItem('token') || ''}` }))
}

export function parseAdminUploadResponse(event?: ProgressEvent): UploadResponse {
  const target = event?.target as XMLHttpRequest | undefined
  if (!target?.response) {
    return { msg: UPLOAD_FAILED_MESSAGE }
  }
  try {
    return JSON.parse(target.response) as UploadResponse
  } catch {
    return { msg: UPLOAD_FAILED_MESSAGE }
  }
}
