import { describe, expect, it, vi } from 'vitest'
import { createImageProcessingRuntime } from './ImageProcessingRuntime'
import type { ImageProcessingBackend } from './types'

describe('ImageProcessingRuntime', () => {
  it('routes final work through the injected backend with task context', async () => {
    const renderFinal = vi.fn<ImageProcessingBackend['renderFinal']>(async ({ context }) => {
      context.reportProgress({ phase: 'render' })
      return {
        file: new File(['final'], `final-${context.revision}.webp`, { type: 'image/webp' }),
        width: 10,
        height: 20,
        mimeType: 'image/webp',
      }
    })
    const onProgress = vi.fn()
    const runtime = createImageProcessingRuntime({
      backend: createBackend({ renderFinal }),
    })

    const result = await runtime.renderFinal({
      source: { url: 'https://example.com/source.png' },
      plan: [],
      taskId: 'apply',
      onProgress,
    })

    expect(result).toMatchObject({
      status: 'completed',
      taskId: 'apply',
      revision: 1,
      output: {
        file: expect.objectContaining({ name: 'final-1.webp' }),
        width: 10,
        height: 20,
      },
    })
    expect(renderFinal).toHaveBeenCalledWith(
      expect.objectContaining({
        source: { url: 'https://example.com/source.png' },
        plan: [],
      }),
    )
    expect(onProgress).toHaveBeenCalledWith({ phase: 'render' })
  })

  it('keeps final tasks non-latest by default', async () => {
    const renderFinal = vi.fn<ImageProcessingBackend['renderFinal']>(async ({ context }) => ({
      file: new File(['final'], `final-${context.revision}.webp`, { type: 'image/webp' }),
    }))
    const runtime = createImageProcessingRuntime({
      backend: createBackend({ renderFinal }),
    })

    const first = runtime.renderFinal({
      source: { url: 'https://example.com/a.png' },
      plan: [],
      taskId: 'apply',
    })
    const second = runtime.renderFinal({
      source: { url: 'https://example.com/b.png' },
      plan: [],
      taskId: 'apply',
    })

    await expect(first).resolves.toMatchObject({
      status: 'completed',
      revision: 1,
    })
    await expect(second).resolves.toMatchObject({
      status: 'completed',
      revision: 2,
    })
    expect(renderFinal).toHaveBeenCalledTimes(2)
  })

  it('marks older final tasks stale when latestOnly is requested', async () => {
    const resolvers: Array<(file: File) => void> = []
    const renderFinal = vi.fn<ImageProcessingBackend['renderFinal']>(
      async ({ context }) =>
        new Promise((resolve) => {
          resolvers.push((file) => resolve({ file }))
          context.reportProgress({ phase: 'render' })
        }),
    )
    const runtime = createImageProcessingRuntime({
      backend: createBackend({ renderFinal }),
    })

    const first = runtime.renderFinal({
      source: { url: 'https://example.com/a.png' },
      plan: [],
      taskId: 'apply',
      latestOnly: true,
    })
    await Promise.resolve()

    const second = runtime.renderFinal({
      source: { url: 'https://example.com/b.png' },
      plan: [],
      taskId: 'apply',
      latestOnly: true,
    })
    await Promise.resolve()

    await expect(first).resolves.toMatchObject({
      status: 'stale',
      taskId: 'apply',
      revision: 1,
    })
    resolvers[0]?.(new File(['old'], 'old.webp', { type: 'image/webp' }))
    resolvers[1]?.(new File(['new'], 'new.webp', { type: 'image/webp' }))
    await expect(second).resolves.toMatchObject({
      status: 'completed',
      taskId: 'apply',
      revision: 2,
      output: {
        file: expect.objectContaining({ name: 'new.webp' }),
      },
    })
    expect(renderFinal).toHaveBeenCalledTimes(2)
  })

  it('cancels active final work by task id', async () => {
    const captured: { signal?: AbortSignal } = {}
    const cancel = vi.fn()
    const renderFinal = vi.fn<ImageProcessingBackend['renderFinal']>(
      async ({ context }) =>
        new Promise((resolve) => {
          captured.signal = context.signal
          void resolve
        }),
    )
    const runtime = createImageProcessingRuntime({
      backend: createBackend({ renderFinal, cancel }),
    })

    const task = runtime.renderFinal({
      source: { url: 'https://example.com/source.png' },
      plan: [],
      taskId: 'apply',
    })
    await Promise.resolve()

    runtime.cancel('apply')

    expect(captured.signal?.aborted).toBe(true)
    await expect(task).resolves.toMatchObject({
      status: 'cancelled',
      taskId: 'apply',
    })
    expect(cancel).toHaveBeenCalledWith('apply')
  })

  it('maps backend failures into structured runtime results', async () => {
    const runtime = createImageProcessingRuntime({
      backend: createBackend({
        renderFinal: vi.fn<ImageProcessingBackend['renderFinal']>(async () => {
          throw new Error('render exploded')
        }),
      }),
    })

    await expect(
      runtime.renderFinal({
        source: { url: 'https://example.com/source.png' },
        plan: [],
      }),
    ).resolves.toMatchObject({
      status: 'failed',
      error: {
        code: 'backend-failed',
        message: 'render exploded',
      },
    })
  })

  it('does not pass through unknown structured error codes', async () => {
    const runtime = createImageProcessingRuntime({
      backend: createBackend({
        renderFinal: vi.fn<ImageProcessingBackend['renderFinal']>(async () => {
          throw {
            code: 'legacy-code',
            message: 'legacy failure',
          }
        }),
      }),
    })

    await expect(
      runtime.renderFinal({
        source: { url: 'https://example.com/source.png' },
        plan: [],
      }),
    ).resolves.toMatchObject({
      status: 'failed',
      error: {
        code: 'unknown',
        message: 'Image processing failed.',
      },
    })
  })

  it('routes preview work through the injected backend as latest-only by default', async () => {
    const renderPreview = vi.fn<ImageProcessingBackend['renderPreview']>(
      async ({ sizeStrategy }) => ({
        sourceImageData: makeImageData([10, 20, 30, 255]),
        previewImageData: makeImageData([245, 235, 225, 255]),
        width: sizeStrategy?.maxSide ?? 1,
        height: 1,
      }),
    )
    const runtime = createImageProcessingRuntime({
      backend: createBackend({ renderPreview }),
    })

    const result = await runtime.renderPreview({
      source: { url: 'https://example.com/source.png' },
      plan: [],
      taskId: 'preview',
      sizeStrategy: { maxSide: 960 },
    })

    expect(result).toMatchObject({
      status: 'completed',
      taskId: 'preview',
      output: {
        sourceImageData: expect.objectContaining({
          data: new Uint8ClampedArray([10, 20, 30, 255]),
        }),
        previewImageData: expect.objectContaining({
          data: new Uint8ClampedArray([245, 235, 225, 255]),
        }),
        width: 960,
        height: 1,
      },
    })
    expect(renderPreview).toHaveBeenCalledWith(
      expect.objectContaining({
        source: { url: 'https://example.com/source.png' },
        plan: [],
        sizeStrategy: { maxSide: 960 },
      }),
    )
  })
})

function createBackend(input: Partial<ImageProcessingBackend> = {}): ImageProcessingBackend {
  return {
    renderFinal:
      input.renderFinal ??
      vi.fn<ImageProcessingBackend['renderFinal']>(async () => ({
        file: new File(['final'], 'final.webp', { type: 'image/webp' }),
      })),
    renderPreview:
      input.renderPreview ??
      vi.fn<ImageProcessingBackend['renderPreview']>(async () => ({
        sourceImageData: makeImageData([0, 0, 0, 255]),
        previewImageData: makeImageData([0, 0, 0, 255]),
        width: 1,
        height: 1,
      })),
    cancel: input.cancel,
    dispose: input.dispose,
  }
}

function makeImageData(values: number[]): ImageData {
  class TestImageDataImpl {
    data: Uint8ClampedArray
    width: number
    height: number

    constructor(data: Uint8ClampedArray, width: number, height: number) {
      this.data = data
      this.width = width
      this.height = height
    }
  }
  return new TestImageDataImpl(new Uint8ClampedArray(values), values.length / 4, 1) as ImageData
}
