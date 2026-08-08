import type { Node } from '@vue-flow/core'
import { effectScope, nextTick, ref, shallowRef, type Ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useImageEditorCoordinateAdapter } from './useImageEditorCoordinateAdapter'

type Rect = { left: number; top: number; width: number; height: number }

function setElementGeometry(
  element: HTMLElement,
  { clientWidth, clientHeight, rect }: { clientWidth: number; clientHeight: number; rect: Rect },
) {
  Object.defineProperties(element, {
    clientWidth: { configurable: true, value: clientWidth },
    clientHeight: { configurable: true, value: clientHeight },
  })
  element.getBoundingClientRect = vi.fn(
    () =>
      ({
        ...rect,
        right: rect.left + rect.width,
        bottom: rect.top + rect.height,
        x: rect.left,
        y: rect.top,
        toJSON: () => ({}),
      }) as DOMRect,
  )
}

function createHarness(
  options: {
    stage?: { clientWidth: number; clientHeight: number; rect: Rect }
    canvas?: Rect
    viewport?: { x: number; y: number; zoom: number }
  } = {},
) {
  const stage = document.createElement('div')
  const stageGeometry =
    options.stage ??
    ({
      clientWidth: 400,
      clientHeight: 300,
      rect: { left: 100, top: 200, width: 600, height: 450 },
    } as const)
  setElementGeometry(stage, stageGeometry)

  const canvas = document.createElement('div')
  setElementGeometry(canvas, {
    clientWidth: options.canvas?.width ?? 1200,
    clientHeight: options.canvas?.height ?? 900,
    rect: options.canvas ?? { left: 20, top: 30, width: 1200, height: 900 },
  })

  const imageEditorState = ref<{ nodeId: string; mode: 'adjust' | 'inpaint' } | null>({
    nodeId: 'image-1',
    mode: 'adjust',
  })
  const nodes: Ref<Node[]> = shallowRef([
    { id: 'image-1', position: { x: 20, y: 40 }, data: { snapshotNode: { width: 400 } } },
  ] as Node[])
  const viewport = ref(options.viewport ?? { x: 0, y: 0, zoom: 1.5 })
  const setCenter = vi.fn(async () => undefined)
  const setViewport = vi.fn(async () => undefined)
  const screenToFlowCoordinate = vi.fn((point: { x: number; y: number }) => ({
    x: point.x / 2,
    y: point.y / 2,
  }))

  const scope = effectScope()
  const adapter = scope.run(() =>
    useImageEditorCoordinateAdapter({
      nodes,
      canvasWrapRef: ref(canvas),
      imageEditorState,
      getImageNodeDisplayPayload: (nodeId) =>
        nodeId === 'image-1' ? { url: 'https://example.com/image.png', ratio: 1 } : null,
      getImageStageElement: (nodeId) => (nodeId === 'image-1' ? stage : null),
      screenToFlowCoordinate,
      getViewport: () => viewport.value,
      setViewport,
      setCenter,
    }),
  )
  if (!adapter) throw new Error('Image Editor coordinate adapter requires a Vue scope')

  return {
    adapter,
    imageEditorState,
    setCenter,
    setViewport,
    screenToFlowCoordinate,
    stage,
    scope,
  }
}

describe('useImageEditorCoordinateAdapter', () => {
  it('keeps contained editor geometry and chrome anchoring stable at a non-default zoom', async () => {
    const harness = createHarness()

    try {
      await nextTick()

      expect(harness.adapter.imageEditorImageRect.value).toEqual({
        left: 50,
        top: 0,
        width: 300,
        height: 300,
      })
      expect(harness.adapter.imageEditorChromeAnchorRect.value).toEqual({
        left: 155,
        top: 170,
        width: 450,
        height: 450,
      })

      await harness.adapter.focusImageEditorAndShowChrome('image-1')

      expect(harness.screenToFlowCoordinate).toHaveBeenCalledWith({ x: 400, y: 425 })
      expect(harness.setCenter).toHaveBeenCalledWith(
        expect.any(Number),
        212.5,
        expect.objectContaining({ duration: 220, interpolate: 'smooth' }),
      )
    } finally {
      harness.scope.stop()
    }
  })

  it('remeasures the contained image and chrome anchor after a viewport resize', async () => {
    const harness = createHarness()

    try {
      await nextTick()
      setElementGeometry(harness.stage, {
        clientWidth: 600,
        clientHeight: 300,
        rect: { left: 80, top: 160, width: 900, height: 450 },
      })

      harness.adapter.scheduleImageEditorChromeAnchorUpdate()
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))

      expect(harness.adapter.imageEditorImageRect.value).toEqual({
        left: 150,
        top: 0,
        width: 300,
        height: 300,
      })
      expect(harness.adapter.imageEditorChromeAnchorRect.value).toEqual({
        left: 285,
        top: 130,
        width: 450,
        height: 450,
      })
    } finally {
      harness.scope.stop()
    }
  })

  it('uses the same coordinate seam for both Image Editing and Image Mask Editing targets', async () => {
    const harness = createHarness()

    try {
      await nextTick()
      const adjustRect = harness.adapter.imageEditorImageRect.value
      const adjustAnchor = harness.adapter.imageEditorChromeAnchorRect.value

      harness.imageEditorState.value = { nodeId: 'image-1', mode: 'inpaint' }
      await nextTick()

      expect(harness.adapter.imageEditorImageRect.value).toEqual(adjustRect)
      expect(harness.adapter.imageEditorChromeAnchorRect.value).toEqual(adjustAnchor)
    } finally {
      harness.scope.stop()
    }
  })

  it('does not focus or reveal chrome after the editor target is cancelled during focus', async () => {
    const harness = createHarness()
    const setCenter = vi.fn(() => {
      harness.imageEditorState.value = null
      return Promise.resolve()
    })
    harness.scope.stop()

    const scope = effectScope()
    const adapter = scope.run(() =>
      useImageEditorCoordinateAdapter({
        nodes: shallowRef([{ id: 'image-1', position: { x: 20, y: 40 } }] as Node[]),
        canvasWrapRef: ref(document.createElement('div')),
        imageEditorState: harness.imageEditorState,
        getImageNodeDisplayPayload: () => ({ url: 'https://example.com/image.png', ratio: 1 }),
        getImageStageElement: () => harness.stage,
        screenToFlowCoordinate: (point) => point,
        getViewport: () => ({ x: 0, y: 0, zoom: 1 }),
        setViewport: vi.fn(),
        setCenter,
      }),
    )
    if (!adapter) throw new Error('Image Editor coordinate adapter requires a Vue scope')

    try {
      harness.imageEditorState.value = { nodeId: 'image-1', mode: 'adjust' }
      await adapter.focusImageEditorAndShowChrome('image-1')

      expect(setCenter).toHaveBeenCalledOnce()
      expect(adapter.isImageEditorChromeReady.value).toBe(false)
      expect(adapter.imageEditorChromeAnchorRect.value).toBeNull()
    } finally {
      scope.stop()
    }
  })

  it('restores the captured viewport once when an active editor exits', async () => {
    const harness = createHarness({ viewport: { x: 12, y: 24, zoom: 0.8 } })

    try {
      await harness.adapter.focusImageEditorAndShowChrome('image-1')
      harness.imageEditorState.value = null
      await nextTick()

      expect(harness.setViewport).toHaveBeenCalledWith(
        { x: 12, y: 24, zoom: 0.8 },
        { duration: 220, interpolate: 'smooth' },
      )
    } finally {
      harness.scope.stop()
    }
  })
})
