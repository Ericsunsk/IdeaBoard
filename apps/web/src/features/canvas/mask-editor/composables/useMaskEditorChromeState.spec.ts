import { describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import { useMaskEditorChromeState } from './useMaskEditorChromeState'

function createHarness() {
  const imageEditorNodeId = ref<string | null>('node-1')
  const getImageNodeSnapshot = vi.fn((nodeId: string) =>
    nodeId === 'node-1' ? { outputCount: 99 } : null,
  )
  const uploadImageToBoard = vi.fn()
  const onWarning = vi.fn()
  const onReferenceImagePreview = vi.fn()
  const state = useMaskEditorChromeState({
    imageEditorNodeId,
    getImageNodeSnapshot,
    uploadImageToBoard,
    onWarning,
    onReferenceImagePreview,
  })

  return { imageEditorNodeId, getImageNodeSnapshot, onReferenceImagePreview, state }
}

describe('useMaskEditorChromeState', () => {
  it('normalizes active output count from the current image node', () => {
    const { state } = createHarness()

    expect(state.activeMaskEditorOutputCount.value).toBe(8)
  })

  it('clamps mask layer opacity', () => {
    const { state } = createHarness()

    state.setMaskEditorLayerOpacity(5)
    expect(state.maskEditorLayerOpacity.value).toBe(15)

    state.setMaskEditorLayerOpacity(150)
    expect(state.maskEditorLayerOpacity.value).toBe(100)
  })

  it('removes reference images and clears transient chrome state when editor node changes', async () => {
    const { imageEditorNodeId, state } = createHarness()
    state.maskEditorReferenceImages.value = [
      { id: 'ref-a', url: 'https://example.com/a.png', ratio: 1 },
      { id: 'ref-b', url: 'https://example.com/b.png', ratio: 1 },
    ]
    state.setMaskEditorLayerOpacity(50)

    state.onMaskEditorReferenceImageRemove('ref-a')
    expect(state.maskEditorReferenceImages.value).toEqual([
      { id: 'ref-b', url: 'https://example.com/b.png', ratio: 1 },
    ])

    imageEditorNodeId.value = 'node-2'
    await nextTick()

    expect(state.maskEditorLayerOpacity.value).toBe(72)
    expect(state.maskEditorReferenceImages.value).toEqual([])
  })

  it('normalizes reference image preview ratio', () => {
    const { onReferenceImagePreview, state } = createHarness()

    state.onMaskEditorReferenceImagePreview({ url: 'https://example.com/a.png' })

    expect(onReferenceImagePreview).toHaveBeenCalledWith({
      url: 'https://example.com/a.png',
      ratio: 1,
    })
  })
})
