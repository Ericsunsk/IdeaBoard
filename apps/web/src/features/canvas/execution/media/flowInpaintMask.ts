import { i18n } from '../../../../shared/i18n'
import { ensureFlowMaskImageUrl } from './flowMediaUpload'

export async function prepareFlowInpaintMask(
  rawMaskDataUrl: string,
  boardId?: string,
): Promise<string> {
  const maskDataUrl = rawMaskDataUrl.trim()
  if (!maskDataUrl) throw new Error(i18n.global.t('canvas.execution.markInpaintAreaFirst'))
  return await ensureFlowMaskImageUrl(maskDataUrl, { boardId })
}
