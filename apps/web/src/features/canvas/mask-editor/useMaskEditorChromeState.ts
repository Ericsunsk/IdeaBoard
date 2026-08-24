import { ref, watch } from 'vue'
import {
  clampMaskLayerOpacity,
  MASK_EDITOR_DEFAULT_LAYER_OPACITY,
} from './config/maskEditorConstants'

type ValueRef<T> = {
  readonly value: T
}

type UseMaskEditorChromeStateOptions = {
  imageEditorNodeId: ValueRef<string | null>
}

export function useMaskEditorChromeState(options: UseMaskEditorChromeStateOptions) {
  const maskEditorLayerOpacity = ref(MASK_EDITOR_DEFAULT_LAYER_OPACITY)

  function setMaskEditorLayerOpacity(value: number) {
    maskEditorLayerOpacity.value = clampMaskLayerOpacity(value)
  }

  watch(
    () => options.imageEditorNodeId.value,
    () => {
      maskEditorLayerOpacity.value = MASK_EDITOR_DEFAULT_LAYER_OPACITY
    },
  )

  return {
    maskEditorLayerOpacity,
    setMaskEditorLayerOpacity,
  }
}
