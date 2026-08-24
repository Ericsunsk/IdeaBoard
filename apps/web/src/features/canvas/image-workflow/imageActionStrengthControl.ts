import { reactive } from 'vue'
import {
  DEFAULT_IMAGE_ACTION_STRENGTHS,
  getImageActionStrengthConfig,
  type ImageStrengthAction,
} from '../execution'
import { useStrengthRangeEditSession } from './useStrengthRangeEditSession'

export function useFlowImageActionStrengthControl(options: {
  onCommit: (action: ImageStrengthAction, strength: number) => void
  onCancel?: (action: ImageStrengthAction) => void
}) {
  const { onCommit, onCancel } = options
  const strengths = reactive<Record<ImageStrengthAction, number>>({
    ...DEFAULT_IMAGE_ACTION_STRENGTHS,
  })
  let dragSession: { action: ImageStrengthAction; pointerId: number; stop: () => void } | null =
    null

  function normalizeStrength(action: ImageStrengthAction, value: number): number {
    const config = getImageActionStrengthConfig(action)
    if (!Number.isFinite(value)) return config.defaultValue
    const clamped = Math.max(config.min, Math.min(config.max, value))
    return Math.round(clamped / config.step) * config.step
  }

  function stopDragSession() {
    dragSession?.stop()
    dragSession = null
  }

  const strengthEdit = useStrengthRangeEditSession<ImageStrengthAction>({
    readValue: (action) => strengths[action],
    normalizeValue: (action, value) => normalizeStrength(action, value),
    writeValue: (action, value) => {
      strengths[action] = value
    },
    onCommit: (action, strength) => {
      onCommit(action, strength)
      strengths[action] = getImageActionStrengthConfig(action).defaultValue
    },
    onCancel: (action) => {
      strengths[action] = getImageActionStrengthConfig(action).defaultValue
      onCancel?.(action)
    },
    onLongPressStart: (action, input, event) => {
      startDragSession(action, input, event)
    },
    isRangeEventIgnored: (action) => dragSession?.action === action,
    isTriggerDisabled: (_action, trigger) =>
      trigger instanceof HTMLButtonElement && trigger.disabled,
  })

  function getStrengthFromPointer(
    action: ImageStrengthAction,
    input: HTMLInputElement,
    event: PointerEvent,
  ): number {
    const rect = input.getBoundingClientRect()
    const config = getImageActionStrengthConfig(action)
    if (rect.width <= 0) return config.defaultValue
    const progress = (event.clientX - rect.left) / rect.width
    return normalizeStrength(action, config.min + progress * (config.max - config.min))
  }

  function startDragSession(
    action: ImageStrengthAction,
    input: HTMLInputElement,
    startEvent: PointerEvent,
  ) {
    stopDragSession()
    const initialStrength = getStrengthFromPointer(action, input, startEvent)
    strengthEdit.beginEdit(action, input, {
      syncInitial: false,
      initialValue: initialStrength,
    })

    const ownerDocument = input.ownerDocument
    const pointerId = startEvent.pointerId
    let hasEffectiveDrag = false

    const onPointerMove = (event: PointerEvent) => {
      if (event.pointerId !== pointerId) return
      const nextStrength = getStrengthFromPointer(action, input, event)
      strengthEdit.writeValueToRange(action, input, nextStrength)
      if (Math.abs(nextStrength - initialStrength) >= getImageActionStrengthConfig(action).step) {
        hasEffectiveDrag = true
      }
    }
    const onPointerEnd = (event: PointerEvent) => {
      if (event.pointerId !== pointerId) return
      stopDragSession()
      if (hasEffectiveDrag) {
        // The document-level drag bypasses the range's native commit event.
        input.dispatchEvent(new Event('change', { bubbles: true }))
        return
      }
      strengthEdit.cancelEdit(action)
    }
    const onPointerCancel = (event: PointerEvent) => {
      if (event.pointerId !== pointerId) return
      stopDragSession()
      strengthEdit.cancelEdit(action)
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

  function hideStrengthControl() {
    stopDragSession()
    strengthEdit.hide()
  }

  function getActionStrength(action: ImageStrengthAction): number {
    return strengths[action]
  }

  function dispose() {
    stopDragSession()
    strengthEdit.dispose()
  }

  return {
    activeStrengthAction: strengthEdit.activeKey,
    consumeSuppressedStrengthClick: strengthEdit.consumeSuppressedClick,
    setStrengthRangeRef: strengthEdit.setRangeRef,
    setStrengthTriggerRef: strengthEdit.setTriggerRef,
    hideStrengthControl,
    getActionStrength,
    onStrengthRangePointerDown: strengthEdit.onRangePointerDown,
    onStrengthInput: strengthEdit.onInput,
    onStrengthChange: strengthEdit.onChange,
    onStrengthPointerUp: strengthEdit.onPointerUp,
    onStrengthClick: (_action: ImageStrengthAction, event: MouseEvent) => {
      strengthEdit.onClick(event)
    },
    dispose,
  }
}
