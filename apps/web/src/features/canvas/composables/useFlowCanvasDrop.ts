import {
  isSupportedUploadImageFile,
  unsupportedUploadImageMessage,
} from '../../../shared/utils/imageUploadSupport'
import {
  CANVAS_LIBRARY_IMPORT_MIME,
  readCanvasLibraryImportTransfer,
  type CanvasLibraryImportDescriptor,
} from '../../library/libraryTransfer'
import type { CanvasGraphNodeIdForKind } from '../graph-runtime'

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
  ) => Promise<CanvasGraphNodeIdForKind<'image'> | null>
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
      await options.createImageNodeFromFile(point, file)
    } catch (error) {
      const msg = error instanceof Error ? error.message : '上传失败'
      options.onWarning(msg)
    }
  }

  return {
    onCanvasDragOver,
    onCanvasDrop,
  }
}
