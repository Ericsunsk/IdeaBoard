import { workspaceUploadApi } from '@/api/media/upload'
import { i18n } from '../../../../../core/i18n'
import { isDataUrl } from '../../../session/media/canvasMediaIo.ts'
import {
  CanvasWorkspaceTaskUnmappableError,
  CanvasWorkspaceUnavailableError,
} from '../workspace/canvasWorkspaceTaskAdapter'

// Inpaint masks are uploaded as assets before submission via params.mask_asset_id.
type MaskAssetUploadTransport = Pick<typeof workspaceUploadApi, 'uploadDirect'>

export type PrepareInpaintMaskAssetDeps = {
  uploadTransport?: MaskAssetUploadTransport | undefined
  clockNow?: (() => number) | undefined
}

async function dataUrlToMaskFile(dataUrl: string, fileName: string): Promise<File> {
  const response = await fetch(dataUrl)
  const blob = await response.blob()
  return new File([blob], fileName, { type: blob.type || 'image/png' })
}

export async function prepareInpaintMaskAsset(
  rawMaskDataUrl: string,
  workspaceId: string,
  deps: PrepareInpaintMaskAssetDeps = {},
): Promise<string> {
  const dataUrl = `${rawMaskDataUrl ?? ''}`.trim()
  if (!dataUrl) throw new Error(i18n.global.t('canvas.execution.markInpaintAreaFirst'))
  const workspace = `${workspaceId ?? ''}`.trim()
  if (!workspace) throw new CanvasWorkspaceUnavailableError()
  if (!isDataUrl(dataUrl)) {
    throw new CanvasWorkspaceTaskUnmappableError(
      'media-urls',
      'Canvas inpaint masks have no v1 mapping as URLs: upload the mask through the workspace direct upload flow first, then resubmit with params.mask_asset_id.',
    )
  }
  const transport = deps.uploadTransport ?? workspaceUploadApi
  const file = await dataUrlToMaskFile(dataUrl, `flow-mask-${(deps.clockNow ?? Date.now)()}.png`)

  const asset = await transport.uploadDirect(workspace, file)
  return asset.id
}
