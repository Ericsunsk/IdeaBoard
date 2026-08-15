import { reactive } from 'vue'
import {
  DEFAULT_IMAGE_ACTION_STRENGTH,
  DEFAULT_IMAGE_ACTION_STRENGTHS,
  type ImageStrengthAction,
} from '../execution'
import { useStrengthRangeEditSession } from '../composables/useStrengthRangeEditSession'

const MIN_STRENGTH = 0
const MAX_STRENGTH = 100

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

  function clampStrength(value: number): number {
    if (!Number.isFinite(value)) return DEFAULT_IMAGE_ACTION_STRENGTH
    return Math.max(MIN_STRENGTH, Math.min(MAX_STRENGTH, Math.round(value)))
  }

  function stopDragSession() {
    dragSession?.stop()
    dragSession = null
  }

  const strengthEdit = useStrengthRangeEditSession<ImageStrengthAction>({
    readValue: (action) => strengths[action],
    normalizeValue: (_action, value) => clampStrength(value),
    writeValue: (action, value) => {
      strengths[action] = value
    },
    onCommit: (action, strength) => {
      onCommit(action, strength)
      strengths[action] = DEFAULT_IMAGE_ACTION_STRENGTH
    },
    onCancel: (action) => {
      strengths[action] = DEFAULT_IMAGE_ACTION_STRENGTH
      onCancel?.(action)
    },
    onLongPressStart: (action, input, event) => {
      startDragSession(action, input, event)
    },
    isRangeEventIgnored: (action) => dragSession?.action === action,
    isTriggerDisabled: (_action, trigger) =>
      trigger instanceof HTMLButtonElement && trigger.disabled,
  })

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
    const initialStrength = getStrengthFromPointer(input, startEvent)
    strengthEdit.beginEdit(action, input, {
      syncInitial: false,
      initialValue: initialStrength,
    })

    const ownerDocument = input.ownerDocument
    const pointerId = startEvent.pointerId
    let hasEffectiveDrag = false

    const onPointerMove = (event: PointerEvent) => {
      if (event.pointerId !== pointerId) return
      const nextStrength = getStrengthFromPointer(input, event)
      strengthEdit.writeValueToRange(action, input, nextStrength)
      if (Math.abs(nextStrength - initialStrength) >= 1) {
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
