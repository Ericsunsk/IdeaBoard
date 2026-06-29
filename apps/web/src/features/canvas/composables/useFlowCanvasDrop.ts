import {
  isSupportedUploadImageFile,
  unsupportedUploadImageMessage,
} from '../../../shared/utils/imageUploadSupport'

type UseFlowCanvasDropOptions = {
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  onWarning: (msg: string) => void
  createImageNodeFromFile: (point: { x: number; y: number }, file: File) => Promise<string | null>
}

export function useFlowCanvasDrop(options: UseFlowCanvasDropOptions) {
  function onCanvasDragOver(event: DragEvent) {
    if (!event.dataTransfer) return
    event.dataTransfer.dropEffect = 'copy'
  }

  async function onCanvasDrop(event: DragEvent) {
    const point = options.screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
    const file = event.dataTransfer?.files?.[0]
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
