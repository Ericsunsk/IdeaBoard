import { describe, expect, it } from 'vitest'
import { nextTick, ref } from 'vue'
import { useMaskEditorChromeState } from './useMaskEditorChromeState'

function createHarness() {
  const imageEditorNodeId = ref<string | null>('node-1')
  const state = useMaskEditorChromeState({ imageEditorNodeId })
  return { imageEditorNodeId, state }
}

describe('useMaskEditorChromeState', () => {
  it('clamps mask layer opacity', () => {
    const { state } = createHarness()

    state.setMaskEditorLayerOpacity(5)
    expect(state.maskEditorLayerOpacity.value).toBe(15)

    state.setMaskEditorLayerOpacity(150)
    expect(state.maskEditorLayerOpacity.value).toBe(100)
  })

  it('resets transient mask opacity when the editor node changes', async () => {
    const { imageEditorNodeId, state } = createHarness()
    state.setMaskEditorLayerOpacity(50)

    imageEditorNodeId.value = 'node-2'
    await nextTick()

    expect(state.maskEditorLayerOpacity.value).toBe(72)
  })
})
