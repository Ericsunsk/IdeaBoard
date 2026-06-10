import { deleteUserBoardUpload } from './generated-query/user-assets/user-assets'
import type { UploadAvatarMetadata, UploadImageMetadata } from './generated-query/model'
import type { ApiResponse } from '../types'
import { api } from './client'

export type { UploadImageMetadata }
export type UploadAvatarResponse = UploadAvatarMetadata

export const UPLOAD_OPERATION_TIMEOUT_MS = 5 * 60 * 1000

function createUploadFormData(file: File): FormData {
  const formData = new FormData()
  formData.append('file', file)
  return formData
}

export const uploadApi = {
  upload: (file: File, boardId: string): Promise<ApiResponse<UploadImageMetadata>> => {
    const normalizedBoardId = boardId.trim()
    return api.request<ApiResponse<UploadImageMetadata>>(
      `/user/boards/${encodeURIComponent(normalizedBoardId)}/uploads`,
      {
        method: 'POST',
        body: createUploadFormData(file),
        timeout: UPLOAD_OPERATION_TIMEOUT_MS,
      },
    )
  },
  recycle: (url: string, boardId: string): Promise<ApiResponse<{ deleted: boolean }>> =>
    deleteUserBoardUpload(boardId.trim(), { url }),
  uploadAvatar: (file: File): Promise<ApiResponse<UploadAvatarResponse>> =>
    api.request<ApiResponse<UploadAvatarResponse>>('/user/profile/avatar', {
      method: 'POST',
      body: createUploadFormData(file),
      timeout: UPLOAD_OPERATION_TIMEOUT_MS,
    }),
}
