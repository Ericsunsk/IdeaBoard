import { computed } from 'vue'
import type { ImageEditorModeState } from '../image-editor'

type ValueRef<T> = {
  readonly value: T
}

export type ImageToolInpaintState = ImageEditorModeState & { mode: 'inpaint' }

export function createImageToolMaskOverlayState(
  imageEditorState: ValueRef<ImageEditorModeState | null>,
) {
  return computed(() => {
    const state = imageEditorState.value
    return state?.mode === 'inpaint' ? (state as ImageToolInpaintState) : null
  })
}
