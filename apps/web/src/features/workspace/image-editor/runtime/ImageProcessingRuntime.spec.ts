import { describe, expect, it, vi } from 'vitest'
import { createImageProcessingRuntime } from './ImageProcessingRuntime'
import type { ImageProcessingBackend } from './types'

describe('ImageProcessingRuntime', () => {
  it('routes preview work through the injected backend with task context', async () => {
    const renderPreview = vi.fn<ImageProcessingBackend['renderPreview']>(async ({ context }) => {
      context.reportProgress({ phase: 'render' })
      return {
        objectUrl: `blob:${context.revision}`,
        width: 10,
        height: 20,
        mimeType: 'image/webp',
      }
    })
    const onProgress = vi.fn()
    const runtime = createImageProcessingRuntime({
      backend: {
        renderPreview,
        renderFinal: vi.fn<ImageProcessingBackend['renderFinal']>(),
      },
    })

    const result = await runtime.renderPreview({
      source: { url: 'https://example.com/source.png' },
      plan: [],
      taskId: 'preview',
      onProgress,
    })

    expect(result).toMatchObject({
      status: 'completed',
      taskId: 'preview',
      revision: 1,
      output: {
        objectUrl: 'blob:1',
        width: 10,
        height: 20,
      },
    })
    expect(renderPreview).toHaveBeenCalledWith(
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
      backend: {
        renderPreview: vi.fn<ImageProcessingBackend['renderPreview']>(),
        renderFinal,
      },
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
      backend: {
        renderPreview: vi.fn<ImageProcessingBackend['renderPreview']>(),
        renderFinal,
      },
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
      backend: {
        renderPreview: vi.fn<ImageProcessingBackend['renderPreview']>(),
        renderFinal,
        cancel,
      },
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
      backend: {
        renderPreview: vi.fn<ImageProcessingBackend['renderPreview']>(async () => {
          throw new Error('render exploded')
        }),
        renderFinal: vi.fn<ImageProcessingBackend['renderFinal']>(),
      },
    })

    await expect(
      runtime.renderPreview({
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
      backend: {
        renderPreview: vi.fn<ImageProcessingBackend['renderPreview']>(async () => {
          throw {
            code: 'legacy-code',
            message: 'legacy failure',
          }
        }),
        renderFinal: vi.fn<ImageProcessingBackend['renderFinal']>(),
      },
    })

    await expect(
      runtime.renderPreview({
        source: { url: 'https://example.com/source.png' },
        plan: [],
      }),
    ).resolves.toMatchObject({
      status: 'failed',
      error: {
        code: 'unknown',
        message: '图像处理失败',
      },
    })
  })

  it('cancels scheduled runtime work and notifies the backend', async () => {
    vi.useFakeTimers()
    try {
      const cancel = vi.fn()
      const runtime = createImageProcessingRuntime({
        previewDebounceMs: 50,
        backend: {
          renderPreview: vi.fn<ImageProcessingBackend['renderPreview']>(async () => ({
            objectUrl: 'blob:preview',
          })),
          renderFinal: vi.fn<ImageProcessingBackend['renderFinal']>(),
          cancel,
        },
      })

      const task = runtime.renderPreview({
        source: { url: 'https://example.com/source.png' },
        plan: [],
        taskId: 'preview',
      })
      runtime.cancel('preview')

      await expect(task).resolves.toMatchObject({
        status: 'cancelled',
        taskId: 'preview',
      })
      expect(cancel).toHaveBeenCalledWith('preview')
    } finally {
      vi.useRealTimers()
    }
  })
})
