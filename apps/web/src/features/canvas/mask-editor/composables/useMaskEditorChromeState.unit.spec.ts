import { afterEach, describe, expect, it, vi } from 'vitest'
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

  return {
    imageEditorNodeId,
    getImageNodeSnapshot,
    onReferenceImagePreview,
    uploadImageToBoard,
    onWarning,
    state,
  }
}

describe('useMaskEditorChromeState', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

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

  it('opens a single-image file dialog and uploads the selected reference image', async () => {
    const input = document.createElement('input')
    const click = vi.spyOn(input, 'click').mockImplementation(() => {})
    vi.spyOn(document, 'createElement').mockImplementation((tagName, options) => {
      if (tagName === 'input') return input
      return Document.prototype.createElement.call(document, tagName, options)
    })
    const { state, uploadImageToBoard } = createHarness()
    const file = new File(['image'], 'reference.png', { type: 'image/png' })
    uploadImageToBoard.mockResolvedValue({
      url: 'https://example.com/ref.png',
      ratio: 1.25,
      mediaAssetId: 42,
    })

    state.onMaskEditorReferenceImageOpen()

    expect(input.type).toBe('file')
    expect(input.accept).toContain('image/')
    expect(input.multiple).toBe(false)
    expect(click).toHaveBeenCalledTimes(1)

    Object.defineProperty(input, 'files', {
      configurable: true,
      value: [file],
    })
    input.dispatchEvent(new Event('change'))
    await nextTick()
    await Promise.resolve()

    expect(uploadImageToBoard).toHaveBeenCalledWith(file)
    expect(state.maskEditorReferenceImages.value).toMatchObject([
      { url: 'https://example.com/ref.png', ratio: 1.25 },
    ])
  })
})
