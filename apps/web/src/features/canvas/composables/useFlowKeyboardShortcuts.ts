import { onMounted, onUnmounted } from 'vue'
import {
  isEditableEventTarget,
  isCanvasEditingSurfaceEventTarget,
} from '../utils/domEventTargets'

type UseFlowKeyboardShortcutsOptions = {
  onSave: () => void | Promise<void>
  onUndo?: () => void
  onRedo?: () => void
  onDeleteSelected?: () => void
  onDeleteInlineSelection?: () => boolean
  isCanvasShortcutSuppressed?: (event: KeyboardEvent) => boolean
  onEscape?: () => boolean | void
  onPanViewport?: (delta: { x: number; y: number }) => void
  onToolSelect?: () => void
  onToolHand?: () => void
  onToolText?: () => void
  onSpacePressedChange?: (pressed: boolean) => void
  onDuplicateModifierChange?: (pressed: boolean) => void
}

const KEYBOARD_PAN_STEP = 48
const KEYBOARD_PAN_FAST_STEP = 120

function getKeyboardPanDelta(key: string, step: number): { x: number; y: number } | null {
  switch (key) {
    case 'arrowleft':
      return { x: -step, y: 0 }
    case 'arrowright':
      return { x: step, y: 0 }
    case 'arrowup':
      return { x: 0, y: -step }
    case 'arrowdown':
      return { x: 0, y: step }
    default:
      return null
  }
}

export function useFlowKeyboardShortcuts(options: UseFlowKeyboardShortcutsOptions) {
  function isCanvasShortcutSuppressed(event: KeyboardEvent): boolean {
    return (
      options.isCanvasShortcutSuppressed?.(event) === true ||
      isCanvasEditingSurfaceEventTarget(event.target)
    )
  }

  function handleSelectStart(event: Event) {
    const target = event.target as HTMLElement | null
    if (!target?.closest('.canvas-layout')) return
    if (isEditableEventTarget(target)) return
    event.preventDefault()
  }

  function handleWindowBlur() {
    options.onSpacePressedChange?.(false)
    options.onDuplicateModifierChange?.(false)
  }

  function onWindowKeydown(event: KeyboardEvent) {
    const key = event.key.toLowerCase()
    const hasCommand = event.metaKey || event.ctrlKey
    if (hasCommand && key === 's') {
      event.preventDefault()
      void options.onSave()
      return
    }
    if (isEditableEventTarget(event.target)) return
    if (key === 'escape') {
      if (options.onEscape?.()) {
        event.preventDefault()
      }
      return
    }
    if (hasCommand && key === 'z') {
      if (isCanvasShortcutSuppressed(event)) return
      const handler = event.shiftKey ? options.onRedo : options.onUndo
      if (handler) {
        event.preventDefault()
        handler()
      }
      return
    }
    if (!event.metaKey && event.ctrlKey && key === 'y') {
      if (isCanvasShortcutSuppressed(event)) return
      if (options.onRedo) {
        event.preventDefault()
        options.onRedo()
      }
      return
    }
    if (!event.metaKey && !event.ctrlKey && !event.altKey) {
      const delta = getKeyboardPanDelta(
        key,
        event.shiftKey ? KEYBOARD_PAN_FAST_STEP : KEYBOARD_PAN_STEP,
      )
      if (delta) {
        if (isCanvasShortcutSuppressed(event)) return
        event.preventDefault()
        options.onPanViewport?.(delta)
        return
      }
    }
    if (!event.metaKey && !event.ctrlKey && !event.altKey && !event.shiftKey) {
      if (key === 'v') {
        if (isCanvasShortcutSuppressed(event)) return
        options.onToolSelect?.()
        return
      }
      if (key === 'h') {
        if (isCanvasShortcutSuppressed(event)) return
        options.onToolHand?.()
        return
      }
      if (key === 't') {
        if (isCanvasShortcutSuppressed(event)) return
        options.onToolText?.()
        return
      }
    }
    if (event.code === 'Space') {
      if (isCanvasShortcutSuppressed(event)) return
      options.onSpacePressedChange?.(true)
    }
    if (event.key === 'Alt') {
      options.onDuplicateModifierChange?.(true)
    }
    if (!options.onDeleteSelected) return
    if (key !== 'delete' && key !== 'backspace') return
    if (event.metaKey || event.ctrlKey || event.altKey) return
    if (isCanvasShortcutSuppressed(event)) return
    event.preventDefault()
    if (options.onDeleteInlineSelection?.()) return
    options.onDeleteSelected()
  }

  function onWindowKeyup(event: KeyboardEvent) {
    if (event.code === 'Space') {
      options.onSpacePressedChange?.(false)
    }
    if (event.key === 'Alt') {
      options.onDuplicateModifierChange?.(false)
    }
  }

  onMounted(() => {
    window.addEventListener('keydown', onWindowKeydown)
    window.addEventListener('keyup', onWindowKeyup)
    window.addEventListener('selectstart', handleSelectStart)
    window.addEventListener('blur', handleWindowBlur)
  })

  onUnmounted(() => {
    window.removeEventListener('keydown', onWindowKeydown)
    window.removeEventListener('keyup', onWindowKeyup)
    window.removeEventListener('selectstart', handleSelectStart)
    window.removeEventListener('blur', handleWindowBlur)
  })
}
