import {
  getCurrentInstance,
  onBeforeUnmount,
  reactive,
  ref,
  type ComponentPublicInstance,
} from 'vue'
import { onLongPress } from '@vueuse/core'
import {
  DEFAULT_IMAGE_ACTION_STRENGTH,
  DEFAULT_IMAGE_ACTION_STRENGTHS,
  type ImageStrengthAction,
} from '../execution'

const LONG_PRESS_MS = 260
const MIN_STRENGTH = 0
const MAX_STRENGTH = 100

type StrengthRangeRefTarget = Element | ComponentPublicInstance | null

export function useFlowImageActionStrengthControl(options: {
  onCommit: (action: ImageStrengthAction, strength: number) => void
}) {
  const { onCommit } = options
  const activeStrengthAction = ref<ImageStrengthAction | null>(null)
  const strengths = reactive<Record<ImageStrengthAction, number>>({
    ...DEFAULT_IMAGE_ACTION_STRENGTHS,
  })
  const suppressClickAction = ref<ImageStrengthAction | null>(null)

  const strengthRangeElements = new Map<ImageStrengthAction, HTMLInputElement>()
  const stopLongPressByAction = new Map<ImageStrengthAction, () => void>()
  let editingAction: ImageStrengthAction | null = null
  let longPressGeneration = 0

  function clampStrength(value: number): number {
    if (!Number.isFinite(value)) return DEFAULT_IMAGE_ACTION_STRENGTH
    return Math.max(MIN_STRENGTH, Math.min(MAX_STRENGTH, Math.round(value)))
  }

  function getRangeInput(el: StrengthRangeRefTarget): HTMLInputElement | null {
    if (el instanceof HTMLInputElement) return el
    if (el && '$el' in el && el.$el instanceof HTMLInputElement) return el.$el
    return null
  }

  function getEventRangeInput(event: Event): HTMLInputElement | null {
    return event.currentTarget instanceof HTMLInputElement ? event.currentTarget : null
  }

  function syncStrengthFromRange(action: ImageStrengthAction, input: HTMLInputElement) {
    strengths[action] = clampStrength(input.valueAsNumber)
  }

  function focusRange(input: HTMLInputElement) {
    input.focus({ preventScroll: true })
  }

  function blurRange(action: ImageStrengthAction) {
    strengthRangeElements.get(action)?.blur()
  }

  function beginStrengthEdit(
    action: ImageStrengthAction,
    input: HTMLInputElement,
    options: { syncInitial: boolean },
  ) {
    if (!options.syncInitial) input.value = `${strengths[action]}`
    activeStrengthAction.value = action
    editingAction = action
    suppressClickAction.value = action
    if (options.syncInitial) syncStrengthFromRange(action, input)
    focusRange(input)
  }

  function finishStrengthEdit(action: ImageStrengthAction) {
    if (editingAction !== action) return
    editingAction = null
    activeStrengthAction.value = null
    blurRange(action)
    onCommit(action, strengths[action])
  }

  function disposeLongPress(action: ImageStrengthAction) {
    stopLongPressByAction.get(action)?.()
    stopLongPressByAction.delete(action)
    strengthRangeElements.delete(action)
  }

  function attachLongPress(action: ImageStrengthAction, input: HTMLInputElement) {
    const generation = longPressGeneration
    stopLongPressByAction.get(action)?.()
    stopLongPressByAction.set(
      action,
      onLongPress(
        input,
        () => {
          if (generation !== longPressGeneration) return
          if (input.disabled) return
          beginStrengthEdit(action, input, { syncInitial: false })
        },
        {
          delay: LONG_PRESS_MS,
          distanceThreshold: false,
        },
      ),
    )
  }

  function setStrengthRangeRef(action: ImageStrengthAction, el: StrengthRangeRefTarget) {
    const input = getRangeInput(el)
    const current = strengthRangeElements.get(action)
    if (current === input) return

    disposeLongPress(action)
    if (!input) return

    strengthRangeElements.set(action, input)
    attachLongPress(action, input)
  }

  function hideStrengthControl() {
    longPressGeneration += 1
    activeStrengthAction.value = null
    if (editingAction) blurRange(editingAction)
    editingAction = null
    suppressClickAction.value = null
    strengthRangeElements.forEach((input, action) => attachLongPress(action, input))
  }

  function onStrengthRangePointerDown(action: ImageStrengthAction, event: PointerEvent) {
    if (event.button !== 0) return
    const input = getEventRangeInput(event)
    if (!input) return
    if (activeStrengthAction.value === action) {
      beginStrengthEdit(action, input, { syncInitial: true })
      return
    }
    suppressClickAction.value = null
    editingAction = null
  }

  function onStrengthInput(action: ImageStrengthAction, event: Event) {
    const input = getEventRangeInput(event)
    if (!input || editingAction !== action || activeStrengthAction.value !== action) return
    syncStrengthFromRange(action, input)
  }

  function onStrengthChange(action: ImageStrengthAction, event: Event) {
    const input = getEventRangeInput(event)
    if (!input || editingAction !== action || activeStrengthAction.value !== action) return
    syncStrengthFromRange(action, input)
    finishStrengthEdit(action)
  }

  function onStrengthPointerUp(action: ImageStrengthAction, event: PointerEvent) {
    const input = getEventRangeInput(event)
    if (input && editingAction === action && activeStrengthAction.value === action) {
      syncStrengthFromRange(action, input)
    }
    finishStrengthEdit(action)
  }

  function onStrengthClick(_action: ImageStrengthAction, event: MouseEvent) {
    const input = getEventRangeInput(event)
    if (input) input.blur()
  }

  function getActionStrength(action: ImageStrengthAction): number {
    return strengths[action]
  }

  function dispose() {
    hideStrengthControl()
    Array.from(stopLongPressByAction.keys()).forEach(disposeLongPress)
  }

  if (getCurrentInstance()) {
    onBeforeUnmount(dispose)
  }

  return {
    activeStrengthAction,
    suppressClickAction,
    setStrengthRangeRef,
    hideStrengthControl,
    getActionStrength,
    onStrengthRangePointerDown,
    onStrengthInput,
    onStrengthChange,
    onStrengthPointerUp,
    onStrengthClick,
    dispose,
  }
}
