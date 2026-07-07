import type { Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { nextTick, ref, shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
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
  const overlay = useImageCanvasExtendOverlay({
    nodes,
    activeTool,
    getImageNodeDisplayPayload: (nodeId) =>
      nodeId === 'image-1' ? { url: 'https://example.com/source.png', ratio: 1 } : null,
    getImageStageElement: (nodeId) => (nodeId === 'image-1' ? stage : null),
    startCanvasInteraction,
    stopCanvasInteraction,
    onCommit,
  })

  return {
    activeTool,
    onCommit,
    overlay,
    startCanvasInteraction,
    stopCanvasInteraction,
  }
}

describe('useImageCanvasExtendOverlay', () => {
  it('owns the canvas interaction lifecycle while expand mode is active', async () => {
    const harness = createHarness()

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
    await harness.overlay.commitExpandMode('extend it', 1)

    expect(harness.overlay.isExpandMode.value).toBe(false)
    expect(harness.stopCanvasInteraction).toHaveBeenCalledTimes(3)
    expect(harness.onCommit).toHaveBeenCalledWith(
      expect.objectContaining({
        nodeId: 'image-1',
        prompt: 'extend it',
        outputCount: 1,
      }),
    )
  })
})
