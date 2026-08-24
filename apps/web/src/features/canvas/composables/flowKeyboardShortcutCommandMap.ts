import { isCanvasEditingSurfaceEventTarget, isEditableEventTarget } from '../utils/domEventTargets'

export type FlowKeyboardShortcutHandlers = {
  onSave: () => void | Promise<void>
  onUndo?: () => void
  onRedo?: () => void
  onDeleteInlineSelection?: () => boolean
  isCanvasShortcutSuppressed?: (event: KeyboardEvent) => boolean
  onEscape?: () => boolean | void
  onPanViewport?: (delta: { x: number; y: number }) => void
  onToolSelect?: () => void
  onToolHand?: () => void
  onSpacePressedChange?: (pressed: boolean) => void
  onDuplicateModifierChange?: (pressed: boolean) => void
}

type ShortcutCommandResult = {
  handled: boolean
  preventDefault?: boolean
}

type FlowKeyboardShortcutCommand = {
  id: string
  allowEditableTarget?: boolean
  match: (event: KeyboardEvent, key: string) => boolean
  run: (
    event: KeyboardEvent,
    handlers: FlowKeyboardShortcutHandlers,
    key: string,
  ) => ShortcutCommandResult
}

const KEYBOARD_PAN_STEP = 48
const KEYBOARD_PAN_FAST_STEP = 120

function hasCommandModifier(event: KeyboardEvent): boolean {
  return event.metaKey || event.ctrlKey
}

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

function isCanvasShortcutSuppressed(
  event: KeyboardEvent,
  handlers: FlowKeyboardShortcutHandlers,
): boolean {
  return (
    handlers.isCanvasShortcutSuppressed?.(event) === true ||
    isCanvasEditingSurfaceEventTarget(event.target)
  )
}

const flowKeyboardShortcutCommands: FlowKeyboardShortcutCommand[] = [
  {
    id: 'save',
    allowEditableTarget: true,
    match: (event, key) => hasCommandModifier(event) && key === 's',
    run: (_event, handlers) => {
      void handlers.onSave()
      return { handled: true, preventDefault: true }
    },
  },
  {
    id: 'escape',
    match: (_event, key) => key === 'escape',
    run: (_event, handlers) => ({
      handled: true,
      preventDefault: handlers.onEscape?.() === true,
    }),
  },
  {
    id: 'undo-redo-z',
    match: (event, key) => hasCommandModifier(event) && key === 'z',
    run: (event, handlers) => {
      const handler = event.shiftKey ? handlers.onRedo : handlers.onUndo
      if (!handler) return { handled: true }
      handler()
      return { handled: true, preventDefault: true }
    },
  },
  {
    id: 'redo-y',
    match: (event, key) => !event.metaKey && event.ctrlKey && key === 'y',
    run: (_event, handlers) => {
      if (!handlers.onRedo) return { handled: true }
      handlers.onRedo()
      return { handled: true, preventDefault: true }
    },
  },
  {
    id: 'viewport-pan',
    match: (event, key) =>
      !event.metaKey && !event.ctrlKey && !event.altKey && !!getKeyboardPanDelta(key, 1),
    run: (event, handlers, key) => {
      const delta = getKeyboardPanDelta(
        key,
        event.shiftKey ? KEYBOARD_PAN_FAST_STEP : KEYBOARD_PAN_STEP,
      )
      if (!delta) return { handled: false }
      handlers.onPanViewport?.(delta)
      return { handled: true, preventDefault: true }
    },
  },
  {
    id: 'tool-select',
    match: (event, key) =>
      !event.metaKey && !event.ctrlKey && !event.altKey && !event.shiftKey && key === 'v',
    run: (_event, handlers) => {
      handlers.onToolSelect?.()
      return { handled: true }
    },
  },
  {
    id: 'tool-hand',
    match: (event, key) =>
      !event.metaKey && !event.ctrlKey && !event.altKey && !event.shiftKey && key === 'h',
    run: (_event, handlers) => {
      handlers.onToolHand?.()
      return { handled: true }
    },
  },
  {
    id: 'space-drag',
    match: (event) => event.code === 'Space',
    run: (_event, handlers) => {
      handlers.onSpacePressedChange?.(true)
      return { handled: true }
    },
  },
  {
    id: 'duplicate-modifier',
    match: (event) => event.key === 'Alt',
    run: (_event, handlers) => {
      handlers.onDuplicateModifierChange?.(true)
      return { handled: true }
    },
  },
  {
    id: 'inline-delete',
    match: (_event, key) => key === 'delete' || key === 'backspace',
    run: (event, handlers) => {
      if (event.metaKey || event.ctrlKey || event.altKey) return { handled: false }
      if (!handlers.onDeleteInlineSelection) return { handled: false }
      return {
        handled: true,
        preventDefault: handlers.onDeleteInlineSelection(),
      }
    },
  },
]

export function dispatchFlowKeyboardShortcutKeydown(
  event: KeyboardEvent,
  handlers: FlowKeyboardShortcutHandlers,
): boolean {
  if (isCanvasShortcutSuppressed(event, handlers)) return false
  const key = event.key.toLowerCase()
  const isEditableTarget = isEditableEventTarget(event.target)
  for (const command of flowKeyboardShortcutCommands) {
    if (!command.match(event, key)) continue
    if (!command.allowEditableTarget && isEditableTarget) return false
    const result = command.run(event, handlers, key)
    if (result.preventDefault) event.preventDefault()
    return result.handled
  }
  return false
}
