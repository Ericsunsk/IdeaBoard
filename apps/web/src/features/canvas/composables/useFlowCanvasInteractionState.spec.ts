import { describe, expect, it } from 'vitest'
import { useFlowCanvasInteractionState } from './useFlowCanvasInteractionState'

describe('useFlowCanvasInteractionState', () => {
  it('keeps viewport and node drag interactions render-only', () => {
    const state = useFlowCanvasInteractionState()

    state.startCanvasInteraction('flow-viewport')

    expect(state.isCanvasInteractionActive.value).toBe(true)
    expect(state.isCanvasInteractionLocked.value).toBe(false)

    state.startCanvasInteraction('node-drag')
    state.stopCanvasInteraction('flow-viewport')

    expect(state.isCanvasInteractionActive.value).toBe(true)
    expect(state.isCanvasInteractionLocked.value).toBe(false)

    state.stopCanvasInteraction('node-drag')

    expect(state.isCanvasInteractionActive.value).toBe(false)
    expect(state.isCanvasInteractionLocked.value).toBe(false)
  })

  it('locks only while a lock-level interaction is active', () => {
    const state = useFlowCanvasInteractionState()

    state.startCanvasInteraction('flow-viewport')
    state.startCanvasInteraction('image-mask-editor')

    expect(state.isCanvasInteractionActive.value).toBe(true)
    expect(state.isCanvasInteractionLocked.value).toBe(true)

    state.stopCanvasInteraction('image-mask-editor')

    expect(state.isCanvasInteractionActive.value).toBe(true)
    expect(state.isCanvasInteractionLocked.value).toBe(false)
  })
})
