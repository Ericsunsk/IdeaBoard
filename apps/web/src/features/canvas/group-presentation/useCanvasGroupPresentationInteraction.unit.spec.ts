import type { Node } from '@vue-flow/core'
import { computed, effectScope, nextTick, ref, shallowRef } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { CanvasGraphGroupId } from '../graph-runtime'
import type { CanvasFlowViewportTransform, CanvasGroupFrame } from '../utils/canvasGroupGeometry'
import { useCanvasGroupPresentationInteraction } from './index'
import type { CanvasSelectionGroupAction } from '../composables/useCanvasGroupRuntime'

function createPromptNode(id: string, x: number, y: number, width = 280): Node {
  return {
    id,
    type: 'canvas',
    position: { x, y },
    data: {
      snapshotNode: { id: 1, type: 'prompt', x, y, width },
    },
  } as Node
}

function createFrame(id: CanvasGraphGroupId, selected = true): CanvasGroupFrame {
  return {
    id,
    nodeIds: ['prompt-1', 'prompt-2'],
    selected,
    order: 0,
    bounds: {
      left: 100,
      top: 80,
      right: 720,
      bottom: 360,
      width: 620,
      height: 280,
    },
  }
}

function createPointerEvent(
  target: EventTarget,
  overrides: Partial<PointerEvent> = {},
): PointerEvent {
  return {
    button: 0,
    pointerType: 'mouse',
    pointerId: 7,
    clientX: 120,
    clientY: 90,
    target,
    preventDefault: vi.fn(),
    stopPropagation: vi.fn(),
    ...overrides,
  } as unknown as PointerEvent
}

function createHarness(
  options: {
    enabled?: boolean
    action?: CanvasSelectionGroupAction | null
    viewport?: CanvasFlowViewportTransform
  } = {},
) {
  const canvasWrapRef = ref(document.createElement('div'))
  const nodes = shallowRef<Node[]>([
    createPromptNode('prompt-1', 100, 80),
    createPromptNode('prompt-2', 440, 80),
  ])
  const groupFrames = computed(() => [createFrame('group-1')])
  const viewport = ref(options.viewport ?? { x: 10, y: 20, zoom: 1 })
  const action = ref<CanvasSelectionGroupAction | null>(
    options.action ?? { kind: 'group', label: '组合', nodeIds: ['prompt-1', 'prompt-2'] },
  )
  const isEnabled = ref(options.enabled ?? true)
  const paneSelectionPointerDownCapture = vi.fn()
  const selectGroupAtPoint = vi.fn(() => true)
  const groupDrag = {
    begin: vi.fn(() => true),
    move: vi.fn(() => true),
    commit: vi.fn(() => true),
    cancel: vi.fn(() => true),
  }
  const groupResize = {
    begin: vi.fn(() => true),
    move: vi.fn(() => true),
    commit: vi.fn(() => true),
    cancel: vi.fn(() => true),
  }
  const dismissTransientCanvasUi = vi.fn()
  const activateSelectionGroupAction = vi.fn()

  const scope = effectScope()
  const runtime = scope.run(() =>
    useCanvasGroupPresentationInteraction({
      canvas: {
        host: canvasWrapRef,
        nodes,
        viewport,
        isInteractionEnabled: computed(() => isEnabled.value),
      },
      grouping: {
        frames: groupFrames,
        action: computed(() => action.value),
        selectAtPoint: selectGroupAtPoint,
        drag: groupDrag,
        resize: groupResize,
        onAction: activateSelectionGroupAction,
      },
      pane: {
        toFlowPoint: (point) => point,
        onPointerDownCapture: paneSelectionPointerDownCapture,
        dismiss: dismissTransientCanvasUi,
      },
    }),
  )
  if (!runtime) throw new Error('Group presentation interaction requires an active Vue scope')

  return {
    runtime,
    dispose: () => scope.stop(),
    action,
    viewport,
    isEnabled,
    paneSelectionPointerDownCapture,
    selectGroupAtPoint,
    groupDrag,
    groupResize,
    dismissTransientCanvasUi,
    activateSelectionGroupAction,
  }
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('useCanvasGroupPresentationInteraction', () => {
  it('resolves the selection group action button position from nodes and viewport', () => {
    const harness = createHarness()

    expect(harness.runtime.selectionGroupActionButtonPosition.value).toEqual({ x: 420, y: 88 })

    harness.viewport.value = { x: 0, y: 0, zoom: 2 }

    expect(harness.runtime.selectionGroupActionButtonPosition.value).toEqual({ x: 820, y: 148 })
  })

  it('resolves an empty group action button position from the selected group frame', () => {
    const harness = createHarness({
      action: {
        kind: 'ungroup',
        label: '解组',
        groupId: 'group-1',
        nodeIds: [],
      },
    })

    expect(harness.runtime.selectionGroupActionButtonPosition.value).toEqual({ x: 420, y: 88 })
  })

  it('delegates pane selection when group interaction is disabled', () => {
    const addListener = vi.spyOn(window, 'addEventListener')
    const pane = document.createElement('div')
    pane.className = 'vue-flow__pane'
    const harness = createHarness({ enabled: false })
    const event = createPointerEvent(pane)

    harness.runtime.onCanvasPointerDownCapture(event)

    expect(harness.paneSelectionPointerDownCapture).toHaveBeenCalledWith(event)
    expect(harness.selectGroupAtPoint).not.toHaveBeenCalled()
    expect(harness.groupDrag.begin).not.toHaveBeenCalled()
    expect(addListener).not.toHaveBeenCalled()
  })

  it('cleans up drag listeners and cancels the active drag on pointer cancel', () => {
    const addListener = vi.spyOn(window, 'addEventListener')
    const removeListener = vi.spyOn(window, 'removeEventListener')
    const pane = document.createElement('div')
    pane.className = 'vue-flow__pane'
    const harness = createHarness()
    const pointerDown = createPointerEvent(pane, { pointerId: 22 })

    harness.runtime.onCanvasPointerDownCapture(pointerDown)

    expect(pointerDown.preventDefault).toHaveBeenCalled()
    expect(pointerDown.stopPropagation).toHaveBeenCalled()
    expect(harness.groupDrag.begin).toHaveBeenCalledWith({ x: 120, y: 90 })

    const pointerCancel = addListener.mock.calls.find(([type]) => type === 'pointercancel')?.[1] as
      ((event: PointerEvent) => void) | undefined
    expect(pointerCancel).toBeDefined()

    pointerCancel?.(createPointerEvent(pane, { pointerId: 22 }))

    expect(harness.groupDrag.cancel).toHaveBeenCalledOnce()
    expect(removeListener).toHaveBeenCalledWith('pointermove', expect.any(Function))
    expect(removeListener).toHaveBeenCalledWith('pointerup', expect.any(Function))
    expect(removeListener).toHaveBeenCalledWith('pointercancel', expect.any(Function))
  })

  it('cleans up resize listeners and cancels the active resize on pointer cancel', () => {
    const addListener = vi.spyOn(window, 'addEventListener')
    const removeListener = vi.spyOn(window, 'removeEventListener')
    const handle = document.createElement('button')
    const harness = createHarness()

    harness.runtime.onGroupResizeStart({
      event: createPointerEvent(handle, { pointerId: 33 }),
      groupId: 'group-1',
      handle: 'e',
    })

    expect(harness.groupResize.begin).toHaveBeenCalledWith('group-1', 'e', { x: 120, y: 90 })
    const pointerCancel = addListener.mock.calls.find(([type]) => type === 'pointercancel')?.[1] as
      ((event: PointerEvent) => void) | undefined
    expect(pointerCancel).toBeDefined()

    pointerCancel?.(createPointerEvent(handle, { pointerId: 33 }))

    expect(harness.groupResize.cancel).toHaveBeenCalledOnce()
    expect(removeListener).toHaveBeenCalledWith('pointermove', expect.any(Function))
    expect(removeListener).toHaveBeenCalledWith('pointerup', expect.any(Function))
    expect(removeListener).toHaveBeenCalledWith('pointercancel', expect.any(Function))
  })

  it('opens and closes the selection group action from shared geometry hit testing', async () => {
    const harness = createHarness()
    const event = createPointerEvent(document.createElement('div'), {
      clientX: 420,
      clientY: 120,
    }) as MouseEvent

    harness.runtime.onCanvasContextMenuCapture(event)

    expect(event.preventDefault).toHaveBeenCalled()
    expect(event.stopPropagation).toHaveBeenCalled()
    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledOnce()
    expect(harness.runtime.isSelectionGroupActionVisible.value).toBe(true)

    harness.action.value = null
    await nextTick()

    expect(harness.runtime.isSelectionGroupActionVisible.value).toBe(false)
  })

  it('keeps the group action open on its control and non-primary pointers, then dismisses on primary outside pointerdown', async () => {
    const harness = createHarness()
    const actionButton = document.createElement('button')
    actionButton.className = 'canvas-selection-group-action-button'
    document.body.appendChild(actionButton)
    const contextMenuEvent = createPointerEvent(document.createElement('div'), {
      clientX: 420,
      clientY: 120,
    }) as MouseEvent

    try {
      harness.runtime.onCanvasContextMenuCapture(contextMenuEvent)
      await nextTick()

      actionButton.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0 }))
      expect(harness.runtime.isSelectionGroupActionVisible.value).toBe(true)

      document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 1 }))
      document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 2 }))
      expect(harness.runtime.isSelectionGroupActionVisible.value).toBe(true)

      document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0 }))
      expect(harness.runtime.isSelectionGroupActionVisible.value).toBe(false)
    } finally {
      actionButton.remove()
      harness.dispose()
    }
  })

  it('binds close policy around delegated shell handlers', () => {
    const harness = createHarness()
    const event = createPointerEvent(document.createElement('div'), {
      clientX: 420,
      clientY: 120,
    }) as MouseEvent
    const delegatedHandlers = {
      setActiveTool: vi.fn((value: 'select' | 'hand') => `tool:${value}`),
      onCanvasPaneClick: vi.fn(() => 'pane'),
    }

    harness.runtime.onCanvasContextMenuCapture(event)
    expect(harness.runtime.isSelectionGroupActionVisible.value).toBe(true)

    const closePolicyHandlers = harness.runtime.bindClosePolicyHandlers(delegatedHandlers)
    const result = closePolicyHandlers.setActiveTool('hand')

    expect(result).toBe('tool:hand')
    expect(delegatedHandlers.setActiveTool).toHaveBeenCalledWith('hand')
    expect(closePolicyHandlers.onCanvasPaneClick()).toBe('pane')
    expect(harness.runtime.isSelectionGroupActionVisible.value).toBe(false)
  })

  it('activates the group action through the presentation interface after closing its affordance', () => {
    const harness = createHarness()
    const event = createPointerEvent(document.createElement('div'), {
      clientX: 420,
      clientY: 120,
    }) as MouseEvent

    harness.runtime.onCanvasContextMenuCapture(event)
    expect(harness.runtime.isSelectionGroupActionVisible.value).toBe(true)

    harness.runtime.onSelectionGroupAction()

    expect(harness.activateSelectionGroupAction).toHaveBeenCalledOnce()
    expect(harness.runtime.isSelectionGroupActionVisible.value).toBe(false)
  })

  it('closes the action menu and cancels controls when its Vue scope is disposed', () => {
    const harness = createHarness()
    const event = createPointerEvent(document.createElement('div'), {
      clientX: 420,
      clientY: 120,
    }) as MouseEvent

    harness.runtime.onCanvasContextMenuCapture(event)
    expect(harness.runtime.isSelectionGroupActionVisible.value).toBe(true)

    harness.dispose()

    expect(harness.runtime.isSelectionGroupActionVisible.value).toBe(false)
    expect(harness.groupDrag.cancel).toHaveBeenCalledOnce()
    expect(harness.groupResize.cancel).toHaveBeenCalledOnce()
  })
})
