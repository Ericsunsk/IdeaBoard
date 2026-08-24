import type { Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { effectScope, nextTick, ref, shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useNodeToolPopoverGroup } from '../components/nodes/primitives/useNodeToolPopoverGroup'
import { useImageCanvasExtendOverlay } from './useImageCanvasExtendOverlay'

function createStageElement() {
  const stage = document.createElement('div')
  Object.defineProperties(stage, {
    clientWidth: { value: 320 },
    clientHeight: { value: 240 },
  })
  return stage
}

function createHarness() {
  const stage = createStageElement()
  const nodes: Ref<Node[]> = shallowRef([{ id: 'image-1', data: { kind: 'image' } }] as Node[])
  const activeTool = ref('select')
  const startCanvasInteraction = vi.fn()
  const stopCanvasInteraction = vi.fn()
  const onCommit = vi.fn(async () => undefined)
  const onBeforePointerCancel = vi.fn()
  const scope = effectScope()
  const overlay = scope.run(() =>
    useImageCanvasExtendOverlay({
      nodes,
      activeTool,
      getImageNodeDisplayPayload: (nodeId) =>
        nodeId === 'image-1' ? { url: 'https://example.com/source.png', ratio: 1 } : null,
      getImageStageElement: (nodeId) => (nodeId === 'image-1' ? stage : null),
      startCanvasInteraction,
      stopCanvasInteraction,
      onBeforePointerCancel,
      onCommit,
    }),
  )
  if (!overlay) throw new Error('Canvas extend overlay requires an active Vue scope')

  return {
    activeTool,
    dispose: () => scope.stop(),
    onCommit,
    onBeforePointerCancel,
    overlay,
    startCanvasInteraction,
    stopCanvasInteraction,
  }
}

describe('useImageCanvasExtendOverlay', () => {
  it('owns the canvas interaction lifecycle while expand mode is active', async () => {
    const harness = createHarness()

    try {
      expect(harness.overlay.enterExpandMode('image-1')).toBe(true)

      expect(harness.overlay.isExpandMode.value).toBe(true)
      expect(harness.startCanvasInteraction).toHaveBeenCalledWith('canvas-extend-overlay')

      harness.overlay.cancelExpandMode()

      expect(harness.overlay.isExpandMode.value).toBe(false)
      expect(harness.stopCanvasInteraction).toHaveBeenCalledWith('canvas-extend-overlay')

      expect(harness.overlay.enterExpandMode('image-1')).toBe(true)
      harness.activeTool.value = 'hand'
      await nextTick()

      expect(harness.overlay.isExpandMode.value).toBe(false)
      expect(harness.stopCanvasInteraction).toHaveBeenCalledTimes(2)

      expect(harness.overlay.enterExpandMode('image-1')).toBe(true)
      await harness.overlay.commitExpandMode('extend it')

      expect(harness.overlay.isExpandMode.value).toBe(false)
      expect(harness.stopCanvasInteraction).toHaveBeenCalledTimes(3)
      expect(harness.onCommit).toHaveBeenCalledWith(
        expect.objectContaining({
          nodeId: 'image-1',
          prompt: 'extend it',
        }),
      )
    } finally {
      harness.dispose()
    }
  })

  it('stays active when a later node popover owns the outside pointer dismissal', async () => {
    const harness = createHarness()
    const popoverElement = document.createElement('div')
    const popoverRoot = ref<HTMLElement | null>(popoverElement)
    document.body.appendChild(popoverElement)
    const popoverScope = effectScope()
    const popovers = popoverScope.run(() =>
      useNodeToolPopoverGroup({
        keys: ['output-count'] as const,
        rootRefs: [popoverRoot],
      }),
    )
    if (!popovers) throw new Error('Node popover requires an active Vue scope')

    try {
      expect(harness.overlay.enterExpandMode('image-1')).toBe(true)
      await nextTick()
      popovers.open('output-count')
      await nextTick()

      document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0 }))

      expect(popovers.activeKey.value).toBeNull()
      expect(harness.overlay.isExpandMode.value).toBe(true)
      expect(harness.onBeforePointerCancel).not.toHaveBeenCalled()
    } finally {
      popoverScope.stop()
      popoverRoot.value?.remove()
      harness.dispose()
    }
  })

  it('clears image tool transient state when its Vue scope is disposed', () => {
    const harness = createHarness()

    expect(harness.overlay.enterExpandMode('image-1')).toBe(true)

    harness.dispose()

    expect(harness.overlay.isExpandMode.value).toBe(false)
    expect(harness.stopCanvasInteraction).toHaveBeenCalledOnce()
  })
})
