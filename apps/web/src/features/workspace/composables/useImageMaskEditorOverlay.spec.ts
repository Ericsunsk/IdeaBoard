import type { Node } from '@vue-flow/core'
import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { useImageMaskEditorOverlay } from './useImageMaskEditorOverlay'

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
  const overlay = useImageMaskEditorOverlay({
    nodes: ref([{ id: 'node-1', data: { kind: 'image' } }] as Node[]),
    activeTool: ref('select'),
    getImageNodeDisplayPayload: (nodeId) =>
      nodeId === 'node-1' ? { url: 'https://example.com/source.png', ratio: 1 } : null,
    getImageStageElement: (nodeId) => (nodeId === 'node-1' ? stage : null),
  })

  return { overlay }
}

describe('useImageMaskEditorOverlay', () => {
  it('keeps mask editor active when committing without a drawable mask', () => {
    const { overlay } = createHarness()

    expect(overlay.enterMaskEditorMode('node-1')).toBe(true)
    expect(overlay.maskEditorNodeId.value).toBe('node-1')

    expect(overlay.commitMaskEditorMode()).toBe(false)

    expect(overlay.isMaskEditorMode.value).toBe(true)
    expect(overlay.maskEditorNodeId.value).toBe('node-1')
  })
})
