import {
  deleteUserBoardUpload,
  uploadUserBoardAsset,
  uploadUserProfileAvatar,
} from './generated-query/user-assets/user-assets'
import type { UploadAvatarMetadata, UploadImageMetadata } from './generated-query/model'
import type { ApiResponse } from '../types'

export type { UploadImageMetadata }
export type UploadAvatarResponse = UploadAvatarMetadata

export const uploadApi = {
  upload: (file: File, boardId: string): Promise<ApiResponse<UploadImageMetadata>> => {
    const normalizedBoardId = boardId.trim()
    return uploadUserBoardAsset(normalizedBoardId, { file })
  },
  recycle: (url: string, boardId: string): Promise<ApiResponse<{ deleted: boolean }>> =>
    deleteUserBoardUpload(boardId.trim(), { url }),
  uploadAvatar: (file: File): Promise<ApiResponse<UploadAvatarResponse>> =>
    uploadUserProfileAvatar({ file }),
}
