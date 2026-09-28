import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { render, screen } from '../../../test/render'
import LibraryAssetGallery from './LibraryAssetGallery.vue'
import type { LibraryAssetView } from '../domain/transfer'

function galleryAsset(index: number): LibraryAssetView {
  return {
    id: `lasset_asset${String(index).padStart(4, '0')}`,
    workspaceId: 'ws_test0001',
    folderId: 'lfolder_folder0001',
    provider: 'unsplash',
    sourceId: `photo-${index}`,
    sourceUrl: `https://example.com/source-${index}`,
    previewUrl: `https://images.unsplash.com/photo-${index}`,
    originalUrl: `https://images.unsplash.com/photo-${index}`,
    title: `Reference ${index}`,
    author: `Author ${index}`,
    width: 800,
    height: 600,
    createdAt: '2026-09-24T00:00:00Z',
    updatedAt: '2026-09-24T00:00:00Z',
  }
}

function masonryColumnCount(container: Element): number {
  return container.querySelector('.items-start')?.children.length ?? 0
}

const baseProps = (assets: LibraryAssetView[]) => ({
  assets,
  loading: false,
  errorMessage: null as string | null,
  hasMore: false,
  isExpanded: false,
  isCanvasPresentation: false,
  windowWidth: 1200,
})

describe('LibraryAssetGallery', () => {
  it('renders identical output across viewport-dependent column counts', async () => {
    const assets = [0, 1, 2, 3, 4].map(galleryAsset)
    const { container, rerender } = render(LibraryAssetGallery, { props: baseProps(assets) })

    expect(masonryColumnCount(container)).toBe(2)
    expect(screen.getAllByRole('article')).toHaveLength(5)
    for (const asset of assets) {
      expect(screen.getByText(asset.title as string)).toBeInTheDocument()
    }

    // Expanded at 1400px -> 6 lanes: buckets redistribute, cards must not
    // duplicate, drop, or swap titles.
    await rerender({ ...baseProps(assets), isExpanded: true, windowWidth: 1400 })

    expect(masonryColumnCount(container)).toBe(6)
    expect(screen.getAllByRole('article')).toHaveLength(5)
    for (const asset of assets) {
      expect(screen.getByText(asset.title as string)).toBeInTheDocument()
    }
  })

  it('keys masonry columns by stable asset id instead of viewport-dependent index', () => {
    // Column keys never reach the DOM (articles are already keyed by
    // asset.id and columns hold no local state), so the template contract
    // itself is the regression surface: an index key would reuse the wrong
    // column subtree when the lane count changes with the viewport.
    const source = readFileSync(resolve(import.meta.dirname, './LibraryAssetGallery.vue'), 'utf8')
    expect(source).toContain('column[0]?.id ?? colIndex')
    expect(source).not.toContain(':key="colIndex"')
  })
})
