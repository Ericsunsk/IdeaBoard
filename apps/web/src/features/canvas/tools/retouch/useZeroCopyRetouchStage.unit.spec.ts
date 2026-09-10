import { beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import {
  isUsableTextureImage,
  resolveDomTextureImage,
  useZeroCopyRetouchStage,
} from './useZeroCopyRetouchStage.ts'

const rendererMocks = vi.hoisted(() => ({
  updateSourceImage: vi.fn(),
  updateUniforms: vi.fn(),
  render: vi.fn(),
  readPixels: vi.fn(),
  readHistogramSample: vi.fn(),
  dispose: vi.fn(),
  isContextLost: vi.fn(() => false),
}))

vi.mock('./processing/webglEngine.ts', async (importOriginal) => {
  const original = await importOriginal<typeof import('./processing/webglEngine.ts')>()
  return {
    ...original,
    isWebGLRetouchSupported: () => true,
    createWebGLRetouchRenderer: () => ({ ...rendererMocks }),
  }
})

function readyImage(width = 800, height = 600): HTMLImageElement {
  const image = document.createElement('img')
  Object.defineProperties(image, {
    complete: { value: true },
    naturalWidth: { value: width },
    naturalHeight: { value: height },
  })
  return image
}

describe('resolveDomTextureImage', () => {
  it('returns null without a stage element', () => {
    expect(resolveDomTextureImage(null)).toBeNull()
    expect(resolveDomTextureImage(undefined)).toBeNull()
  })

  it('prefers the largest decoded image and skips incomplete ones', () => {
    const stage = document.createElement('div')
    const small = readyImage(100, 100)
    const large = readyImage(800, 600)
    const broken = document.createElement('img')
    stage.append(small, broken, large)
    expect(resolveDomTextureImage(stage)).toBe(large)
  })

  it('rejects images without decoded size', () => {
    expect(isUsableTextureImage(null)).toBe(false)
    expect(isUsableTextureImage(document.createElement('img'))).toBe(false)
    expect(isUsableTextureImage(readyImage())).toBe(true)
  })
})

describe('useZeroCopyRetouchStage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
  })

  function mountStage(domImage: HTMLImageElement | null) {
    const scope = effectScope()
    const isAdjustMode = ref(true)
    const layers = ref([
      {
        id: 'hsl:1',
        type: 'effect',
        kind: 'hsl',
        name: 'HSL',
        visible: true,
        params: { hue: 10, saturation: 20, lightness: 5 },
      },
    ] as never[])
    const handle = scope.run(() =>
      useZeroCopyRetouchStage({
        isAdjustMode,
        layers: layers as never,
        resolveDomImage: () => domImage,
      }),
    )
    if (!handle) throw new Error('stage was not created')
    handle.setCanvasRef(document.createElement('canvas'))
    return { scope, handle, isAdjustMode, layers }
  }

  it('binds the DOM image immediately without network reads', async () => {
    const { scope, handle } = mountStage(readyImage())
    await Promise.resolve()

    expect(rendererMocks.updateSourceImage).toHaveBeenCalledTimes(1)
    expect(handle.stageStatus.value).toBe('ready')
    expect(handle.isGpuActive.value).toBe(true)
    scope.stop()
  })

  it('updates uniforms with zero readbacks while layers change', async () => {
    const { scope, layers } = mountStage(readyImage())
    await Promise.resolve()
    rendererMocks.readPixels.mockClear()
    rendererMocks.updateUniforms.mockClear()

    layers.value = [
      {
        id: 'hsl:1',
        type: 'effect',
        kind: 'hsl',
        name: 'HSL',
        visible: true,
        params: { hue: 30, saturation: 40, lightness: 5 },
      },
    ] as never[]
    await Promise.resolve()
    vi.advanceTimersByTime(32)

    expect(rendererMocks.updateUniforms).toHaveBeenCalled()
    expect(rendererMocks.render).toHaveBeenCalled()
    expect(rendererMocks.readPixels).not.toHaveBeenCalled()
    scope.stop()
  })

  it('transitions to error without fallback when no DOM source image exists', async () => {
    const { scope, handle } = mountStage(null)
    await Promise.resolve()

    expect(handle.stageStatus.value).toBe('error')
    expect(handle.isGpuActive.value).toBe(false)
    scope.stop()
  })
})
