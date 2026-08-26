import { uploadUserBoardAsset, uploadUserProfileAvatar } from './generated-query/user-assets/user-assets'
import type { UploadAvatarMetadata, UploadImageMetadata } from './generated-query/model'
import { resolveApiData } from './apiResponse'
import { i18n } from '../i18n'

export const UPLOAD_OPERATION_TIMEOUT_MS = 5 * 60 * 1000

function getUploadRequestOptions(): RequestInit {
  return { timeout: UPLOAD_OPERATION_TIMEOUT_MS } as RequestInit
}

export const uploadApi = {
  upload: async (file: File, boardId: string): Promise<UploadImageMetadata> => {
    const normalizedBoardId = boardId.trim()
    const data = await resolveApiData(
      uploadUserBoardAsset(normalizedBoardId, { file }, getUploadRequestOptions()),
      i18n.global.t('common.errors.uploadFailed'),
    )
    if (!data.url?.trim()) throw new Error(i18n.global.t('common.errors.uploadFailed'))
    const mediaAssetId = Number(data.media_asset_id)
    if (!Number.isInteger(mediaAssetId) || mediaAssetId <= 0) {
      throw new Error(i18n.global.t('common.errors.uploadProcessingFailed'))
    }
    return data
  },
  uploadAvatar: async (file: File): Promise<UploadAvatarMetadata> => {
    const data = await resolveApiData(
      uploadUserProfileAvatar({ file }, getUploadRequestOptions()),
      i18n.global.t('common.errors.avatarUploadFailed'),
    )
    if (!data.avatar?.trim()) throw new Error(i18n.global.t('common.errors.avatarUploadFailed'))
    return data
  },
}
