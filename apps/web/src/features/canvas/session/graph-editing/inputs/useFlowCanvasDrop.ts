import {
  isSupportedUploadImageFile,
  unsupportedUploadImageMessage,
} from '../../../../../core/utils/imageUploadSupport'
import { userFacingErrorCopy } from '../../../../../core/errors/errorCatalog'
import { i18n } from '../../../../../core/i18n'
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
