import type { Node } from '@vue-flow/core'
import { describe, expect, it } from 'vitest'
import { ref, shallowRef } from 'vue'
import type { Ref } from 'vue'
import { useImageEditorMode } from '../../image-editor'
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
  const nodes: Ref<Node[]> = shallowRef([{ id: 'node-1', data: { kind: 'image' } }] as Node[])
  const editorMode = useImageEditorMode({
    nodes,
    activeTool: ref('select'),
    getImageNodeDisplayPayload: (nodeId) =>
      nodeId === 'node-1' ? { url: 'https://example.com/source.png', ratio: 1 } : null,
    getImageStageElement: (nodeId) => (nodeId === 'node-1' ? stage : null),
  })
  const overlay = useImageMaskEditorOverlay({
    imageEditorState: editorMode.imageEditorState,
  })

  return { editorMode, overlay }
}

describe('useImageMaskEditorOverlay', () => {
  it('keeps mask editor active when committing without a drawable mask', () => {
    const { editorMode, overlay } = createHarness()

    expect(editorMode.enterImageEditorMode('node-1', 'inpaint')).toBe(true)
    expect(overlay.maskEditorNodeId.value).toBe('node-1')

    expect(overlay.commitMaskDraft()).toBe(false)

    expect(editorMode.imageEditorState.value).not.toBeNull()
    expect(overlay.maskEditorNodeId.value).toBe('node-1')
  })
})
