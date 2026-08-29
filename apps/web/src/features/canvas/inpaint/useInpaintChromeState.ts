import { ref, watch } from 'vue'
import {
  clampMaskLayerOpacity,
  INPAINT_DEFAULT_LAYER_OPACITY,
} from './config/inpaintConstants'

type ValueRef<T> = {
  readonly value: T
}

type UseInpaintChromeStateOptions = {
  retouchNodeId: ValueRef<string | null>
}

export function useInpaintChromeState(options: UseInpaintChromeStateOptions) {
  const inpaintLayerOpacity = ref(INPAINT_DEFAULT_LAYER_OPACITY)

  function setInpaintLayerOpacity(value: number) {
    inpaintLayerOpacity.value = clampMaskLayerOpacity(value)
  }

  watch(
    () => options.retouchNodeId.value,
    () => {
      inpaintLayerOpacity.value = INPAINT_DEFAULT_LAYER_OPACITY
    },
  )

  return {
    inpaintLayerOpacity,
    setInpaintLayerOpacity,
  }
}
