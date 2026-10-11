import { useEventListener } from '@vueuse/core'
import { isEditableEventTarget } from '../../session/browser/domEventTargets.ts'
import {
  dispatchFlowKeyboardShortcutKeydown,
  type FlowKeyboardShortcutHandlers,
} from './flowKeyboardShortcutCommandMap'

type UseCanvasKeyboardShortcutsOptions = FlowKeyboardShortcutHandlers

const windowTarget = () => (typeof window === 'undefined' ? null : window)

export function useCanvasKeyboardShortcuts(options: UseCanvasKeyboardShortcutsOptions) {
  function handleSelectStart(event: Event) {
    const target = event.target
    const targetElement =
      target instanceof Element ? target : target instanceof Node ? target.parentElement : null
    if (!targetElement?.closest('.canvas-layout')) return
    if (isEditableEventTarget(targetElement)) return
    event.preventDefault()
  }

  function handleWindowBlur() {
    options.onSpacePressedChange?.(false)
    options.onDuplicateModifierChange?.(false)
  }

  function onWindowKeydown(event: KeyboardEvent) {
    dispatchFlowKeyboardShortcutKeydown(event, options)
  }

  function onWindowKeyup(event: KeyboardEvent) {
    if (event.code === 'Space') {
      options.onSpacePressedChange?.(false)
    }
    if (event.key === 'Alt') {
      options.onDuplicateModifierChange?.(false)
    }
  }

  useEventListener<KeyboardEvent>(windowTarget, 'keydown', onWindowKeydown)
  useEventListener<KeyboardEvent>(windowTarget, 'keyup', onWindowKeyup)
  useEventListener<Event>(windowTarget, 'selectstart', handleSelectStart)
  useEventListener<Event>(windowTarget, 'blur', handleWindowBlur)
}
