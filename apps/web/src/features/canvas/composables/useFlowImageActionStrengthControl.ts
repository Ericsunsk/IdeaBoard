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
  onCancel?: (action: ImageStrengthAction) => void
}) {
  const { onCommit, onCancel } = options
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
  let dragSession: { action: ImageStrengthAction; pointerId: number; stop: () => void } | null =
    null

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

  function writeStrengthToRange(
    action: ImageStrengthAction,
    input: HTMLInputElement,
    value: number,
  ) {
    const strength = clampStrength(value)
    strengths[action] = strength
    input.value = `${strength}`
  }

  function syncStrengthFromRange(action: ImageStrengthAction, input: HTMLInputElement) {
    writeStrengthToRange(action, input, input.valueAsNumber)
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
    options: { syncInitial: boolean; initialStrength?: number },
  ) {
    if (!options.syncInitial) {
      writeStrengthToRange(action, input, options.initialStrength ?? strengths[action])
    }
    activeStrengthAction.value = action
    editingAction = action
    suppressClickAction.value = action
    if (options.syncInitial) syncStrengthFromRange(action, input)
    focusRange(input)
  }

  function finishStrengthEdit(action: ImageStrengthAction) {
    if (editingAction !== action) return
    const committedStrength = strengths[action]
    editingAction = null
    activeStrengthAction.value = null
    blurRange(action)
    onCommit(action, committedStrength)
    strengths[action] = DEFAULT_IMAGE_ACTION_STRENGTH
  }

  function cancelStrengthEdit(action: ImageStrengthAction) {
    if (editingAction !== action) return
    editingAction = null
    activeStrengthAction.value = null
    blurRange(action)
    strengths[action] = DEFAULT_IMAGE_ACTION_STRENGTH
    onCancel?.(action)
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

  function stopDragSession() {
    dragSession?.stop()
    dragSession = null
  }

  function getStrengthFromPointer(input: HTMLInputElement, event: PointerEvent): number {
    const rect = input.getBoundingClientRect()
    if (rect.width <= 0) return DEFAULT_IMAGE_ACTION_STRENGTH
    return clampStrength(((event.clientX - rect.left) / rect.width) * MAX_STRENGTH)
  }

  function startDragSession(
    action: ImageStrengthAction,
    input: HTMLInputElement,
    startEvent: PointerEvent,
  ) {
    stopDragSession()
    beginStrengthEdit(action, input, {
      syncInitial: false,
      initialStrength: DEFAULT_IMAGE_ACTION_STRENGTH,
    })

    const ownerDocument = input.ownerDocument
    const pointerId = startEvent.pointerId
    let hasEffectiveDrag = false

    const onPointerMove = (event: PointerEvent) => {
      if (event.pointerId !== pointerId) return
      const nextStrength = getStrengthFromPointer(input, event)
      writeStrengthToRange(action, input, nextStrength)
      if (Math.abs(nextStrength - DEFAULT_IMAGE_ACTION_STRENGTH) >= 1) {
        hasEffectiveDrag = true
      }
    }
    const onPointerEnd = (event: PointerEvent) => {
      if (event.pointerId !== pointerId) return
      stopDragSession()
      if (hasEffectiveDrag) {
        finishStrengthEdit(action)
        return
      }
      cancelStrengthEdit(action)
    }
    const onPointerCancel = (event: PointerEvent) => {
      if (event.pointerId !== pointerId) return
      stopDragSession()
      cancelStrengthEdit(action)
    }

    ownerDocument.addEventListener('pointermove', onPointerMove)
    ownerDocument.addEventListener('pointerup', onPointerEnd)
    ownerDocument.addEventListener('pointercancel', onPointerCancel)
    dragSession = {
      action,
      pointerId,
      stop: () => {
        ownerDocument.removeEventListener('pointermove', onPointerMove)
        ownerDocument.removeEventListener('pointerup', onPointerEnd)
        ownerDocument.removeEventListener('pointercancel', onPointerCancel)
      },
    }
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
      (event) => {
        if (generation !== longPressGeneration) return
        if (isTriggerDisabled(trigger)) return
        const input = strengthRangeElements.get(action)
        if (!input) return
        if (input.disabled) return
        startDragSession(action, input, event)
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
    stopDragSession()
    activeStrengthAction.value = null
    if (editingAction) blurRange(editingAction)
    editingAction = null
    suppressClickAction.value = null
    strengthTriggerElements.forEach((trigger, action) => attachTriggerBinding(action, trigger))
  }

  function onStrengthRangePointerDown(action: ImageStrengthAction, event: PointerEvent) {
    if (dragSession?.action === action) return
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
    if (dragSession?.action === action) return
    const input = getEventRangeInput(event)
    if (!input || editingAction !== action || activeStrengthAction.value !== action) return
    syncStrengthFromRange(action, input)
  }

  function onStrengthChange(action: ImageStrengthAction, event: Event) {
    if (dragSession?.action === action) return
    const input = getEventRangeInput(event)
    if (!input || editingAction !== action || activeStrengthAction.value !== action) return
    syncStrengthFromRange(action, input)
    finishStrengthEdit(action)
  }

  function onStrengthPointerUp(action: ImageStrengthAction, event: PointerEvent) {
    if (dragSession?.action === action) return
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
