import { uploadUserBoardAsset, uploadUserProfileAvatar } from './generated-query/user-assets/user-assets'
import type { UploadAvatarMetadata, UploadImageMetadata } from './generated-query/model'
import { resolveApiData } from './apiResponse'

export const UPLOAD_OPERATION_TIMEOUT_MS = 5 * 60 * 1000

function getUploadRequestOptions(): RequestInit {
  return { timeout: UPLOAD_OPERATION_TIMEOUT_MS } as RequestInit
}

export const uploadApi = {
  upload: async (file: File, boardId: string): Promise<UploadImageMetadata> => {
    const normalizedBoardId = boardId.trim()
    const data = await resolveApiData(
      uploadUserBoardAsset(normalizedBoardId, { file }, getUploadRequestOptions()),
      '上传图片失败',
    )
    if (!data.url?.trim()) throw new Error('上传图片失败')
    const mediaAssetId = Number(data.media_asset_id)
    if (!Number.isInteger(mediaAssetId) || mediaAssetId <= 0) {
      throw new Error('上传图片失败：缺少媒体资产标识')
    }
    return data
  },
  uploadAvatar: async (file: File): Promise<UploadAvatarMetadata> => {
    const data = await resolveApiData(
      uploadUserProfileAvatar({ file }, getUploadRequestOptions()),
      '头像上传失败，请稍后重试',
    )
    if (!data.avatar?.trim()) throw new Error('头像上传失败，请稍后重试')
    return data
  },
}
