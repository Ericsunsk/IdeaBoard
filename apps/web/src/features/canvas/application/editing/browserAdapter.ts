import { IMAGE_UPLOAD_ACCEPT } from '../../../../core/constants/media'

export type CanvasGraphEditingBrowserAdapter = {
  pickImageFile: () => Promise<File | null>
  readDroppedFile: (event: DragEvent) => File | null
  readDropScreenPoint: (event: DragEvent) => { x: number; y: number }
  markDropAsCopy: (event: DragEvent) => void
}

export function createCanvasGraphEditingBrowserAdapter(
  ownerDocument: Document = document,
): CanvasGraphEditingBrowserAdapter {
  return {
    pickImageFile: () =>
      new Promise((resolve) => {
        const input = ownerDocument.createElement('input')
        input.type = 'file'
        input.accept = IMAGE_UPLOAD_ACCEPT
        input.addEventListener('change', () => resolve(input.files?.[0] ?? null), { once: true })
        input.addEventListener('cancel', () => resolve(null), { once: true })
        input.click()
      }),
    readDroppedFile: (event) => event.dataTransfer?.files?.[0] ?? null,
    readDropScreenPoint: (event) => ({ x: event.clientX, y: event.clientY }),
    markDropAsCopy: (event) => {
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
    },
  }
}
