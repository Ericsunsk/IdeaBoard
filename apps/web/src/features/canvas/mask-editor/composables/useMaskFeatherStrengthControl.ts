import { onLongPress } from '@vueuse/core'
import { getCurrentInstance, onBeforeUnmount, ref, type ComponentPublicInstance } from 'vue'
import {
  clampMaskFeatherStrength,
  MASK_EDITOR_DEFAULT_FEATHER_STRENGTH,
} from '../config/maskEditorConstants'

const DEFAULT_LONG_PRESS_MS = 260

type FeatherRangeRefTarget = Element | ComponentPublicInstance | null

type UseMaskFeatherStrengthControlOptions = {
  onApply: (strength: number) => void
  longPressMs?: number
}

export function useMaskFeatherStrengthControl(options: UseMaskFeatherStrengthControlOptions) {
  const strength = ref(MASK_EDITOR_DEFAULT_FEATHER_STRENGTH)
  const visible = ref(false)
  const rangeRef = ref<HTMLInputElement | null>(null)
  const longPressMs = options.longPressMs ?? DEFAULT_LONG_PRESS_MS

  let stopLongPress: (() => void) | null = null
  let isEditing = false
  let shouldSuppressNextClick = false
  let longPressGeneration = 0

  function getRangeInput(event: Event): HTMLInputElement | null {
    return event.currentTarget instanceof HTMLInputElement ? event.currentTarget : null
  }

  function syncStrengthFromRange(input: HTMLInputElement) {
    strength.value = clampMaskFeatherStrength(input.valueAsNumber)
  }

  function beginStrengthEdit(input: HTMLInputElement, options: { syncInitial: boolean }) {
    if (!options.syncInitial) input.value = `${strength.value}`
    visible.value = true
    isEditing = true
    shouldSuppressNextClick = true
    if (options.syncInitial) syncStrengthFromRange(input)
    input.focus({ preventScroll: true })
  }

  function finishStrengthEdit() {
    if (!isEditing) return
    isEditing = false
    visible.value = false
    rangeRef.value?.blur()
    options.onApply(strength.value)
  }

  function disposeLongPress() {
    stopLongPress?.()
    stopLongPress = null
  }

  function setRangeRef(el: FeatherRangeRefTarget) {
    const input =
      el instanceof HTMLInputElement
        ? el
        : el && '$el' in el && el.$el instanceof HTMLInputElement
          ? el.$el
          : null
    if (rangeRef.value === input) return
    disposeLongPress()
    rangeRef.value = input
    if (!input) return
    const generation = longPressGeneration
    stopLongPress = onLongPress(
      input,
      () => {
        if (generation !== longPressGeneration) return
        beginStrengthEdit(input, { syncInitial: false })
      },
      {
        delay: longPressMs,
        distanceThreshold: false,
      },
    )
  }

  function onButtonPointerDown(event: PointerEvent) {
    if (event.button !== 0) return
    const input = getRangeInput(event)
    if (!input) return
    if (visible.value) {
      beginStrengthEdit(input, { syncInitial: true })
      return
    }
    isEditing = false
    shouldSuppressNextClick = false
  }

  function onButtonClick(event?: MouseEvent) {
    const input = event?.currentTarget instanceof HTMLInputElement ? event.currentTarget : null
    if (shouldSuppressNextClick) {
      shouldSuppressNextClick = false
      input?.blur()
      return
    }
    input?.blur()
    options.onApply(strength.value)
  }

  function onStrengthInput(event: Event) {
    const input = getRangeInput(event)
    if (!input || !isEditing) return
    syncStrengthFromRange(input)
  }

  function onStrengthChange(event: Event) {
    const input = getRangeInput(event)
    if (!input || !isEditing) return
    syncStrengthFromRange(input)
    finishStrengthEdit()
  }

  function onStrengthPointerUp(event: PointerEvent) {
    const input = getRangeInput(event)
    if (input && isEditing) syncStrengthFromRange(input)
    finishStrengthEdit()
  }

  function dispose() {
    longPressGeneration += 1
    disposeLongPress()
    isEditing = false
    visible.value = false
    rangeRef.value = null
  }

  if (getCurrentInstance()) {
    onBeforeUnmount(dispose)
  }

  return {
    strength,
    visible,
    setRangeRef,
    onButtonPointerDown,
    onButtonClick,
    onStrengthInput,
    onStrengthChange,
    onStrengthPointerUp,
    dispose,
  }
}
