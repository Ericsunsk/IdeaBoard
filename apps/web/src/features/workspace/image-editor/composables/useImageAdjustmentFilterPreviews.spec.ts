import { nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageEditorLayer } from '../types'
import {
  cancelImageAdjustmentPreviewRender,
  renderImageAdjustmentPreview,
} from '../services/imageAdjustmentPreviewRenderer'
import { useImageAdjustmentFilterPreviews } from './useImageAdjustmentFilterPreviews'

vi.mock('../services/imageAdjustmentPreviewRenderer', () => ({
  IMAGE_ADJUSTMENT_PREVIEW_TASK_ID: 'image-adjustment-preview',
  cancelImageAdjustmentPreviewRender: vi.fn(),
  renderImageAdjustmentPreview: vi.fn(),
}))

const cancelPreviewRenderMock = vi.mocked(cancelImageAdjustmentPreviewRender)
const renderPreviewMock = vi.mocked(renderImageAdjustmentPreview)
const completedPreviewResult = (objectUrl: string | null) => ({
  status: 'completed' as const,
  taskId: 'image-adjustment-preview',
  revision: 1,
  output: { objectUrl },
})
const stalePreviewResult = (revision: number) => ({
  status: 'stale' as const,
  taskId: 'image-adjustment-preview',
  revision,
  error: { code: 'stale' as const, message: 'stale preview render' },
})

function createHslLayer(params = { hue: 0, saturation: 0, brightness: 0, contrast: 0 }) {
  return {
    id: 'adjustment:hsl',
    type: 'adjustment',
    kind: 'hsl',
    name: '色相/饱和度 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

function createThresholdLayer(params = { threshold: 128, invert: false }) {
  return {
    id: 'adjustment:threshold',
    type: 'adjustment',
    kind: 'threshold',
    name: '阈值 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

function createCurvesLayer(params = { input: 50, output: 50 }) {
  return {
    id: 'adjustment:curves',
    type: 'adjustment',
    kind: 'curves',
    name: '曲线 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

function createLevelsLayer(params = { shadows: 0, midtones: 50, highlights: 100 }) {
  return {
    id: 'adjustment:levels',
    type: 'adjustment',
    kind: 'levels',
    name: '色阶 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

function createInvertLayer(params = { enabled: true }) {
  return {
    id: 'adjustment:invert',
    type: 'adjustment',
    kind: 'invert',
    name: '反相 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

async function flushPreviewPromises() {
  await vi.runOnlyPendingTimersAsync()
  await Promise.resolve()
  await nextTick()
}

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
  cancelPreviewRenderMock.mockReset()
  renderPreviewMock.mockReset()
})

describe('useImageAdjustmentFilterPreviews', () => {
  it('renders effective adjustments through the compiled image adjustment plan', async () => {
    vi.useFakeTimers()
    renderPreviewMock.mockResolvedValue(completedPreviewResult('blob:preview'))
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers: ref([
        createHslLayer({ hue: 90, saturation: 25, brightness: -10, contrast: 15 }),
        createCurvesLayer(),
        createLevelsLayer(),
        createInvertLayer({ enabled: false }),
      ]),
    })

    await flushPreviewPromises()

    expect(renderPreviewMock).toHaveBeenCalledTimes(1)
    expect(renderPreviewMock).toHaveBeenCalledWith(
      'https://example.com/source.png',
      expect.arrayContaining([expect.objectContaining({ kind: 'hsl' })]),
      { taskId: 'image-adjustment-preview', debounceMs: 80 },
    )
    expect(preview.imageAdjustmentFilterPreviews.value).toEqual([
      { type: 'rendered-image', url: 'blob:preview' },
    ])
  })

  it('does not render hidden, inactive, or neutral adjustment stacks', async () => {
    vi.useFakeTimers()
    const layers = ref<ImageEditorLayer[]>([
      { ...createHslLayer(), visible: false },
      createCurvesLayer(),
      createLevelsLayer(),
      createInvertLayer({ enabled: false }),
    ])
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers,
    })

    await flushPreviewPromises()

    expect(preview.imageAdjustmentPlan.value).toEqual([])
    expect(preview.imageAdjustmentFilterPreviews.value).toEqual([])
    expect(renderPreviewMock).not.toHaveBeenCalled()
    expect(cancelPreviewRenderMock).toHaveBeenCalledWith('image-adjustment-preview')
  })

  it('normalizes non-finite params before rendering previews', async () => {
    vi.useFakeTimers()
    renderPreviewMock.mockResolvedValue(completedPreviewResult('blob:preview'))
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers: ref([
        createThresholdLayer({ threshold: Number.NaN, invert: false }),
        createHslLayer({
          hue: Number.NaN,
          saturation: Number.POSITIVE_INFINITY,
          brightness: Number.NEGATIVE_INFINITY,
          contrast: Number.NaN,
        }),
      ]),
    })

    await flushPreviewPromises()

    expect(preview.imageAdjustmentPlan.value).toEqual([
      { kind: 'threshold', threshold: 128, invert: false },
    ])
    expect(preview.imageAdjustmentFilterPreviews.value).toEqual([
      { type: 'rendered-image', url: 'blob:preview' },
    ])
  })

  it('revokes stale rendered preview object URLs', async () => {
    vi.useFakeTimers()
    const revokeSpy = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
    renderPreviewMock
      .mockResolvedValueOnce(completedPreviewResult('blob:first-preview'))
      .mockResolvedValueOnce(completedPreviewResult('blob:second-preview'))
    const layers = ref<ImageEditorLayer[]>([createInvertLayer()])
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers,
    })

    await flushPreviewPromises()
    expect(preview.imageAdjustmentFilterPreviews.value).toEqual([
      { type: 'rendered-image', url: 'blob:first-preview' },
    ])

    layers.value = [createThresholdLayer()]
    await nextTick()
    await flushPreviewPromises()

    expect(revokeSpy).toHaveBeenCalledWith('blob:first-preview')
    expect(preview.imageAdjustmentFilterPreviews.value).toEqual([
      { type: 'rendered-image', url: 'blob:second-preview' },
    ])
  })

  it('delegates rapid adjustment changes to the runtime scheduler and keeps the latest preview', async () => {
    vi.useFakeTimers()
    renderPreviewMock
      .mockResolvedValueOnce(stalePreviewResult(1))
      .mockResolvedValueOnce(stalePreviewResult(2))
      .mockResolvedValueOnce(completedPreviewResult('blob:latest-preview'))
    const layers = ref<ImageEditorLayer[]>([createInvertLayer()])
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers,
    })

    layers.value = [createThresholdLayer({ threshold: 32, invert: false })]
    await nextTick()
    layers.value = [createThresholdLayer({ threshold: 224, invert: true })]
    await nextTick()
    await flushPreviewPromises()

    expect(renderPreviewMock).toHaveBeenCalledTimes(3)
    expect(renderPreviewMock).toHaveBeenLastCalledWith(
      'https://example.com/source.png',
      [{ kind: 'threshold', threshold: 224, invert: true }],
      { taskId: 'image-adjustment-preview', debounceMs: 80 },
    )
    expect(preview.imageAdjustmentFilterPreviews.value).toEqual([
      { type: 'rendered-image', url: 'blob:latest-preview' },
    ])
  })

  it('clears rendered preview URLs when the stack becomes neutral', async () => {
    vi.useFakeTimers()
    const revokeSpy = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
    renderPreviewMock.mockResolvedValue(completedPreviewResult('blob:preview'))
    const layers = ref<ImageEditorLayer[]>([createInvertLayer()])
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers,
    })

    await flushPreviewPromises()
    layers.value = [createInvertLayer({ enabled: false })]
    await nextTick()

    expect(revokeSpy).toHaveBeenCalledWith('blob:preview')
    expect(preview.imageAdjustmentFilterPreviews.value).toEqual([])
    expect(cancelPreviewRenderMock).toHaveBeenCalledWith('image-adjustment-preview')
  })

  it('keeps preview state empty when async preview rendering fails', async () => {
    vi.useFakeTimers()
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    renderPreviewMock.mockRejectedValue(new Error('preview failed'))
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers: ref([createInvertLayer()]),
    })

    await flushPreviewPromises()

    expect(preview.imageAdjustmentFilterPreviews.value).toEqual([])
    expect(warnSpy).toHaveBeenCalledWith(
      '[image-editor] failed to render adjustment preview',
      expect.any(Error),
    )
  })
})
