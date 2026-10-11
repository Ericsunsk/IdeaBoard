import type { Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { effectScope, ref, shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useImageOutpaintOverlay } from './useImageOutpaintOverlay'

/**
 * canvas/outpaint-tool-lifecycle: outpaint enters/exits a tool session,
 * preserves node identity and draft frame bounds through commit, then
 * restores neutral Canvas state without stale overlays.
 */
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
  const scope = effectScope()
  const overlay = scope.run(() =>
    useImageOutpaintOverlay({
      nodes,
      activeTool,
      getImageNodeDisplayPayload: (nodeId) =>
        nodeId === 'image-1' ? { url: 'https://example.com/source.png', ratio: 1 } : null,
      getImageStageElement: (nodeId) => (nodeId === 'image-1' ? stage : null),
      startCanvasInteraction,
      stopCanvasInteraction,
      onCommit,
    }),
  )
  if (!overlay) throw new Error('Outpaint overlay requires an active Vue scope')
  return { dispose: () => scope.stop(), onCommit, overlay, startCanvasInteraction, stopCanvasInteraction }
}

function pointerDownOn(target: HTMLElement, clientX: number, clientY: number) {
  const event = new PointerEvent('pointerdown', { bubbles: true, clientX, clientY })
  target.dispatchEvent(event)
  return event
}

describe('outpaint tool lifecycle', () => {
  it('enters a session with node identity, preserves draft bounds through commit, then restores neutral state', async () => {
    const harness = createHarness()
    try {
      expect(harness.overlay.enterOutpaintMode('image-1')).toBe(true)
      expect(harness.overlay.isOutpaintMode.value).toBe(true)
      expect(harness.overlay.outpaintNodeId.value).toBe('image-1')
      const defaultStyle = { ...harness.overlay.outpaintOverlayStyle.value! }
      expect(defaultStyle.width).toBe('288px')

      const handle = document.createElement('div')
      document.body.appendChild(handle)
      try {
        harness.overlay.beginHandleResize(pointerDownOn(handle, 100, 100), 'e')
        harness.overlay.onHandlePointerMove(
          new PointerEvent('pointermove', { bubbles: true, clientX: 150, clientY: 100 }),
        )
      } finally {
        handle.remove()
      }
      const draftStyle = { ...harness.overlay.outpaintOverlayStyle.value! }
      expect(draftStyle.width).toBe('338px')

      await harness.overlay.commitOutpaintMode('extend it')
      expect(harness.onCommit).toHaveBeenCalledOnce()
      expect(harness.onCommit).toHaveBeenCalledWith(
        expect.objectContaining({
          nodeId: 'image-1',
          prompt: 'extend it',
          frameRect: expect.objectContaining({ left: 16, width: 338 }),
          originalImageRect: expect.objectContaining({ left: 40, top: 0 }),
        }),
      )

      expect(harness.overlay.isOutpaintMode.value).toBe(false)
      expect(harness.overlay.outpaintNodeId.value).toBeNull()
      expect(harness.overlay.outpaintOverlayStyle.value).toBeNull()
      expect(harness.stopCanvasInteraction).toHaveBeenCalledWith('outpaint-overlay')
    } finally {
      harness.dispose()
    }
  })

  it('restores neutral default bounds on re-enter instead of leaking the previous draft', () => {
    const harness = createHarness()
    try {
      expect(harness.overlay.enterOutpaintMode('image-1')).toBe(true)
      const handle = document.createElement('div')
      document.body.appendChild(handle)
      try {
        harness.overlay.beginHandleResize(pointerDownOn(handle, 100, 100), 'e')
        harness.overlay.onHandlePointerMove(
          new PointerEvent('pointermove', { bubbles: true, clientX: 150, clientY: 100 }),
        )
      } finally {
        handle.remove()
      }
      expect(harness.overlay.outpaintOverlayStyle.value?.width).toBe('338px')

      harness.overlay.cancelOutpaintMode()
      expect(harness.overlay.isOutpaintMode.value).toBe(false)
      expect(harness.overlay.outpaintOverlayStyle.value).toBeNull()

      expect(harness.overlay.enterOutpaintMode('image-1')).toBe(true)
      expect(harness.overlay.outpaintNodeId.value).toBe('image-1')
      expect(harness.overlay.outpaintOverlayStyle.value).toMatchObject({
        left: '16px',
        top: '-24px',
        width: '288px',
        height: '288px',
      })
    } finally {
      harness.dispose()
    }
  })
})
