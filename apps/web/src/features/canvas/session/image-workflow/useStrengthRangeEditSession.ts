import type { ElementRefTarget } from '@/core/vue/refs'
import { onLongPress } from '@vueuse/core'
import { getCurrentInstance, onBeforeUnmount, ref, type Ref } from 'vue'

const DEFAULT_LONG_PRESS_MS = 260

type StrengthRangeEditSession<TKey> = {
  activeKey: Ref<TKey | null>
  setRangeRef: (key: TKey, el: ElementRefTarget) => void
  setTriggerRef: (key: TKey, el: ElementRefTarget) => void
  beginEdit: (
    key: TKey,
    input: HTMLInputElement,
    options: { shouldSyncInitial: boolean; initialValue?: number },
  ) => void
  finishEdit: (key: TKey) => void
  cancelEdit: (key: TKey) => void
  hide: () => void
  writeValueToRange: (key: TKey, input: HTMLInputElement, value: number) => void
  onRangePointerDown: (key: TKey, event: PointerEvent) => void
  onInput: (key: TKey, event: Event) => void
  onChange: (key: TKey, event: Event) => void
  onPointerUp: (key: TKey, event: PointerEvent) => void
  onClick: (event: MouseEvent) => void
  consumeSuppressedClick: (key: TKey) => boolean
  dispose: () => void
}

type UseStrengthRangeEditSessionOptions<TKey> = {
  readValue: (key: TKey) => number
  normalizeValue: (key: TKey, value: number) => number
  writeValue: (key: TKey, value: number) => void
  onCommit: (key: TKey, value: number) => void
  onCancel?: (key: TKey) => void
  longPressMs?: number
  onLongPressStart?: (
    key: TKey,
    input: HTMLInputElement,
    event: PointerEvent,
    session: StrengthRangeEditSession<TKey>,
  ) => void
  isRangeEventIgnored?: (key: TKey) => boolean
  isTriggerDisabled?: (key: TKey, trigger: HTMLElement, input: HTMLInputElement) => boolean
}

function getElement(el: ElementRefTarget): Element | null {
  if (el instanceof Element) return el
  if (el && '$el' in el && el.$el instanceof Element) return el.$el
  return null
}

function getRangeInput(el: ElementRefTarget): HTMLInputElement | null {
  const element = getElement(el)
  if (element instanceof HTMLInputElement) return element
  return null
}

function getTriggerElement(el: ElementRefTarget): HTMLElement | null {
  const element = getElement(el)
  if (element instanceof HTMLElement) return element
  return null
}

function getEventRangeInput(event: Event): HTMLInputElement | null {
  return event.currentTarget instanceof HTMLInputElement ? event.currentTarget : null
}

export function useStrengthRangeEditSession<TKey>(
  options: UseStrengthRangeEditSessionOptions<TKey>,
): StrengthRangeEditSession<TKey> {
  const activeKey = ref<TKey | null>(null) as Ref<TKey | null>
  const rangeElements = new Map<TKey, HTMLInputElement>()
  const triggerElements = new Map<TKey, HTMLElement>()
  const stopTriggerBindingByKey = new Map<TKey, () => void>()
  const longPressMs = options.longPressMs ?? DEFAULT_LONG_PRESS_MS

  let editingKey: TKey | null = null
  let suppressedClickKey: TKey | null = null
  let longPressGeneration = 0

  function writeValueToRange(key: TKey, input: HTMLInputElement, value: number) {
    const normalized = options.normalizeValue(key, value)
    options.writeValue(key, normalized)
    input.value = `${normalized}`
  }

  function syncValueFromRange(key: TKey, input: HTMLInputElement) {
    writeValueToRange(key, input, input.valueAsNumber)
  }

  function blurRange(key: TKey) {
    rangeElements.get(key)?.blur()
  }

  function beginEdit(
    key: TKey,
    input: HTMLInputElement,
    editOptions: { shouldSyncInitial: boolean; initialValue?: number },
  ) {
    if (!editOptions.shouldSyncInitial) {
      writeValueToRange(key, input, editOptions.initialValue ?? options.readValue(key))
    }
    activeKey.value = key
    editingKey = key
    suppressedClickKey = key
    if (editOptions.shouldSyncInitial) syncValueFromRange(key, input)
    input.focus({ preventScroll: true })
  }

  function finishEdit(key: TKey) {
    if (editingKey !== key) return
    const committedValue = options.readValue(key)
    editingKey = null
    activeKey.value = null
    blurRange(key)
    options.onCommit(key, committedValue)
  }

  function cancelEdit(key: TKey) {
    if (editingKey !== key) return
    editingKey = null
    activeKey.value = null
    blurRange(key)
    options.onCancel?.(key)
  }

  function clearSuppressedClick(key: TKey) {
    if (suppressedClickKey === key) suppressedClickKey = null
  }

  function consumeSuppressedClick(key: TKey): boolean {
    if (suppressedClickKey !== key) return false
    suppressedClickKey = null
    return true
  }

  function disposeTriggerBinding(key: TKey) {
    stopTriggerBindingByKey.get(key)?.()
    stopTriggerBindingByKey.delete(key)
  }

  function attachTriggerBinding(key: TKey, trigger: HTMLElement) {
    const generation = longPressGeneration
    disposeTriggerBinding(key)

    const onTriggerPointerDown = (event: PointerEvent) => {
      if (event.button !== 0) return
      clearSuppressedClick(key)
    }
    trigger.addEventListener('pointerdown', onTriggerPointerDown)

    const stopLongPress = onLongPress(
      trigger,
      (event) => {
        if (generation !== longPressGeneration) return
        const input = rangeElements.get(key)
        if (!input || input.disabled) return
        if (options.isTriggerDisabled?.(key, trigger, input)) return
        if (options.onLongPressStart) {
          options.onLongPressStart(key, input, event, api)
          return
        }
        beginEdit(key, input, { shouldSyncInitial: false })
      },
      {
        delay: longPressMs,
        distanceThreshold: false,
      },
    )
    stopTriggerBindingByKey.set(key, () => {
      stopLongPress()
      trigger.removeEventListener('pointerdown', onTriggerPointerDown)
    })
  }

  function setRangeRef(key: TKey, el: ElementRefTarget) {
    const input = getRangeInput(el)
    const current = rangeElements.get(key)
    if (current === input) return
    if (!input) {
      rangeElements.delete(key)
      return
    }
    rangeElements.set(key, input)
  }

  function setTriggerRef(key: TKey, el: ElementRefTarget) {
    const trigger = getTriggerElement(el)
    const current = triggerElements.get(key)
    if (current === trigger) return

    disposeTriggerBinding(key)
    if (!trigger) {
      triggerElements.delete(key)
      return
    }

    triggerElements.set(key, trigger)
    attachTriggerBinding(key, trigger)
  }

  function hide() {
    longPressGeneration += 1
    activeKey.value = null
    if (editingKey !== null) blurRange(editingKey)
    editingKey = null
    suppressedClickKey = null
    triggerElements.forEach((trigger, key) => attachTriggerBinding(key, trigger))
  }

  function onRangePointerDown(key: TKey, event: PointerEvent) {
    if (options.isRangeEventIgnored?.(key)) return
    if (event.button !== 0) return
    const input = getEventRangeInput(event)
    if (!input) return
    if (activeKey.value === key) {
      beginEdit(key, input, { shouldSyncInitial: true })
      return
    }
    suppressedClickKey = null
    editingKey = null
  }

  function onInput(key: TKey, event: Event) {
    if (options.isRangeEventIgnored?.(key)) return
    const input = getEventRangeInput(event)
    if (!input || editingKey !== key || activeKey.value !== key) return
    syncValueFromRange(key, input)
  }

  function onChange(key: TKey, event: Event) {
    if (options.isRangeEventIgnored?.(key)) return
    const input = getEventRangeInput(event)
    if (!input || editingKey !== key || activeKey.value !== key) return
    syncValueFromRange(key, input)
    finishEdit(key)
  }

  function onPointerUp(key: TKey, event: PointerEvent) {
    if (options.isRangeEventIgnored?.(key)) return
    const input = getEventRangeInput(event)
    if (input && editingKey === key && activeKey.value === key) {
      syncValueFromRange(key, input)
    }
    finishEdit(key)
  }

  function onClick(event: MouseEvent) {
    const input = getEventRangeInput(event)
    if (input) input.blur()
  }

  function dispose() {
    hide()
    Array.from(stopTriggerBindingByKey.keys()).forEach(disposeTriggerBinding)
    rangeElements.clear()
    triggerElements.clear()
  }

  const api: StrengthRangeEditSession<TKey> = {
    activeKey,
    setRangeRef,
    setTriggerRef,
    beginEdit,
    finishEdit,
    cancelEdit,
    hide,
    writeValueToRange,
    onRangePointerDown,
    onInput,
    onChange,
    onPointerUp,
    onClick,
    consumeSuppressedClick,
    dispose,
  }

  if (getCurrentInstance()) {
    onBeforeUnmount(dispose)
  }

  return api
}
