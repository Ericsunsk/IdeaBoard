import { workspaceUploadApi } from '../../../../../shared/api/upload'
import { workspaceUploadIdempotencyKey } from '../../../../../shared/api/idempotencyKey'
import { i18n } from '../../../../../shared/i18n'
import { isDataUrl } from '../../../infrastructure/media/canvasMediaIo.ts'
import {
  CanvasWorkspaceTaskUnmappableError,
  CanvasWorkspaceUnavailableError,
} from '../workspace/canvasWorkspaceTaskAdapter'

// ---------------------------------------------------------------------------
// Workspace mask assets (Public Data Plane v1).
//
// Inpaint masks painted on canvas are data-URLs; the v1 task contract links
// masks by opaque `asset_...` id only (`params.mask_asset_id`), so the mask
// bytes must travel the workspace upload-session flow first:
// declare session -> stage bytes (PUT) -> complete (materialize asset).
// This helper reuses the existing `workspaceUploadApi` session transport
// (never the retired board upload); every step mints its own
// `upload:<operation>` idempotency key so retries replay instead of
// duplicating sessions.
// ---------------------------------------------------------------------------

type MaskAssetUploadTransport = Pick<
  typeof workspaceUploadApi,
  'createSession' | 'stageBytes' | 'completeSession'
>

export type PrepareInpaintMaskAssetDeps = {
  uploadTransport?: MaskAssetUploadTransport | undefined
  mintIdempotencyKey?:
    | ((operation: 'create' | 'bytes' | 'complete') => string)
    | undefined
  digestSha256Hex?: ((bytes: ArrayBuffer) => Promise<string>) | undefined
  clockNow?: (() => number) | undefined
}

async function dataUrlToMaskFile(dataUrl: string, fileName: string): Promise<File> {
  const response = await fetch(dataUrl)
  const blob = await response.blob()
  return new File([blob], fileName, { type: blob.type || 'image/png' })
}

async function digestSha256Hex(bytes: ArrayBuffer): Promise<string> {
  const subtle = globalThis.crypto?.subtle
  if (!subtle) throw new Error(i18n.global.t('common.errors.uploadFailed'))
  const digest = await subtle.digest('SHA-256', bytes)
  return [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, '0')).join('')
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
      'Canvas inpaint masks have no v1 mapping as URLs: upload the mask through the workspace upload-session flow first, then resubmit with params.mask_asset_id.',
    )
  }
  const transport = deps.uploadTransport ?? workspaceUploadApi
  const mint = deps.mintIdempotencyKey ?? ((op) => workspaceUploadIdempotencyKey(op))
  const digest = deps.digestSha256Hex ?? digestSha256Hex
  const file = await dataUrlToMaskFile(dataUrl, `flow-mask-${(deps.clockNow ?? Date.now)()}.png`)
  const bytes = await file.arrayBuffer()
  const checksumSha256 = await digest(bytes)
  const mimeType = file.type || 'image/png'
  const session = await transport.createSession(
    workspace,
    {
      filename: file.name,
      mimeType,
      sizeBytes: file.size,
      checksumSha256,
      mediaKind: 'image',
    },
    { idempotencyKey: mint('create') },
  )
  await transport.stageBytes(workspace, session.id, file, {
    contentType: mimeType,
    idempotencyKey: mint('bytes'),
  })
  const completed = await transport.completeSession(
    workspace,
    session.id,
    { checksumSha256, sizeBytes: file.size, mimeType, mediaKind: 'image' },
    { idempotencyKey: mint('complete') },
  )
  const assetId = `${completed.asset?.id ?? ''}`.trim()
  if (!assetId) throw new Error(i18n.global.t('common.errors.uploadProcessingFailed'))
  return assetId
}
