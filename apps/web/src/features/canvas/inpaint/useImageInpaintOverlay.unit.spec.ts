import type { Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import { computed, ref, shallowRef } from 'vue'
import type { Ref } from 'vue'
import { useRetouchMode } from '../retouch/useRetouchMode'
import { useImageInpaintOverlay } from './useImageInpaintOverlay'

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
  const editorMode = useRetouchMode({
    nodes,
    activeTool: ref('select'),
    getImageNodeDisplayPayload: (nodeId) =>
      nodeId === 'node-1' ? { url: 'https://example.com/source.png', ratio: 1 } : null,
  })
  const overlay = useImageInpaintOverlay({
    inpaintEditorState: computed(() => {
      const state = editorMode.retouchState.value
      return state?.mode === 'inpaint' ? { nodeId: state.nodeId, mode: 'inpaint' as const } : null
    }),
    imageRect: computed(() => ({ left: 40, top: 0, width: stage.clientWidth - 80, height: 240 })),
  })

  return { editorMode, overlay }
}

function createMaskCanvas() {
  const canvas = document.createElement('canvas')
  canvas.width = 200
  canvas.height = 100
  const imageData = {
    width: canvas.width,
    height: canvas.height,
    data: new Uint8ClampedArray(canvas.width * canvas.height * 4),
  } as ImageData
  const ctx = {
    clearRect: vi.fn(),
    drawImage: vi.fn(),
    getImageData: vi.fn(() => imageData),
    putImageData: vi.fn(),
    save: vi.fn(),
    restore: vi.fn(),
    beginPath: vi.fn(),
    closePath: vi.fn(),
    fill: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    stroke: vi.fn(),
    createImageData: vi.fn((width: number, height: number) => ({
      width,
      height,
      data: new Uint8ClampedArray(width * height * 4),
    })),
    globalCompositeOperation: 'source-over',
    fillStyle: '',
    strokeStyle: '',
    lineCap: 'round',
    lineJoin: 'round',
    lineWidth: 0,
    canvas,
  } as unknown as CanvasRenderingContext2D
  vi.spyOn(canvas, 'getContext').mockReturnValue(ctx)
  canvas.hasPointerCapture = vi.fn(() => false)
  canvas.setPointerCapture = vi.fn()
  canvas.releasePointerCapture = vi.fn()
  return canvas
}

describe('useImageInpaintOverlay', () => {
  it('keeps mask editor active when committing without a drawable mask', () => {
    const { editorMode, overlay } = createHarness()

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    expect(overlay.inpaintNodeId.value).toBe('node-1')

    expect(overlay.commitMaskDraft()).toBe(false)

    expect(editorMode.retouchState.value).not.toBeNull()
    expect(overlay.inpaintNodeId.value).toBe('node-1')
  })

  it('reuses the pointer-down canvas layout while painting', () => {
    const { editorMode, overlay } = createHarness()
    const canvas = createMaskCanvas()
    const readRect = vi.fn(() => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect)
    canvas.getBoundingClientRect = readRect

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    overlay.setMaskCanvasRef('node-1', canvas)
    overlay.onCanvasPointerDown(
      new PointerEvent('pointerdown', {
        button: 0,
        clientX: 60,
        clientY: 45,
        pointerId: 1,
        pointerType: 'mouse',
      }),
    )
    window.dispatchEvent(
      new PointerEvent('pointermove', {
        clientX: 70,
        clientY: 50,
        pointerId: 1,
        pointerType: 'mouse',
      }),
    )

    expect(readRect).toHaveBeenCalledTimes(1)
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))
  })
})
