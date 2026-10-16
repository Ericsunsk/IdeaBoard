import { prepareImageAssetForUpload } from '../../../../core/media/imageAssetPipeline'
import { i18n } from '../../../../core/i18n'
import type { LibrarySaveSourceInput } from '../../../library'
import {
  fetchImageBlobFromUrl,
  filenameFromImageUrl,
  resolveFetchUrl,
} from '../media/canvasMediaIo'

export type CanvasImageSaveSource = {
  displayUrl: string
  mediaAssetId?: string | undefined
  title?: string | undefined
}

export type CanvasImageSaveAdapterOptions = {
  fetchBytes?: ((url: string) => Promise<Blob>) | undefined
}

function defaultFetchBytes(url: string): Promise<Blob> {
  const fetchUrl = resolveFetchUrl(url)
  if (!fetchUrl)
    throw new Error(i18n.global.t('canvas.session.downloadImageFailed', { reason: url }))
  return fetchImageBlobFromUrl(fetchUrl)
}

export function createCanvasImageSaveInputs(
  sources: CanvasImageSaveSource[],
  options: CanvasImageSaveAdapterOptions = {},
): LibrarySaveSourceInput[] {
  const fetchBytes = options.fetchBytes ?? defaultFetchBytes
  return sources.map((source) => ({
    source: 'media',
    ...(source.title == null ? {} : { title: source.title }),
    ...(source.mediaAssetId
      ? { mediaAssetId: source.mediaAssetId }
      : { file: () => prepareSourceFile(source, fetchBytes) }),
  }))
}

async function prepareSourceFile(
  source: CanvasImageSaveSource,
  fetchBytes: (url: string) => Promise<Blob>,
): Promise<File> {
  const blob = await fetchBytes(source.displayUrl)
  if (!blob.size) throw new Error('empty response')
  const file = new File(
    [blob],
    filenameFromImageUrl(source.displayUrl, blob.type || 'image/jpeg'),
    {
      type: blob.type || 'image/jpeg',
    },
  )
  const prepared = await prepareImageAssetForUpload(file, {
    messages: {
      unknownError: i18n.global.t('canvas.session.unknownError'),
      readImageSizeSkipCompression: i18n.global.t('canvas.session.readImageSizeSkipCompression'),
      compressFallbackOriginal: i18n.global.t('canvas.session.compressFallbackOriginal'),
    },
  })
  return prepared.uploadFile
}
