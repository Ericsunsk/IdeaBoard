import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageProcessingBackend, ImageProcessingRuntime } from './types'

const { createMainThreadBackendMock, createImageProcessingRuntimeMock } =
  vi.hoisted(() => ({
    createMainThreadBackendMock: vi.fn(),
    createImageProcessingRuntimeMock: vi.fn(),
  }))

vi.mock('./imageProcessingRuntime', () => ({
  createImageProcessingRuntime: createImageProcessingRuntimeMock,
}))

vi.mock('./webglImageProcessingBackend', () => ({
  createMainThreadImageProcessingBackend: createMainThreadBackendMock,
}))

afterEach(async () => {
  const runtimeModule = await import('./retouchRuntime')
  runtimeModule.disposeImageRetouchProcessingRuntime()
  vi.restoreAllMocks()
  createMainThreadBackendMock.mockReset()
  createImageProcessingRuntimeMock.mockReset()
})

describe('image retouch processing runtime', () => {
  it('wires the WebGL main-thread backend to the image processing runtime', async () => {
    const runtimeModule = await import('./retouchRuntime')
    const backend = createBackendStub()
    const runtime = createRuntimeStub()
    createMainThreadBackendMock.mockReturnValue(backend)
    createImageProcessingRuntimeMock.mockReturnValue(runtime)

    expect(runtimeModule.getImageRetouchProcessingRuntime()).toBe(runtime)
    expect(createMainThreadBackendMock).toHaveBeenCalledWith()
    expect(createImageProcessingRuntimeMock).toHaveBeenCalledWith({ backend })
  })

  it('returns the same runtime until disposed', async () => {
    const runtimeModule = await import('./retouchRuntime')
    const runtime = createRuntimeStub()
    createImageProcessingRuntimeMock.mockReturnValue(runtime)

    const first = runtimeModule.getImageRetouchProcessingRuntime()
    const second = runtimeModule.getImageRetouchProcessingRuntime()
    expect(first).toBe(runtime)
    expect(second).toBe(runtime)
    expect(createImageProcessingRuntimeMock).toHaveBeenCalledTimes(1)

    runtimeModule.disposeImageRetouchProcessingRuntime()
    runtimeModule.getImageRetouchProcessingRuntime()
    expect(createImageProcessingRuntimeMock).toHaveBeenCalledTimes(2)
  })
})

function createBackendStub(): ImageProcessingBackend {
  return {
    renderFinal: vi.fn<ImageProcessingBackend['renderFinal']>(),
    renderPreview: vi.fn<ImageProcessingBackend['renderPreview']>(),
  }
}

function createRuntimeStub(): ImageProcessingRuntime {
  return {
    renderFinal: vi.fn<ImageProcessingRuntime['renderFinal']>(),
    renderPreview: vi.fn<ImageProcessingRuntime['renderPreview']>(),
    cancel: vi.fn<ImageProcessingRuntime['cancel']>(),
    dispose: vi.fn<ImageProcessingRuntime['dispose']>(),
  }
}
