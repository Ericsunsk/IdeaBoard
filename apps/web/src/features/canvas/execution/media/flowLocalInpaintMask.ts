import { ensureFlowMaskImageUrl } from './flowMediaUpload'

export async function prepareFlowLocalInpaintMask(
  rawMaskDataUrl: string,
  boardId?: string,
): Promise<string> {
  const maskDataUrl = rawMaskDataUrl.trim()
  if (!maskDataUrl) throw new Error('请先标记局部重绘区域')
  return await ensureFlowMaskImageUrl(maskDataUrl, { boardId })
}
