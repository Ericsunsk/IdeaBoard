import type { ComponentPublicInstance } from 'vue'
import type { ImageStrengthAction } from '../execution'

export type ImageActionStrengthControlHandlers = {
  getValue: (action: ImageStrengthAction) => number
  setRangeRef: (action: ImageStrengthAction, el: Element | ComponentPublicInstance | null) => void
  setTriggerRef: (action: ImageStrengthAction, el: Element | ComponentPublicInstance | null) => void
  onRangePointerDown: (action: ImageStrengthAction, event: PointerEvent) => void
  onInput: (action: ImageStrengthAction, event: Event) => void
  onChange: (action: ImageStrengthAction, event: Event) => void
  onPointerUp: (action: ImageStrengthAction, event: PointerEvent) => void
  onClick: (action: ImageStrengthAction, event: MouseEvent) => void
}

export type ImageActionStrengthControlSource = ImageActionStrengthControlHandlers & {
  activeAction: { readonly value: ImageStrengthAction | null }
}
