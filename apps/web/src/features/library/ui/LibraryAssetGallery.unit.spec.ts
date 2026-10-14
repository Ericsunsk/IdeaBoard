import { describe, expect, it } from 'vitest'
import { fireEvent } from '@testing-library/vue'
import { render, screen, within } from '../../../test/render'
import LibraryAssetGallery from './LibraryAssetGallery.vue'
import { libraryAssetKey } from '../core/validators'
import type { LibraryAssetView } from '../core/transfer'

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

const baseProps = (assets: LibraryAssetView[]) => ({
  assets,
  isLoading: false,
  errorMessage: null as string | null,
  hasMore: false,
  isExpanded: false,
  isCanvasPresentation: false,
  windowWidth: 1200,
})

describe('LibraryAssetGallery', () => {
  it('starts pointer dragging only from the preview and prevents native dragging', async () => {
    const asset = galleryAsset(0)
    const view = render(LibraryAssetGallery, { props: baseProps([asset]) })
    const preview = screen.getByRole('button', { name: 'View Reference 0' })
    await fireEvent.pointerDown(preview)
    expect(view.emitted()['asset-pointer-down']).toHaveLength(1)
    await fireEvent.pointerDown(screen.getByRole('button', { name: 'Use as search reference' }))
    expect(view.emitted()['asset-pointer-down']).toHaveLength(1)
    const native = new Event('dragstart', { bubbles: true, cancelable: true })
    expect(preview.dispatchEvent(native)).toBe(false)
    await fireEvent.click(preview)
    expect(view.emitted()['open-asset']).toHaveLength(1)
  })

  it('renders identical output across viewport-dependent column counts', async () => {
    const assets = [0, 1, 2, 3, 4].map(galleryAsset)
    const { rerender } = render(LibraryAssetGallery, { props: baseProps(assets) })

    expect(screen.getAllByRole('article')).toHaveLength(5)
    for (const image of screen.getAllByRole('img')) {
      expect(image).toHaveAttribute('loading', 'lazy')
    }
    for (const asset of assets) {
      expect(screen.getByText(asset.title as string)).toBeInTheDocument()
    }

    // Expanded at 1400px -> 6 lanes: buckets redistribute, cards must not
    // duplicate, drop, or swap titles.
    await rerender({ ...baseProps(assets), isExpanded: true, windowWidth: 1400 })

    expect(screen.getAllByRole('article')).toHaveLength(5)
    for (const asset of assets) {
      expect(screen.getByText(asset.title as string)).toBeInTheDocument()
    }
  })

  it('preserves stateful asset identity and selected/focused asset across column redistribution', async () => {
    // library/masonry-child-identity: Reference 2 carries image-error state;
    // Reference 3 changes lanes and must retain DOM-local state and focus.
    const assets = [0, 1, 2, 3, 4].map(galleryAsset)
    const selectedKey = libraryAssetKey('unsplash', 'photo-3')
    const { rerender } = render(LibraryAssetGallery, {
      props: { ...baseProps(assets), referenceKeys: [selectedKey] },
    })

    const articleFor = (title: string): HTMLElement => {
      const article = screen.getByText(title).closest('article')
      if (!(article instanceof HTMLElement)) throw new Error(`missing article for ${title}`)
      return article
    }

    // Selection marker identity: the select-reference button renders a
    // distinct marker per selection state; capture both variants. This stays
    // icon-implementation-agnostic (no SVG path hardcoding).
    const markerFor = (title: string): string => {
      const button = within(articleFor(title)).getByRole('button', {
        name: 'Use as search reference',
      })
      return button.querySelector('svg')?.innerHTML ?? 'missing-marker'
    }

    // Stateful child: fail Reference 2's image (per-asset fallback keyed by id).
    const failedImage = within(articleFor('Reference 2')).getByRole('img', {
      name: 'Reference 2',
    })
    await fireEvent.error(failedImage)
    expect(articleFor('Reference 2').textContent).toContain('Image unavailable')

    // Selected asset: Reference 3 renders the selected marker, Reference 0 does not.
    const selectedMarker = markerFor('Reference 3')
    const unselectedMarker = markerFor('Reference 0')
    expect(selectedMarker).not.toBe('missing-marker')
    expect(selectedMarker).not.toBe(unselectedMarker)

    // This asset actually changes lanes; browser-local state must survive too.
    const movingArticle = articleFor('Reference 3')
    const viewer3 = within(movingArticle).getByRole('button', { name: 'View Reference 3' })
    const localState = { draft: 'asset-local browser state' }
    Object.assign(viewer3, { localState })
    viewer3.focus()
    expect(document.activeElement).toBe(viewer3)

    // Expanded at 1400px -> 6 lanes: buckets redistribute.
    await rerender({
      ...baseProps(assets),
      referenceKeys: [selectedKey],
      isExpanded: true,
      windowWidth: 1400,
    })

    expect(screen.getAllByRole('article')).toHaveLength(5)
    // Image-error state follows Reference 2 into its new lane (and stays off Reference 0).
    expect(articleFor('Reference 2').textContent).toContain('Image unavailable')
    expect(within(articleFor('Reference 2')).queryByRole('img')).toBeNull()
    expect(within(articleFor('Reference 0')).queryByText('Image unavailable')).toBeNull()
    // Selection follows Reference 3 into its new lane.
    expect(markerFor('Reference 3')).toBe(selectedMarker)
    expect(markerFor('Reference 0')).toBe(unselectedMarker)
    expect(articleFor('Reference 3')).toBe(movingArticle)
    expect(within(movingArticle).getByRole('button', { name: 'View Reference 3' })).toBe(viewer3)
    expect(Object.getOwnPropertyDescriptor(viewer3, 'localState')?.value).toBe(localState)
    expect(document.activeElement).toBe(viewer3)
  })

  it('keeps each existing card DOM identity through reorder and insertion', async () => {
    const assets = [0, 1, 2, 3, 4].map(galleryAsset)
    const { rerender } = render(LibraryAssetGallery, { props: baseProps(assets) })
    const cards = new Map(
      assets.map((asset) => [asset.id, screen.getByText(asset.title!).closest('article')]),
    )
    const viewer = screen.getByRole('button', { name: 'View Reference 3' })
    const localState = { draft: 'Reference 3' }
    Object.assign(viewer, { localState })
    await rerender(baseProps([galleryAsset(9), ...[...assets].reverse()]))
    for (const asset of assets) {
      expect(screen.getByText(asset.title!).closest('article')).toBe(cards.get(asset.id))
    }
    const preservedViewer = screen.getByRole('button', { name: 'View Reference 3' })
    expect(preservedViewer).toBe(viewer)
    expect(Object.getOwnPropertyDescriptor(preservedViewer, 'localState')?.value).toBe(localState)
  })
})
