import { describe, expect, it } from 'vitest'
import { nextTick, ref } from 'vue'
import { useInpaintChromeState } from './useInpaintChromeState'

function createHarness() {
  const retouchNodeId = ref<string | null>('node-1')
  const state = useInpaintChromeState({ retouchNodeId })
  return { retouchNodeId, state }
}

describe('useInpaintChromeState', () => {
  it('clamps mask layer opacity', () => {
    const { state } = createHarness()

    state.setInpaintLayerOpacity(5)
    expect(state.inpaintLayerOpacity.value).toBe(15)

    state.setInpaintLayerOpacity(150)
    expect(state.inpaintLayerOpacity.value).toBe(100)
  })

  it('resets transient mask opacity when the editor node changes', async () => {
    const { retouchNodeId, state } = createHarness()
    state.setInpaintLayerOpacity(50)

    retouchNodeId.value = 'node-2'
    await nextTick()

    expect(state.inpaintLayerOpacity.value).toBe(72)
  })
})
