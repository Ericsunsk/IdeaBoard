import { uploadApi } from '../../../../shared/api/upload'
import { i18n } from '../../../../shared/i18n'
import { isDataUrl } from '../../services/canvasMediaIO'

interface EnsureFlowMaskImageUrlOptions {
  boardId?: string | undefined
}

async function dataUrlToFile(dataUrl: string, fileName: string): Promise<File> {
  const response = await fetch(dataUrl)
  const blob = await response.blob()
  return new File([blob], fileName, { type: blob.type || 'image/png' })
}

export async function ensureMaskImageUrl(
  rawMaskUrl: string,
  options: EnsureFlowMaskImageUrlOptions = {},
): Promise<string> {
  const next = rawMaskUrl.trim()
  if (!next) return ''
  if (!isDataUrl(next)) return next

  const boardId = options.boardId?.trim() ?? ''
  if (!boardId) {
    throw new Error(i18n.global.t('canvas.execution.missingBoardIdMask'))
  }
  const file = await dataUrlToFile(next, `flow-mask-${Date.now()}.png`)
  return (await uploadApi.upload(file, boardId)).url
}
