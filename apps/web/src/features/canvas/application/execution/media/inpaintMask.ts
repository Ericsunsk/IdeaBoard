import { i18n } from '../../../../../shared/i18n'
import { ensureMaskImageUrl } from './mediaUpload'

export async function prepareInpaintMask(
  rawMaskDataUrl: string,
  boardId?: string,
): Promise<string> {
  const maskDataUrl = rawMaskDataUrl.trim()
  if (!maskDataUrl) throw new Error(i18n.global.t('canvas.execution.markInpaintAreaFirst'))
  return await ensureMaskImageUrl(maskDataUrl, { boardId })
}
