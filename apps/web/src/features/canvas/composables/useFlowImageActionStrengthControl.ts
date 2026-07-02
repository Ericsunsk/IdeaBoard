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

type StrengthRefTarget = Element | ComponentPublicInstance | null

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
  const strengthTriggerElements = new Map<ImageStrengthAction, HTMLElement>()
  const stopTriggerBindingByAction = new Map<ImageStrengthAction, () => void>()
  let editingAction: ImageStrengthAction | null = null
  let longPressGeneration = 0

  function clampStrength(value: number): number {
    if (!Number.isFinite(value)) return DEFAULT_IMAGE_ACTION_STRENGTH
    return Math.max(MIN_STRENGTH, Math.min(MAX_STRENGTH, Math.round(value)))
  }

  function getElement(el: StrengthRefTarget): Element | null {
    if (el instanceof Element) return el
    if (el && '$el' in el && el.$el instanceof Element) return el.$el
    return null
  }

  function getRangeInput(el: StrengthRefTarget): HTMLInputElement | null {
    const element = getElement(el)
    if (element instanceof HTMLInputElement) return element
    return null
  }

  function getTriggerElement(el: StrengthRefTarget): HTMLElement | null {
    const element = getElement(el)
    if (element instanceof HTMLElement) return element
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

  function isTriggerDisabled(trigger: HTMLElement) {
    return trigger instanceof HTMLButtonElement && trigger.disabled
  }

  function clearSuppressedClick(action: ImageStrengthAction) {
    if (suppressClickAction.value === action) suppressClickAction.value = null
  }

  function consumeSuppressedStrengthClick(action: ImageStrengthAction): boolean {
    if (suppressClickAction.value !== action) return false
    suppressClickAction.value = null
    return true
  }

  function disposeTriggerBinding(action: ImageStrengthAction) {
    stopTriggerBindingByAction.get(action)?.()
    stopTriggerBindingByAction.delete(action)
  }

  function attachTriggerBinding(action: ImageStrengthAction, trigger: HTMLElement) {
    const generation = longPressGeneration
    disposeTriggerBinding(action)

    const onTriggerPointerDown = (event: PointerEvent) => {
      if (event.button !== 0) return
      clearSuppressedClick(action)
    }
    trigger.addEventListener('pointerdown', onTriggerPointerDown)

    const stopLongPress = onLongPress(
      trigger,
      () => {
        if (generation !== longPressGeneration) return
        if (isTriggerDisabled(trigger)) return
        const input = strengthRangeElements.get(action)
        if (!input) return
        if (input.disabled) return
        beginStrengthEdit(action, input, { syncInitial: false })
      },
      {
        delay: LONG_PRESS_MS,
        distanceThreshold: false,
      },
    )
    stopTriggerBindingByAction.set(action, () => {
      stopLongPress()
      trigger.removeEventListener('pointerdown', onTriggerPointerDown)
    })
  }

  function setStrengthRangeRef(action: ImageStrengthAction, el: StrengthRefTarget) {
    const input = getRangeInput(el)
    const current = strengthRangeElements.get(action)
    if (current === input) return

    if (!input) {
      strengthRangeElements.delete(action)
      return
    }
    strengthRangeElements.set(action, input)
  }

  function setStrengthTriggerRef(action: ImageStrengthAction, el: StrengthRefTarget) {
    const trigger = getTriggerElement(el)
    const current = strengthTriggerElements.get(action)
    if (current === trigger) return

    disposeTriggerBinding(action)
    if (!trigger) {
      strengthTriggerElements.delete(action)
      return
    }

    strengthTriggerElements.set(action, trigger)
    attachTriggerBinding(action, trigger)
  }

  function hideStrengthControl() {
    longPressGeneration += 1
    activeStrengthAction.value = null
    if (editingAction) blurRange(editingAction)
    editingAction = null
    suppressClickAction.value = null
    strengthTriggerElements.forEach((trigger, action) => attachTriggerBinding(action, trigger))
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
    Array.from(stopTriggerBindingByAction.keys()).forEach(disposeTriggerBinding)
    strengthRangeElements.clear()
    strengthTriggerElements.clear()
  }

  if (getCurrentInstance()) {
    onBeforeUnmount(dispose)
  }

  return {
    activeStrengthAction,
    consumeSuppressedStrengthClick,
    setStrengthRangeRef,
    setStrengthTriggerRef,
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
