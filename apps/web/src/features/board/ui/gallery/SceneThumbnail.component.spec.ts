import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineComponent, h } from 'vue'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { renderWithUser, waitFor } from '../../../../test/render'
import type { MediaAssetDeliveryMetadata } from '../../../../api/mediaAssetMetadata'
import { parsePreviewScene } from '../../core/preview/scene'
import SceneThumbnail from './SceneThumbnail.vue'

vi.mock('../../../../core/media/imageDimensions', () => ({
  invalidateImageNaturalSize: vi.fn(),
  readImageNaturalSize: vi.fn(async () => ({ width: 120, height: 80 })),
}))

const WORKSPACE_ID = 'ws_test123'

const mediaMocks = vi.hoisted(() => ({
  readMetadata: vi.fn(),
}))

vi.mock('../../../../api/mediaAssetMetadata', () => ({
  getWorkspaceMediaAssetMetadata: mediaMocks.readMetadata,
  ensureWorkspaceMediaInvalidation: () => () => {},
  subscribeAssetInvalidation: () => () => {},
}))

function loadScene(name: string) {
  const raw = readFileSync(
    join(
      dirname(fileURLToPath(import.meta.url)),
      '../../../../../../api/internal/boards/previewscene/testdata',
      `${name}.json`,
    ),
    'utf8',
  )
  const parsed = parsePreviewScene((JSON.parse(raw) as { scene: unknown }).scene)
  if (!parsed) throw new Error(`fixture scene failed to parse: ${name}`)
  return parsed
}

function readyMetadata(assetId: string): MediaAssetDeliveryMetadata {
  const url = `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${assetId}/derivatives/preview`
  return {
    asset_id: assetId,
    status: 'ready',
    mime_type: 'image/jpeg',
    width: 120,
    height: 80,
    size_bytes: 1024,
    original: { status: 'ready', url },
    derivatives: { preview: { status: 'ready', url } },
  }
}

function stubIntersectionObserver(isIntersecting: boolean) {
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      callback: (entries: Array<{ isIntersecting: boolean }>) => void
      constructor(callback: (entries: Array<{ isIntersecting: boolean }>) => void) {
        this.callback = callback
      }
      observe() {
        this.callback([{ isIntersecting }])
      }
      unobserve() {}
      disconnect() {}
    },
  )
}

let viewport = { width: 240, height: 180 }
const resizeObservers: Array<{
  resize: () => void
  disconnect: ReturnType<typeof vi.fn>
}> = []

function stubResizeObserver() {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      disconnect = vi.fn()
      constructor(callback: (entries: Array<{ contentRect: typeof viewport }>) => void) {
        resizeObservers.push({
          resize: () => callback([{ contentRect: viewport }]),
          disconnect: this.disconnect,
        })
      }
      observe() {
        resizeObservers.at(-1)!.resize()
      }
      unobserve() {}
    },
  )
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(() => viewport.width)
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockImplementation(() => viewport.height)
}

function resizeThumbnail(width: number, height: number) {
  viewport = { width, height }
  for (const observer of resizeObservers) observer.resize()
}

function thumbnailProps(sceneName = 'mixed', overrides: Record<string, unknown> = {}) {
  return {
    scene: loadScene(sceneName),
    workspaceId: WORKSPACE_ID,
    boardId: 'board-1',
    contentVersion: 'cv-1',
    ...overrides,
  }
}

beforeEach(() => {
  viewport = { width: 240, height: 180 }
  resizeObservers.length = 0
  stubResizeObserver()
  stubIntersectionObserver(true)
  mediaMocks.readMetadata.mockImplementation(async (_ws: string, assetId: string) =>
    readyMetadata(assetId),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
  vi.restoreAllMocks()
})

describe('SceneThumbnail', () => {
  test('renders every supported node kind from scene geometry', () => {
    const view = renderWithUser(SceneThumbnail, { props: thumbnailProps() })
    const svg = view.container.querySelector('svg')
    expect(svg?.getAttribute('viewBox')).toBe('0 0 1 0.81')
    expect(svg?.querySelectorAll('.scene-frame')).toHaveLength(3)
    expect(svg?.querySelectorAll('.scene-bar')).toHaveLength(4)
    expect(svg?.querySelectorAll('.scene-slot')).toHaveLength(2)
    expect(svg?.querySelectorAll('.scene-spoke')).toHaveLength(2)
    expect(svg?.querySelectorAll('.scene-blend')).toHaveLength(1)
    expect(svg?.querySelectorAll('.scene-edge')).toHaveLength(2)
    expect(svg?.querySelectorAll('.scene-group')).toHaveLength(1)
  })

  test('paints each raster inside its node before later overlapping frames', async () => {
    const scene = loadScene('mixed')
    const image = scene.nodes[0]!
    const prompt = scene.nodes[1]!
    prompt.x = image.x
    prompt.y = image.y
    const view = renderWithUser(SceneThumbnail, { props: thumbnailProps('mixed', { scene }) })
    await waitFor(() => expect(view.container.querySelector('image')).not.toBeNull())
    const nodes = Array.from(view.container.querySelectorAll('[data-scene-node]'))
    expect(nodes.map((node) => node.getAttribute('data-scene-node'))).toEqual([
      'image-1',
      'prompt-2',
      'fusion-4',
    ])
    expect(nodes[0]?.querySelector('image')).not.toBeNull()
    expect(nodes[1]?.querySelector('.scene-frame')).not.toBeNull()
    expect(nodes[1]?.querySelector('image')).toBeNull()
  })

  test('gives repeated Fusion media independent slot clips and mounts', async () => {
    const scene = loadScene('fusion-input-mediaref')
    const node = scene.nodes[0]!
    node.media.push({ ...node.media[0]!, index: 1 })
    const view = renderWithUser(SceneThumbnail, { props: thumbnailProps('mixed', { scene }) })
    await waitFor(() => expect(view.container.querySelectorAll('image')).toHaveLength(2))
    const clips = Array.from(view.container.querySelectorAll('clipPath')).map((clip) => clip.id)
    expect(new Set(clips).size).toBe(2)
    const images = Array.from(view.container.querySelectorAll('image'))
    expect(images[0]?.getAttribute('clip-path')).not.toBe(images[1]?.getAttribute('clip-path'))
    expect(images[0]?.getAttribute('x')).not.toBe(images[1]?.getAttribute('x'))
  })

  test('renders ready rasters for resolvable media only', async () => {
    const view = renderWithUser(SceneThumbnail, { props: thumbnailProps() })
    await waitFor(() => expect(view.container.querySelectorAll('image')).toHaveLength(1))
    const images = view.container.querySelectorAll('image')
    // mixed: image-1 output raster only; prompt simplified shapes and
    // numeric-only fusion inputs mount no rasters.
    expect(images[0]?.getAttribute('href')).toContain('asset_out_m')
  })

  test('keeps the loaded SVG visible while a replacement loads or errors', async () => {
    const view = renderWithUser(SceneThumbnail, { props: thumbnailProps() })
    await waitFor(() => expect(view.container.querySelectorAll('image')).toHaveLength(1))
    const original = view.container.querySelector('image')!
    original.dispatchEvent(new Event('load'))
    await waitFor(() => expect(original.getAttribute('opacity')).toBe('1'))
    mediaMocks.readMetadata.mockImplementation(async (_ws: string, assetId: string) => ({
      ...readyMetadata(assetId),
      derivatives: { preview: { status: 'ready', url: '/api/replacement-preview' } },
    }))
    await view.rerender({ ...thumbnailProps(), contentVersion: 'cv-2' })
    await waitFor(() => expect(view.container.querySelectorAll('image')).toHaveLength(2))
    const failed = view.container.querySelectorAll('image')[1]!
    expect(failed.getAttribute('opacity')).toBe('0')
    expect(view.container.querySelector('image')).toBe(original)
    failed.dispatchEvent(new Event('error'))
    await waitFor(() => expect(view.container.querySelectorAll('image')).toHaveLength(1))
    expect(view.container.querySelector('image')).toBe(original)
    await view.rerender({ ...thumbnailProps(), contentVersion: 'cv-3' })
    await waitFor(() => expect(view.container.querySelectorAll('image')).toHaveLength(2))
    const replacement = view.container.querySelectorAll('image')[1]!
    replacement.dispatchEvent(new Event('load'))
    await waitFor(() => expect(view.container.querySelectorAll('image')).toHaveLength(1))
    expect(view.container.querySelector('image')).toBe(replacement)
    expect(replacement.getAttribute('opacity')).toBe('1')
  })

  test('never exposes prompt text or pixel colors in the DOM', () => {
    const view = renderWithUser(SceneThumbnail, { props: thumbnailProps() })
    expect(view.container.querySelectorAll('text')).toHaveLength(0)
    expect(view.container.textContent).toBe('')
    const html = view.container.querySelector('svg')?.outerHTML ?? ''
    expect(html.toLowerCase()).not.toContain('secret')
    expect(html).not.toContain('#')
  })

  test('keeps identical geometry when only the theme changes', () => {
    const first = renderWithUser(SceneThumbnail, { props: thumbnailProps() })
    // Theme switches re-map tokens in CSS: the same scene renders the same
    // DOM under a dark host, with no reprojection and no document writes.
    const DarkHost = defineComponent({
      setup(_, { slots }) {
        return () => h('div', { class: 'dark' }, slots.default?.())
      },
    })
    const second = renderWithUser(DarkHost, {
      slots: { default: () => h(SceneThumbnail, thumbnailProps()) },
    })
    const firstSvg = first.container.querySelector('svg')?.outerHTML ?? ''
    const secondSvg = second.container.querySelector('svg')?.outerHTML ?? ''
    expect(firstSvg).not.toBe('')
    expect(secondSvg).toBe(firstSvg)
  })

  test('shows placeholders while media stays pending', async () => {
    mediaMocks.readMetadata.mockImplementation(async () => new Promise(() => {}))
    const view = renderWithUser(SceneThumbnail, { props: thumbnailProps() })
    await Promise.resolve()
    expect(view.container.querySelectorAll('image')).toHaveLength(0)
    expect(view.container.querySelectorAll('.scene-frame')).toHaveLength(3)
    expect(view.container.querySelectorAll('.scene-glyph')).not.toHaveLength(0)
  })

  test('starts no media reads while the card stays offscreen', async () => {
    stubIntersectionObserver(false)
    const view = renderWithUser(SceneThumbnail, { props: thumbnailProps() })
    await Promise.resolve()
    expect(mediaMocks.readMetadata).not.toHaveBeenCalled()
    expect(view.container.querySelectorAll('.scene-frame')).toHaveLength(3)
  })

  test('starts no media reads before positive size measurement, then loads after resize', async () => {
    viewport = { width: 0, height: 0 }
    const view = renderWithUser(SceneThumbnail, { props: thumbnailProps() })
    await Promise.resolve()
    expect(mediaMocks.readMetadata).not.toHaveBeenCalled()
    expect(view.container.querySelectorAll('.scene-frame')).toHaveLength(3)
    resizeThumbnail(240, 180)
    await waitFor(() => expect(view.container.querySelectorAll('image')).toHaveLength(1))
  })

  test('loads every eligible image and retains mounts when card size changes', async () => {
    const scene = loadScene('image-input-output')
    const image = scene.nodes[0]!
    scene.extent = { width: 1, height: 1 }
    scene.nodes = Array.from({ length: 12 }, (_, index) => ({
      ...image,
      id: `image-${index}`,
      width: index === 11 ? 0.05 : 0.2,
      height: 0.2,
      media: [{ ...image.media[0]!, asset: `asset_resize_${index}` }],
    }))
    const view = renderWithUser(SceneThumbnail, { props: thumbnailProps('mixed', { scene }) })
    await waitFor(() => expect(view.container.querySelectorAll('image')).toHaveLength(11))
    const original = view.container.querySelector('image')!
    expect(mediaMocks.readMetadata).toHaveBeenCalledTimes(11)
    resizeThumbnail(480, 360)
    await waitFor(() => expect(view.container.querySelectorAll('image')).toHaveLength(12))
    expect(view.container.querySelector('image')).toBe(original)
    expect(mediaMocks.readMetadata).toHaveBeenCalledTimes(12)
    resizeThumbnail(240, 180)
    await waitFor(() => expect(view.container.querySelectorAll('image')).toHaveLength(11))
    expect(view.container.querySelectorAll('.scene-frame')).toHaveLength(12)
    expect(view.container.querySelector('image')).toBe(original)
    expect(mediaMocks.readMetadata).toHaveBeenCalledTimes(12)
    view.unmount()
    expect(resizeObservers.every((observer) => observer.disconnect.mock.calls.length > 0)).toBe(
      true,
    )
  })

  test('cancels pending media when resize removes raster eligibility', async () => {
    const signals: AbortSignal[] = []
    mediaMocks.readMetadata.mockImplementation(
      (_ws: string, _assetId: string, options: { signal: AbortSignal }) => {
        signals.push(options.signal)
        return new Promise(() => {})
      },
    )
    const view = renderWithUser(SceneThumbnail, { props: thumbnailProps() })
    await waitFor(() => expect(signals).toHaveLength(1))
    expect(signals[0]?.aborted).toBe(false)
    resizeThumbnail(0, 0)
    await waitFor(() => expect(signals[0]?.aborted).toBe(true))
    expect(view.container.querySelectorAll('.scene-frame')).toHaveLength(3)
    expect(view.container.querySelectorAll('image')).toHaveLength(0)
  })

  test('reloads rasters when the content version changes', async () => {
    const view = renderWithUser(SceneThumbnail, { props: thumbnailProps() })
    await Promise.resolve()
    expect(mediaMocks.readMetadata).toHaveBeenCalledTimes(1)

    await view.rerender({ ...thumbnailProps(), contentVersion: 'cv-2' })
    await Promise.resolve()
    expect(mediaMocks.readMetadata).toHaveBeenCalledTimes(2)
  })

  test('renders the empty layout state without a viewBox', () => {
    const view = renderWithUser(SceneThumbnail, { props: thumbnailProps('empty') })
    expect(view.container.querySelector('svg')).toBeNull()
    expect(view.container.querySelector('.scene-empty')).not.toBeNull()
  })
})
