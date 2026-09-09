import { createApp, defineComponent } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useCanvasKeyboardShortcuts } from './useCanvasKeyboardShortcuts'

function mountKeyboardShortcuts(options: Parameters<typeof useCanvasKeyboardShortcuts>[0]) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        useCanvasKeyboardShortcuts(options)
        return () => null
      },
    }),
  )
  app.mount(host)
  let isUnmounted = false

  return {
    unmount: () => {
      if (isUnmounted) return
      isUnmounted = true
      app.unmount()
      host.remove()
    },
  }
}

function dispatchKeydown(init: KeyboardEventInit & { key: string }, target: EventTarget = window) {
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

function dispatchKeyup(init: KeyboardEventInit & { key: string }, target: EventTarget = window) {
  const event = new KeyboardEvent('keyup', {
    bubbles: true,
    cancelable: true,
    ...init,
  })
  target.dispatchEvent(event)
  return event
}

function dispatchSelectStart(target: EventTarget = window) {
  const event = new Event('selectstart', {
    bubbles: true,
    cancelable: true,
  })
  target.dispatchEvent(event)
  return event
}

function dispatchWindowBlur() {
  const event = new Event('blur')
  window.dispatchEvent(event)
  return event
}

describe('useCanvasKeyboardShortcuts', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('does not delete selected nodes from canvas editing surfaces', () => {
    const onDeleteInlineSelection = vi.fn()
    const harness = mountKeyboardShortcuts({
      onSave: vi.fn(),
      onDeleteInlineSelection,
    })
    const editingSurface = document.createElement('button')
    editingSurface.type = 'button'
    editingSurface.dataset.retouch = ''
    document.body.appendChild(editingSurface)

    try {
      const deleteEvent = dispatchDelete(editingSurface)
      const backspaceEvent = dispatchBackspace(editingSurface)

      expect(deleteEvent.defaultPrevented).toBe(false)
      expect(backspaceEvent.defaultPrevented).toBe(false)
      expect(onDeleteInlineSelection).not.toHaveBeenCalled()
    } finally {
      editingSurface.remove()
      harness.unmount()
    }
  })

  it('ignores Delete from nested flow editing surface controls', () => {
    const onDeleteInlineSelection = vi.fn()
    const harness = mountKeyboardShortcuts({
      onSave: vi.fn(),
      onDeleteInlineSelection,
    })
    const editingSurface = document.createElement('div')
    editingSurface.dataset.flowEditingSurface = ''
    const nestedButton = document.createElement('button')
    nestedButton.type = 'button'
    const nestedText = document.createElement('span')
    editingSurface.append(nestedButton, nestedText)
    document.body.appendChild(editingSurface)

    try {
      const buttonDeleteEvent = dispatchDelete(nestedButton)
      const textBackspaceEvent = dispatchBackspace(nestedText)

      expect(buttonDeleteEvent.defaultPrevented).toBe(false)
      expect(textBackspaceEvent.defaultPrevented).toBe(false)
      expect(onDeleteInlineSelection).not.toHaveBeenCalled()
    } finally {
      editingSurface.remove()
      harness.unmount()
    }
  })

  it('suppresses canvas shortcuts while a tool editing mode is active', () => {
    const onUndo = vi.fn()
    const onRedo = vi.fn()
    const onDeleteInlineSelection = vi.fn()
    const onPanViewport = vi.fn()
    const onToolSelect = vi.fn()
    const onToolHand = vi.fn()
    const onSpacePressedChange = vi.fn()
    const harness = mountKeyboardShortcuts({
      onSave: vi.fn(),
      onUndo,
      onRedo,
      onDeleteInlineSelection,
      isCanvasShortcutSuppressed: () => true,
      onPanViewport,
      onToolSelect,
      onToolHand,
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
      expect(onDeleteInlineSelection).not.toHaveBeenCalled()
      expect(onPanViewport).not.toHaveBeenCalled()
      expect(onToolSelect).not.toHaveBeenCalled()
      expect(onToolHand).not.toHaveBeenCalled()
      expect(onSpacePressedChange).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })

  it('does not save while an explicit Canvas focus scope suppresses shortcuts', () => {
    const onSave = vi.fn()
    const harness = mountKeyboardShortcuts({
      onSave,
      isCanvasShortcutSuppressed: () => true,
    })

    try {
      const event = dispatchKeydown({ key: 's', metaKey: true })

      expect(event.defaultPrevented).toBe(false)
      expect(onSave).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })

  it('leaves graph Delete to Vue Flow when there is no inline selection', () => {
    const onDeleteInlineSelection = vi.fn(() => false)
    const harness = mountKeyboardShortcuts({
      onSave: vi.fn(),
      onDeleteInlineSelection,
    })

    try {
      const event = dispatchDelete()

      expect(event.defaultPrevented).toBe(false)
      expect(onDeleteInlineSelection).toHaveBeenCalledOnce()
    } finally {
      harness.unmount()
    }
  })

  it('prevents native Delete only when inline selection handles it', () => {
    const onDeleteInlineSelection = vi.fn(() => true)
    const harness = mountKeyboardShortcuts({
      onSave: vi.fn(),
      onDeleteInlineSelection,
    })

    try {
      const event = dispatchDelete()

      expect(event.defaultPrevented).toBe(true)
      expect(onDeleteInlineSelection).toHaveBeenCalledOnce()
    } finally {
      harness.unmount()
    }
  })

  it('cleans up global listeners when the component scope is disposed', () => {
    const onSave = vi.fn()
    const onSpacePressedChange = vi.fn()
    const onDuplicateModifierChange = vi.fn()
    const harness = mountKeyboardShortcuts({
      onSave,
      onSpacePressedChange,
      onDuplicateModifierChange,
    })
    const canvasLayout = document.createElement('div')
    canvasLayout.className = 'canvas-layout'
    const canvasText = document.createElement('span')
    canvasLayout.appendChild(canvasText)
    document.body.appendChild(canvasLayout)

    try {
      const mountedSaveEvent = dispatchKeydown({ key: 's', metaKey: true })
      dispatchKeyup({ key: ' ', code: 'Space' })
      dispatchWindowBlur()
      const mountedSelectStartEvent = dispatchSelectStart(canvasText)

      expect(mountedSaveEvent.defaultPrevented).toBe(true)
      expect(onSave).toHaveBeenCalledOnce()
      expect(onSpacePressedChange).toHaveBeenCalledTimes(2)
      expect(onDuplicateModifierChange).toHaveBeenCalledOnce()
      expect(mountedSelectStartEvent.defaultPrevented).toBe(true)

      harness.unmount()

      const unmountedSaveEvent = dispatchKeydown({ key: 's', metaKey: true })
      dispatchKeyup({ key: ' ', code: 'Space' })
      dispatchWindowBlur()
      const unmountedSelectStartEvent = dispatchSelectStart(canvasText)

      expect(unmountedSaveEvent.defaultPrevented).toBe(false)
      expect(onSave).toHaveBeenCalledOnce()
      expect(onSpacePressedChange).toHaveBeenCalledTimes(2)
      expect(onDuplicateModifierChange).toHaveBeenCalledOnce()
      expect(unmountedSelectStartEvent.defaultPrevented).toBe(false)
    } finally {
      canvasLayout.remove()
      harness.unmount()
    }
  })

  it('prevents selection when selectstart originates from a canvas text node', () => {
    const harness = mountKeyboardShortcuts({ onSave: vi.fn() })
    const canvasLayout = document.createElement('div')
    canvasLayout.className = 'canvas-layout'
    const canvasText = document.createTextNode('Canvas text')
    canvasLayout.appendChild(canvasText)
    document.body.appendChild(canvasLayout)

    try {
      const event = dispatchSelectStart(canvasText)

      expect(event.defaultPrevented).toBe(true)
    } finally {
      canvasLayout.remove()
      harness.unmount()
    }
  })
})
