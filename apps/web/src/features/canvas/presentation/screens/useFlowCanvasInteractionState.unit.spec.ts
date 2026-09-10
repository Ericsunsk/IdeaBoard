import { describe, expect, it } from 'vitest'
import { useFlowCanvasInteractionState } from './useFlowCanvasInteractionState'

describe('useFlowCanvasInteractionState', () => {
  it('keeps viewport, node drag, and outpaint interactions render-only', () => {
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

    state.startCanvasInteraction('outpaint-overlay')
    expect(state.isCanvasInteractionActive.value).toBe(true)
    expect(state.isCanvasInteractionLocked.value).toBe(false)

    state.stopCanvasInteraction('outpaint-overlay')
    state.stopCanvasInteraction('viewport-pan')
    state.stopCanvasInteraction('node-drag')

    expect(state.isCanvasInteractionActive.value).toBe(false)
    expect(state.isCanvasInteractionLocked.value).toBe(false)
  })

  it('locks only while a pixel-editing interaction is active', () => {
    const state = useFlowCanvasInteractionState()

    state.startCanvasInteraction('flow-viewport')
    state.startCanvasInteraction('retouch-raster')

    expect(state.isCanvasInteractionActive.value).toBe(true)
    expect(state.isCanvasInteractionLocked.value).toBe(true)

    state.stopCanvasInteraction('retouch-raster')

    expect(state.isCanvasInteractionActive.value).toBe(true)
    expect(state.isCanvasInteractionLocked.value).toBe(false)
  })

  it('stays locked while any pixel-editing interaction remains active', () => {
    const state = useFlowCanvasInteractionState()

    state.startCanvasInteraction('retouch-raster')
    state.startCanvasInteraction('inpaint')

    expect(state.isCanvasInteractionLocked.value).toBe(true)

    state.stopCanvasInteraction('retouch-raster')

    expect(state.isCanvasInteractionLocked.value).toBe(true)

    state.stopCanvasInteraction('inpaint')

    expect(state.isCanvasInteractionLocked.value).toBe(false)
  })
})
