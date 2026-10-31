import type { ElementRefTarget } from '@/core/vue/refs'
import type { ImageStrengthAction } from '../../../session/execution'

export type ImageActionStrengthControlHandlers = {
  getValue: (action: ImageStrengthAction) => number
  setRangeRef: (action: ImageStrengthAction, el: ElementRefTarget) => void
  setTriggerRef: (action: ImageStrengthAction, el: ElementRefTarget) => void
  onRangePointerDown: (action: ImageStrengthAction, event: PointerEvent) => void
  onInput: (action: ImageStrengthAction, event: Event) => void
  onChange: (action: ImageStrengthAction, event: Event) => void
  onPointerUp: (action: ImageStrengthAction, event: PointerEvent) => void
  onClick: (action: ImageStrengthAction, event: MouseEvent) => void
}

export type ImageActionStrengthControlSource = ImageActionStrengthControlHandlers & {
  activeAction: { readonly value: ImageStrengthAction | null }
}
