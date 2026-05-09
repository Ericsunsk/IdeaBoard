import { uploadApi } from '../../../shared/api'
import { isDataUrl } from './flowMediaIO'

interface EnsureFlowMaskImageUrlOptions {
  boardId?: string
}


export async function dataUrlToFile(dataUrl: string, fileName: string): Promise<File> {
  const response = await fetch(dataUrl)
  const blob = await response.blob()
  return new File([blob], fileName, { type: blob.type || 'image/png' })
}

export async function ensureFlowMaskImageUrl(
  rawMaskUrl: string,
  options: EnsureFlowMaskImageUrlOptions = {},
): Promise<string> {
  const next = rawMaskUrl.trim()
  if (!next) return ''
  if (!isDataUrl(next)) return next

  const file = await dataUrlToFile(next, `flow-mask-${Date.now()}.png`)
  const uploadRes = await uploadApi.upload(file, options.boardId)
  if (uploadRes.code !== 0 || !uploadRes.data?.url) {
    throw new Error(uploadRes.msg || '蒙版上传失败')
  }
  return uploadRes.data.url
}
