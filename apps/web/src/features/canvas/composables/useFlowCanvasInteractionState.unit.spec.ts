import { describe, expect, it } from 'vitest'
import { useFlowCanvasInteractionState } from './useFlowCanvasInteractionState'

describe('useFlowCanvasInteractionState', () => {
  it('keeps viewport and node drag interactions render-only', () => {
    const state = useFlowCanvasInteractionState()

    state.startCanvasInteraction('flow-viewport')

    expect(state.isCanvasInteractionActive.value).toBe(true)
    expect(state.isViewportPanInteractionActive.value).toBe(false)
    expect(state.isCanvasInteractionLocked.value).toBe(false)

    state.startCanvasInteraction('viewport-pan')
    expect(state.isViewportPanInteractionActive.value).toBe(true)

    state.startCanvasInteraction('node-drag')
    state.stopCanvasInteraction('flow-viewport')

    expect(state.isCanvasInteractionActive.value).toBe(true)
    expect(state.isViewportPanInteractionActive.value).toBe(true)
    expect(state.isCanvasInteractionLocked.value).toBe(false)

    state.stopCanvasInteraction('viewport-pan')
    state.stopCanvasInteraction('node-drag')

    expect(state.isCanvasInteractionActive.value).toBe(false)
    expect(state.isCanvasInteractionLocked.value).toBe(false)
  })

  it('locks only while a lock-level interaction is active', () => {
    const state = useFlowCanvasInteractionState()

    state.startCanvasInteraction('flow-viewport')
    state.startCanvasInteraction('canvas-extend-overlay')

    expect(state.isCanvasInteractionActive.value).toBe(true)
    expect(state.isCanvasInteractionLocked.value).toBe(true)

    state.stopCanvasInteraction('canvas-extend-overlay')

    expect(state.isCanvasInteractionActive.value).toBe(true)
    expect(state.isCanvasInteractionLocked.value).toBe(false)
  })

  it('stays locked while any lock-level interaction remains active', () => {
    const state = useFlowCanvasInteractionState()

    state.startCanvasInteraction('canvas-extend-overlay')
    state.startCanvasInteraction('image-mask-editor')

    expect(state.isCanvasInteractionLocked.value).toBe(true)

    state.stopCanvasInteraction('canvas-extend-overlay')

    expect(state.isCanvasInteractionLocked.value).toBe(true)

    state.stopCanvasInteraction('image-mask-editor')

    expect(state.isCanvasInteractionLocked.value).toBe(false)
  })
})
