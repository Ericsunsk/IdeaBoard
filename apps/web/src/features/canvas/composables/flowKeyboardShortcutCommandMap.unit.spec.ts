import { describe, expect, it, vi } from 'vitest'
import {
  dispatchFlowKeyboardShortcutKeydown,
  type FlowKeyboardShortcutHandlers,
} from './flowKeyboardShortcutCommandMap'

function createHandlers(
  overrides: Partial<FlowKeyboardShortcutHandlers> = {},
): FlowKeyboardShortcutHandlers {
  return {
    onSave: vi.fn(),
    onUndo: vi.fn(),
    onRedo: vi.fn(),
    onDeleteInlineSelection: vi.fn(() => true),
    isCanvasShortcutSuppressed: vi.fn(() => false),
    onEscape: vi.fn(() => true),
    onPanViewport: vi.fn(),
    onToolSelect: vi.fn(),
    onToolHand: vi.fn(),
    onSpacePressedChange: vi.fn(),
    onDuplicateModifierChange: vi.fn(),
    ...overrides,
  }
}

function runShortcut(
  init: KeyboardEventInit & { key: string },
  handlers: FlowKeyboardShortcutHandlers,
  target: EventTarget = window,
) {
  const event = new KeyboardEvent('keydown', {
    bubbles: true,
    cancelable: true,
    ...init,
  })
  let handled = false
  target.addEventListener(
    'keydown',
    (received) => {
      handled = dispatchFlowKeyboardShortcutKeydown(received as KeyboardEvent, handlers)
    },
    { once: true },
  )
  target.dispatchEvent(event)
  return { event, handled }
}

describe('flowKeyboardShortcutCommandMap', () => {
  it('dispatches command shortcuts from the command map', () => {
    const cases: Array<{
      name: string
      init: KeyboardEventInit & { key: string }
      assert: (handlers: FlowKeyboardShortcutHandlers, event: KeyboardEvent) => void
    }> = [
      {
        name: 'save',
        init: { key: 's', metaKey: true },
        assert: (handlers, event) => {
          expect(handlers.onSave).toHaveBeenCalledOnce()
          expect(event.defaultPrevented).toBe(true)
        },
      },
      {
        name: 'undo',
        init: { key: 'z', metaKey: true },
        assert: (handlers, event) => {
          expect(handlers.onUndo).toHaveBeenCalledOnce()
          expect(event.defaultPrevented).toBe(true)
        },
      },
      {
        name: 'redo with Shift+Command+Z',
        init: { key: 'z', metaKey: true, shiftKey: true },
        assert: (handlers, event) => {
          expect(handlers.onRedo).toHaveBeenCalledOnce()
          expect(event.defaultPrevented).toBe(true)
        },
      },
      {
        name: 'redo with Ctrl+Y',
        init: { key: 'y', ctrlKey: true },
        assert: (handlers, event) => {
          expect(handlers.onRedo).toHaveBeenCalledOnce()
          expect(event.defaultPrevented).toBe(true)
        },
      },
      {
        name: 'keyboard pan',
        init: { key: 'ArrowLeft' },
        assert: (handlers, event) => {
          expect(handlers.onPanViewport).toHaveBeenCalledWith({ x: -48, y: 0 })
          expect(event.defaultPrevented).toBe(true)
        },
      },
      {
        name: 'fast keyboard pan',
        init: { key: 'ArrowRight', shiftKey: true },
        assert: (handlers, event) => {
          expect(handlers.onPanViewport).toHaveBeenCalledWith({ x: 120, y: 0 })
          expect(event.defaultPrevented).toBe(true)
        },
      },
      {
        name: 'select tool',
        init: { key: 'v' },
        assert: (handlers, event) => {
          expect(handlers.onToolSelect).toHaveBeenCalledOnce()
          expect(event.defaultPrevented).toBe(false)
        },
      },
      {
        name: 'hand tool',
        init: { key: 'h' },
        assert: (handlers, event) => {
          expect(handlers.onToolHand).toHaveBeenCalledOnce()
          expect(event.defaultPrevented).toBe(false)
        },
      },
      {
        name: 'space drag state',
        init: { key: ' ', code: 'Space' },
        assert: (handlers, event) => {
          expect(handlers.onSpacePressedChange).toHaveBeenCalledWith(true)
          expect(event.defaultPrevented).toBe(false)
        },
      },
      {
        name: 'duplicate modifier state',
        init: { key: 'Alt' },
        assert: (handlers, event) => {
          expect(handlers.onDuplicateModifierChange).toHaveBeenCalledWith(true)
          expect(event.defaultPrevented).toBe(false)
        },
      },
      {
        name: 'inline delete',
        init: { key: 'Delete' },
        assert: (handlers, event) => {
          expect(handlers.onDeleteInlineSelection).toHaveBeenCalledOnce()
          expect(event.defaultPrevented).toBe(true)
        },
      },
    ]

    for (const testCase of cases) {
      const handlers = createHandlers()
      const { event, handled } = runShortcut(testCase.init, handlers)

      expect(handled, testCase.name).toBe(true)
      testCase.assert(handlers, event)
    }
  })

  it('keeps save available for editable targets while suppressing canvas commands', () => {
    const input = document.createElement('input')
    document.body.appendChild(input)
    const saveHandlers = createHandlers()
    const deleteHandlers = createHandlers()

    try {
      const save = runShortcut({ key: 's', metaKey: true }, saveHandlers, input)
      const del = runShortcut({ key: 'Delete' }, deleteHandlers, input)

      expect(save.handled).toBe(true)
      expect(save.event.defaultPrevented).toBe(true)
      expect(saveHandlers.onSave).toHaveBeenCalledOnce()
      expect(del.handled).toBe(false)
      expect(del.event.defaultPrevented).toBe(false)
      expect(deleteHandlers.onDeleteInlineSelection).not.toHaveBeenCalled()
    } finally {
      input.remove()
    }
  })

  it('suppresses every Canvas shortcut while an explicit Canvas focus scope is active', () => {
    const handlers = createHandlers({
      isCanvasShortcutSuppressed: vi.fn(() => true),
    })

    const undo = runShortcut({ key: 'z', metaKey: true }, handlers)
    const pan = runShortcut({ key: 'ArrowDown' }, handlers)
    const tool = runShortcut({ key: 'v' }, handlers)
    const space = runShortcut({ key: ' ', code: 'Space' }, handlers)
    const del = runShortcut({ key: 'Delete' }, handlers)
    const alt = runShortcut({ key: 'Alt' }, handlers)
    const save = runShortcut({ key: 's', metaKey: true }, handlers)

    expect(undo.handled).toBe(false)
    expect(pan.handled).toBe(false)
    expect(tool.handled).toBe(false)
    expect(space.handled).toBe(false)
    expect(del.handled).toBe(false)
    expect(alt.handled).toBe(false)
    expect(save.handled).toBe(false)
    expect(handlers.onUndo).not.toHaveBeenCalled()
    expect(handlers.onPanViewport).not.toHaveBeenCalled()
    expect(handlers.onToolSelect).not.toHaveBeenCalled()
    expect(handlers.onSpacePressedChange).not.toHaveBeenCalled()
    expect(handlers.onDeleteInlineSelection).not.toHaveBeenCalled()
    expect(handlers.onDuplicateModifierChange).not.toHaveBeenCalled()
    expect(handlers.onSave).not.toHaveBeenCalled()
  })

  it('leaves Delete default handling alone when inline deletion does not consume it', () => {
    const handlers = createHandlers({
      onDeleteInlineSelection: vi.fn(() => false),
    })

    const { event, handled } = runShortcut({ key: 'Backspace' }, handlers)

    expect(handled).toBe(true)
    expect(event.defaultPrevented).toBe(false)
    expect(handlers.onDeleteInlineSelection).toHaveBeenCalledOnce()
  })
})
