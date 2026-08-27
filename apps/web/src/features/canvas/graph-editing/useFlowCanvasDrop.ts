import {
  isSupportedUploadImageFile,
  unsupportedUploadImageMessage,
} from '../../../shared/utils/imageUploadSupport'
import {
  CANVAS_LIBRARY_IMPORT_MIME,
  readCanvasLibraryImportTransfer,
  type CanvasLibraryImportDescriptor,
} from '../../library/libraryTransfer'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'
import { i18n } from '../../../shared/i18n'
import type { CanvasImageNodeCreationResult } from './useFlowImageUploadActions'

type UseFlowCanvasDropOptions = {
  browser: {
    markDropAsCopy: (event: DragEvent) => void
    readDroppedFile: (event: DragEvent) => File | null
    readDropScreenPoint: (event: DragEvent) => { x: number; y: number }
  }
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  onWarning: (msg: string) => void
  createImageNodeFromFile: (
    point: { x: number; y: number },
    file: File,
  ) => Promise<CanvasImageNodeCreationResult | null>
  addLibraryAsset: (
    asset: CanvasLibraryImportDescriptor,
    point: { x: number; y: number },
  ) => void
}

export function useFlowCanvasDrop(options: UseFlowCanvasDropOptions) {
  function onCanvasDragOver(event: DragEvent) {
    options.browser.markDropAsCopy(event)
  }

  async function onCanvasDrop(event: DragEvent) {
    const point = options.screenToFlowCoordinate(options.browser.readDropScreenPoint(event))

    const transferTypes = Array.from(event.dataTransfer?.types ?? [])
    if (transferTypes.includes(CANVAS_LIBRARY_IMPORT_MIME)) {
      const libraryAsset = readCanvasLibraryImportTransfer(event.dataTransfer)
      if (libraryAsset) options.addLibraryAsset(libraryAsset, point)
      return
    }

    const file = options.browser.readDroppedFile(event)
    if (!file) return
    if (!isSupportedUploadImageFile(file)) {
      options.onWarning(unsupportedUploadImageMessage())
      return
    }
    try {
      const result = await options.createImageNodeFromFile(point, file)
      if (result && result.kind !== 'ready') options.onWarning(result.failureMessage)
    } catch (error) {
      options.onWarning(userFacingErrorCopy(error, i18n.global.t('canvas.session.uploadFailed')))
    }
  }

  return {
    onCanvasDragOver,
    onCanvasDrop,
  }
}
