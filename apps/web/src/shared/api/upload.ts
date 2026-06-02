import { apiDelete, apiPost } from './client'
import type { ApiResponse } from '../types'

export type UploadImageMetadata = {
  url: string
  width?: number
  height?: number
  mime?: string
  ext?: string
}

export type UploadAvatarResponse = Omit<UploadImageMetadata, 'url'> & {
  avatar: string
}

export const uploadApi = {
  upload: (file: File, boardId: string): Promise<ApiResponse<UploadImageMetadata>> => {
    const form = new FormData()
    form.append('file', file)
    const normalizedBoardId = boardId.trim()
    return apiPost<UploadImageMetadata>(
      `/user/boards/${encodeURIComponent(normalizedBoardId)}/uploads`,
      { body: form },
    )
  },
  recycle: (url: string, boardId: string): Promise<ApiResponse<{ deleted: boolean }>> =>
    apiDelete<{ deleted: boolean }>(`/user/boards/${encodeURIComponent(boardId.trim())}/uploads`, {
      body: JSON.stringify({ url }),
      headers: { 'Content-Type': 'application/json' },
    }),
  uploadAvatar: (file: File): Promise<ApiResponse<UploadAvatarResponse>> => {
    const form = new FormData()
    form.append('file', file)
    return apiPost<UploadAvatarResponse>('/user/profile/avatar', { body: form })
  },
}
