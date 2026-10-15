import { mediaDisplayFixture } from '../../../test/mediaDisplay'
import { describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { render, within } from '../../../test/render'
import '../../../app/style.css'
import LibraryAssetGallery from './LibraryAssetGallery.vue'
import type { LibraryAssetView } from '../core/transfer'

vi.mock('../../../core/config/runtimeUrls', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../core/config/runtimeUrls')>()),
  resolveApiUrl: () =>
    'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" width="1" height="1"/%3E',
}))

const image =
  'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" width="1" height="1"/%3E'
function asset(index: number, height = 600): LibraryAssetView {
  return {
    id: `lasset_browser${index}`,
    workspaceId: 'ws_test0001',
    provider: 'unsplash',
    sourceId: `photo-${index}`,
    sourceUrl: 'https://example.com/source',
    previewUrl: image,
    originalUrl: image,
    title: `Reference ${index}`,
    author: 'IdeaBoard',
    width: 800,
    height,
    createdAt: '2026-09-24T00:00:00Z',
    display: mediaDisplayFixture(),
    updatedAt: '2026-09-24T00:00:00Z',
  }
}
const propsFor = (assets: LibraryAssetView[], isLoading = false) => ({
  assets,
  isLoading: isLoading,
  errorMessage: null,
  hasMore: true,
  isExpanded: false,
  isCanvasPresentation: false,
  windowWidth: 1200,
})

function cards(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll('article'))
}

function assertRoundRobinLayout(container: HTMLElement, count: number, gap: number) {
  const articles = cards(container)
  const grid = articles[0]!.parentElement!
  const bounds = grid.getBoundingClientRect()
  const width = (bounds.width - (count - 1) * gap) / count
  const bottoms = Array<number>(count).fill(bounds.top)
  for (const [index, article] of articles.entries()) {
    const lane = index % count
    const rect = article.getBoundingClientRect()
    expect(rect.width).toBeCloseTo(width, 0)
    expect(rect.left).toBeCloseTo(bounds.left + lane * (width + gap), 0)
    expect(rect.top).toBeCloseTo(bottoms[lane]!, 0)
    bottoms[lane] = rect.bottom + gap
  }
  expect(bounds.height).toBeCloseTo(Math.max(...bottoms) - bounds.top - gap, 0)
  const loadMore = within(container).getByRole('button', { name: 'Load more' })
  expect(loadMore.getBoundingClientRect().top).toBeGreaterThanOrEqual(bounds.bottom)
}

describe('LibraryAssetGallery browser contract', () => {
  function renderGallery(assets: LibraryAssetView[]) {
    const root = document.createElement('div')
    root.style.cssText = 'width: 600px; height: 400px;'
    document.body.append(root)
    return { root, ...render(LibraryAssetGallery, { container: root, props: propsFor(assets) }) }
  }

  it('preserves the moving card DOM, local state and real focus when lanes change', async () => {
    await page.viewport(1400, 800)
    const assets = Array.from({ length: 9 }, (_, index) => asset(index))
    const { root, rerender, unmount } = renderGallery(assets)
    try {
      await expect.poll(() => assertRoundRobinLayout(root, 2, 8)).toBeUndefined()
      const article = cards(root)[3]!
      const viewer = within(article).getByRole('button', { name: 'View Reference 3' })
      const localState = { draft: 'asset-local browser state' }
      Object.assign(viewer, { localState })
      viewer.focus()
      expect(document.activeElement).toBe(viewer)
      const previousLeft = article.getBoundingClientRect().left
      await rerender({ ...propsFor(assets), isExpanded: true, windowWidth: 1400 })
      await expect.poll(() => assertRoundRobinLayout(root, 6, 12)).toBeUndefined()
      expect(cards(root)[3]).toBe(article)
      expect(article.getBoundingClientRect().left).not.toBe(previousLeft)
      const preserved = within(article).getByRole('button', { name: 'View Reference 3' })
      expect(preserved).toBe(viewer)
      expect(Object.getOwnPropertyDescriptor(preserved, 'localState')?.value).toBe(localState)
      expect(document.activeElement).toBe(viewer)
    } finally {
      unmount()
      root.remove()
    }
  })

  it('retains round-robin responsive lanes, unequal heights and fallback clearance after resize', async () => {
    await page.viewport(1700, 800)
    const assets = [80, 1200, 600, 1600, 300, 800, 80, 600, 1400].map((height, index) =>
      asset(index, height),
    )
    const { root, rerender, unmount } = renderGallery(assets)
    try {
      await expect.poll(() => assertRoundRobinLayout(root, 2, 8)).toBeUndefined()
      const failedCard = cards(root)[0]!
      within(failedCard).getByRole('img').dispatchEvent(new Event('error'))
      await expect.poll(() => assertRoundRobinLayout(root, 2, 8)).toBeUndefined()
      expect(failedCard.getBoundingClientRect().height).toBeGreaterThanOrEqual(96)
      for (const [windowWidth, count] of [
        [600, 2],
        [700, 3],
        [900, 4],
        [1100, 5],
        [1400, 6],
        [1600, 7],
      ]) {
        await rerender({ ...propsFor(assets), isExpanded: true, windowWidth })
        await expect.poll(() => assertRoundRobinLayout(root, count!, 12)).toBeUndefined()
        expect(cards(root)[0]).toBe(failedCard)
      }
      root.style.width = '420px'
      await expect.poll(() => assertRoundRobinLayout(root, 7, 12)).toBeUndefined()
      expect(failedCard.getBoundingClientRect().height).toBeGreaterThanOrEqual(96)
      expect(within(failedCard).getByText('Image unavailable')).toBeVisible()
      // A narrow viewport still uses measured fallback geometry; subsequent
      // cards must remain below the wrapped fallback without overlap.
      root.style.width = '180px'
      await expect.poll(() => assertRoundRobinLayout(root, 7, 12)).toBeUndefined()
      expect(failedCard.getBoundingClientRect().height).toBeGreaterThanOrEqual(96)
    } finally {
      unmount()
      root.remove()
    }
  })
})
