import { computed } from 'vue'
import type { RetouchModeState } from '../retouch'

type ValueRef<T> = {
  readonly value: T
}

type ImageToolInpaintState = RetouchModeState & { mode: 'inpaint' }

export function createImageToolMaskOverlayState(retouchState: ValueRef<RetouchModeState | null>) {
  return computed(() => {
    const state = retouchState.value
    return state?.mode === 'inpaint' ? (state as ImageToolInpaintState) : null
  })
}
