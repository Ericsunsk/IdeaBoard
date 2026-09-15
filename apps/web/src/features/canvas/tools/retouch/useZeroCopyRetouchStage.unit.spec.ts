import { beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import {
  isUsableTextureImage,
  resolveDomTextureImage,
  useZeroCopyRetouchStage,
} from './useZeroCopyRetouchStage.ts'

const rendererMocks = vi.hoisted(() => {
  const instances: Array<{
    updateSourceImage: ReturnType<typeof vi.fn>
    updateUniforms: ReturnType<typeof vi.fn>
    render: ReturnType<typeof vi.fn>
    readPixels: ReturnType<typeof vi.fn>
    readHistogramSample: ReturnType<typeof vi.fn>
    dispose: ReturnType<typeof vi.fn>
    isContextLost: ReturnType<typeof vi.fn>
  }> = []
  return {
    instances,
    latest: () => instances[instances.length - 1]!,
  }
})

vi.mock('./processing/webglEngine.ts', async (importOriginal) => {
  const original = await importOriginal<typeof import('./processing/webglEngine.ts')>()
  return {
    ...original,
    isWebGLRetouchSupported: () => true,
    createWebGLRetouchRenderer: () => {
      const instance = {
        updateSourceImage: vi.fn(),
        updateUniforms: vi.fn(),
        render: vi.fn(),
        readPixels: vi.fn(),
        readHistogramSample: vi.fn(),
        dispose: vi.fn(),
        isContextLost: vi.fn(() => false),
      }
      rendererMocks.instances.push(instance)
      return instance
    },
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
    rendererMocks.instances.length = 0
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

    expect(rendererMocks.latest().updateSourceImage).toHaveBeenCalledTimes(1)
    expect(handle.stageStatus.value).toBe('ready')
    expect(handle.isGpuActive.value).toBe(true)
    scope.stop()
  })

  it('updates uniforms with zero readbacks while layers change', async () => {
    const { scope, layers } = mountStage(readyImage())
    await Promise.resolve()
    rendererMocks.latest().readPixels.mockClear()
    rendererMocks.latest().updateUniforms.mockClear()

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

    expect(rendererMocks.latest().updateUniforms).toHaveBeenCalled()
    expect(rendererMocks.latest().render).toHaveBeenCalled()
    expect(rendererMocks.latest().readPixels).not.toHaveBeenCalled()
    scope.stop()
  })

  it('stays pending instead of failing when no DOM source image exists yet', async () => {
    const { scope, handle } = mountStage(null)
    await Promise.resolve()

    expect(rendererMocks.latest().updateSourceImage).not.toHaveBeenCalled()
    expect(handle.stageStatus.value).toBe('idle')
    expect(handle.isGpuActive.value).toBe(false)
    scope.stop()
  })

  it('retries the bind on the next layer change once the DOM image decodes', async () => {
    let domImage: HTMLImageElement | null = null
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
    await Promise.resolve()
    expect(rendererMocks.latest().updateSourceImage).not.toHaveBeenCalled()
    expect(handle.stageStatus.value).toBe('idle')

    // The node image finishes decoding, then the user drags a slider.
    const decoded = readyImage()
    domImage = decoded
    layers.value = [
      {
        id: 'hsl:1',
        type: 'effect',
        kind: 'hsl',
        name: 'HSL',
        visible: true,
        params: { hue: 10, saturation: 100, lightness: 5 },
      },
    ] as never[]
    await Promise.resolve()
    vi.advanceTimersByTime(32)

    expect(rendererMocks.latest().updateSourceImage).toHaveBeenCalledTimes(1)
    expect(rendererMocks.latest().updateSourceImage).toHaveBeenCalledWith(decoded, 800, 600)
    expect(rendererMocks.latest().render).toHaveBeenCalled()
    expect(handle.stageStatus.value).toBe('ready')
    scope.stop()
  })

  it('rebuilds the renderer when the stage canvas is replaced', async () => {
    const { scope, handle, layers } = mountStage(readyImage())
    await Promise.resolve()
    expect(rendererMocks.instances).toHaveLength(1)
    expect(rendererMocks.latest().updateSourceImage).toHaveBeenCalledTimes(1)
    const firstRenderer = rendererMocks.latest()

    // Node switch / remount replaces the stage canvas: draws must follow it
    // instead of going to the detached canvas.
    handle.setCanvasRef(document.createElement('canvas'))
    await Promise.resolve()
    vi.advanceTimersByTime(32)

    expect(rendererMocks.instances).toHaveLength(2)
    expect(firstRenderer.dispose).toHaveBeenCalledTimes(1)
    expect(rendererMocks.latest().updateSourceImage).toHaveBeenCalledTimes(1)
    expect(handle.stageStatus.value).toBe('ready')

    // Later draws go to the new renderer only.
    firstRenderer.render.mockClear()
    rendererMocks.latest().render.mockClear()
    layers.value = [
      {
        id: 'hsl:1',
        type: 'effect',
        kind: 'hsl',
        name: 'HSL',
        visible: true,
        params: { hue: 10, saturation: 100, lightness: 5 },
      },
    ] as never[]
    await Promise.resolve()
    vi.advanceTimersByTime(32)

    expect(rendererMocks.latest().render).toHaveBeenCalled()
    expect(firstRenderer.render).not.toHaveBeenCalled()
    scope.stop()
  })
})
