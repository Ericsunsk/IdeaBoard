import { createApp, defineComponent } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFlowKeyboardShortcuts } from './useFlowKeyboardShortcuts'

function mountKeyboardShortcuts(options: Parameters<typeof useFlowKeyboardShortcuts>[0]) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        useFlowKeyboardShortcuts(options)
        return () => null
      },
    }),
  )
  app.mount(host)

  return {
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

function dispatchKeydown(
  init: KeyboardEventInit & { key: string },
  target: EventTarget = window,
) {
  const event = new KeyboardEvent('keydown', {
    bubbles: true,
    cancelable: true,
    ...init,
  })
  target.dispatchEvent(event)
  return event
}

function dispatchDelete(target: EventTarget = window) {
  return dispatchKeydown({ key: 'Delete' }, target)
}

function dispatchBackspace(target: EventTarget = window) {
  return dispatchKeydown({ key: 'Backspace' }, target)
}

describe('useFlowKeyboardShortcuts', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('does not delete selected nodes from canvas editing surfaces', () => {
    const onDeleteSelected = vi.fn()
    const harness = mountKeyboardShortcuts({
      onSave: vi.fn(),
      onDeleteSelected,
    })
    const editingSurface = document.createElement('button')
    editingSurface.type = 'button'
    editingSurface.dataset.imageEditor = ''
    document.body.appendChild(editingSurface)

    try {
      const deleteEvent = dispatchDelete(editingSurface)
      const backspaceEvent = dispatchBackspace(editingSurface)

      expect(deleteEvent.defaultPrevented).toBe(false)
      expect(backspaceEvent.defaultPrevented).toBe(false)
      expect(onDeleteSelected).not.toHaveBeenCalled()
    } finally {
      editingSurface.remove()
      harness.unmount()
    }
  })

  it('suppresses canvas shortcuts while a tool editing mode is active', () => {
    const onUndo = vi.fn()
    const onRedo = vi.fn()
    const onDeleteSelected = vi.fn()
    const onPanViewport = vi.fn()
    const onToolSelect = vi.fn()
    const onToolHand = vi.fn()
    const onToolText = vi.fn()
    const onSpacePressedChange = vi.fn()
    const harness = mountKeyboardShortcuts({
      onSave: vi.fn(),
      onUndo,
      onRedo,
      onDeleteSelected,
      isCanvasShortcutSuppressed: () => true,
      onPanViewport,
      onToolSelect,
      onToolHand,
      onToolText,
      onSpacePressedChange,
    })

    try {
      dispatchKeydown({ key: 'z', metaKey: true })
      dispatchKeydown({ key: 'y', ctrlKey: true })
      dispatchKeydown({ key: 'ArrowLeft' })
      dispatchKeydown({ key: 'v' })
      dispatchKeydown({ key: 'h' })
      dispatchKeydown({ key: 't' })
      dispatchKeydown({ key: ' ', code: 'Space' })
      const deleteEvent = dispatchDelete()
      const backspaceEvent = dispatchBackspace()

      expect(deleteEvent.defaultPrevented).toBe(false)
      expect(backspaceEvent.defaultPrevented).toBe(false)
      expect(onUndo).not.toHaveBeenCalled()
      expect(onRedo).not.toHaveBeenCalled()
      expect(onDeleteSelected).not.toHaveBeenCalled()
      expect(onPanViewport).not.toHaveBeenCalled()
      expect(onToolSelect).not.toHaveBeenCalled()
      expect(onToolHand).not.toHaveBeenCalled()
      expect(onToolText).not.toHaveBeenCalled()
      expect(onSpacePressedChange).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })

  it('keeps save available while canvas shortcuts are suppressed', () => {
    const onSave = vi.fn()
    const harness = mountKeyboardShortcuts({
      onSave,
      isCanvasShortcutSuppressed: () => true,
    })

    try {
      const event = dispatchKeydown({ key: 's', metaKey: true })

      expect(event.defaultPrevented).toBe(true)
      expect(onSave).toHaveBeenCalledOnce()
    } finally {
      harness.unmount()
    }
  })

  it('deletes selected nodes when no edit target or edit mode is active', () => {
    const onDeleteSelected = vi.fn()
    const harness = mountKeyboardShortcuts({
      onSave: vi.fn(),
      onDeleteSelected,
    })

    try {
      const event = dispatchDelete()

      expect(event.defaultPrevented).toBe(true)
      expect(onDeleteSelected).toHaveBeenCalledOnce()
    } finally {
      harness.unmount()
    }
  })
})
