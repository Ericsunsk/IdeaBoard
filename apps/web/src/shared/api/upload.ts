import {
  deleteUserBoardUpload,
  uploadUserBoardAsset,
  uploadUserProfileAvatar,
} from './generated-query/user-assets/user-assets'
import type {
  ApiResponseDeleted,
  ApiResponseUploadAvatar,
  ApiResponseUploadImage,
  UploadAvatarMetadata,
  UploadImageMetadata,
} from './generated-query/model'

export type { UploadImageMetadata }
export type UploadAvatarResponse = UploadAvatarMetadata

export const UPLOAD_OPERATION_TIMEOUT_MS = 5 * 60 * 1000

function getUploadRequestOptions(): RequestInit {
  return { timeout: UPLOAD_OPERATION_TIMEOUT_MS } as RequestInit
}

export const uploadApi = {
  upload: (file: File, boardId: string): Promise<ApiResponseUploadImage> => {
    const normalizedBoardId = boardId.trim()
    return uploadUserBoardAsset(normalizedBoardId, { file }, getUploadRequestOptions())
  },
  recycle: (url: string, boardId: string): Promise<ApiResponseDeleted> =>
    deleteUserBoardUpload(boardId.trim(), { url }),
  uploadAvatar: (file: File): Promise<ApiResponseUploadAvatar> =>
    uploadUserProfileAvatar({ file }, getUploadRequestOptions()),
}
