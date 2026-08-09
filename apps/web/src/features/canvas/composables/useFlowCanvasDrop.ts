import {
  isSupportedUploadImageFile,
  unsupportedUploadImageMessage,
} from '../../../shared/utils/imageUploadSupport'
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
}

export function useFlowCanvasDrop(options: UseFlowCanvasDropOptions) {
  function onCanvasDragOver(event: DragEvent) {
    options.browser.markDropAsCopy(event)
  }

  async function onCanvasDrop(event: DragEvent) {
    const point = options.screenToFlowCoordinate(options.browser.readDropScreenPoint(event))
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
