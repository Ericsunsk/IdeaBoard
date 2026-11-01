import { mediaDisplayFixture } from '../../../../test/mediaDisplay'
import { describe, expect, it } from 'vitest'
import { fireEvent } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { render, screen } from '../../../../test/render'
import SearchResultCard from './SearchResultCard.vue'
import type { SearchResult } from '../../core/models'

const result: SearchResult = {
  id: 'photo-1',
  provider: 'unsplash',
  title: 'Warm light',
  author: 'Author',
  author_url: 'https://example.com/author',
  source_url: 'https://example.com/source',
  thumbnail_url: 'https://example.com/thumb.jpg',
  preview_url: 'https://example.com/preview.jpg',
  original_url: 'https://example.com/original.jpg',
  width: 1200,
  height: 800,
  media_kind: 'image',
  display: mediaDisplayFixture(),
}

function renderCard() {
  return render(SearchResultCard, {
    props: {
      result,
      index: 0,
      isSelected: false,
      isUsedAsReference: false,
      isSaved: false,
      hasImageError: true,
      imageRetryCount: 0,
    },
  })
}

describe('Search result drag source', () => {
  it('selects bounded display tiers from measured layout and preserves provider identity', async () => {
    const view = renderCard()
    const image = screen.getByRole('img', { name: 'Warm light' })
    await view.rerender({ displayWidth: 120 })
    expect(image).toHaveAttribute('src', '/api/media/previews/fixture/thumb')
    await view.rerender({ displayWidth: 320 })
    expect(image).toHaveAttribute('src', '/api/media/previews/fixture/preview')
    await view.rerender({ displayWidth: 2000 })
    expect(image).toHaveAttribute('src', '/api/media/previews/fixture/large-preview')
    expect(image).not.toHaveAttribute('srcset')
    expect(image).toHaveAttribute('loading', 'eager')
    expect(result.original_url).toBe('https://example.com/original.jpg')
  })

  it('shows unavailable display without requesting the provider original', async () => {
    const view = renderCard()
    await view.rerender({ result: { ...result, display: undefined }, hasImageError: false })
    expect(screen.queryByRole('img')).toBeNull()
    expect(screen.getByText('The image could not be loaded. Try again.')).toBeInTheDocument()
  })

  it('prepares pointer dragging from the image and preserves ordinary preview clicks', async () => {
    const { emitted } = renderCard()
    const imageButton = screen.getByRole('img', { name: 'Warm light' }).closest('button')!
    await fireEvent.pointerDown(imageButton, { button: 0, isPrimary: true })
    expect(emitted().cardPointerDown).toHaveLength(1)
    await fireEvent.click(imageButton)
    expect(emitted().openLightbox).toHaveLength(1)
    const nativeDrag = new Event('dragstart', { bubbles: true, cancelable: true })
    expect(imageButton.dispatchEvent(nativeDrag)).toBe(false)
  })

  it('keeps selection, reference, save and retry buttons outside the drag source', async () => {
    const { emitted } = renderCard()
    const imageButton = screen.getByRole('img', { name: 'Warm light' }).closest('button')!
    for (const button of screen.getAllByRole('button')) {
      if (button === imageButton) continue
      await fireEvent.pointerDown(button, { button: 0, isPrimary: true })
      await fireEvent.click(button)
    }
    expect(emitted().cardPointerDown).toBeUndefined()
    expect(emitted().openLightbox).toBeUndefined()
    expect(emitted().toggleSelect).toHaveLength(1)
    expect(emitted().useAsReference).toHaveLength(1)
    expect(emitted().toggleSave).toHaveLength(1)
    expect(emitted().retryImage).toHaveLength(1)
  })
})

// Ported from searchComponents.unit.spec.ts (deleted): presentation cases for
// the same SFC, kept verbatim alongside the drag-source cases above.
const presentationItem: SearchResult = {
  id: 'img-1',
  provider: 'unsplash',
  title: 'Mountain Sunset',
  author: 'John Doe',
  author_url: 'https://example.com/author',
  media_kind: 'image',
  display: mediaDisplayFixture(),
  source_url: 'https://example.com/source',
  thumbnail_url: 'https://example.com/thumb.jpg',
  preview_url: 'https://example.com/preview.jpg',
  original_url: 'https://example.com/orig.jpg',
  width: 800,
  height: 600,
}

describe('SearchResultCard presentation', () => {
  it('renders preview image with aspect ratio and handles lightbox click', async () => {
    const user = userEvent.setup()
    let wasLightboxOpened = false
    render(SearchResultCard, {
      props: {
        result: presentationItem,
        index: 0,
        isSelected: false,
        isUsedAsReference: false,
        isSaved: false,
        hasImageError: false,
        imageRetryCount: 0,
        onOpenLightbox: () => {
          wasLightboxOpened = true
        },
      },
    })
    const img = screen.getByAltText('Mountain Sunset')
    expect(img).toHaveAttribute('src', '/api/media/previews/fixture/large-preview')
    const zoomBtn = screen.getByRole('button', { name: /view|查看/i })
    await user.click(zoomBtn)
    expect(wasLightboxOpened).toBe(true)
  })

  it('renders retry button when hasImageError is true', () => {
    render(SearchResultCard, {
      props: {
        result: presentationItem,
        index: 0,
        isSelected: false,
        isUsedAsReference: false,
        isSaved: false,
        hasImageError: true,
        imageRetryCount: 1,
      },
    })
    expect(screen.getByRole('button', { name: /重试|Retry/i })).toBeInTheDocument()
  })

  it('renders selected, saved, and reference states correctly', () => {
    const { rerender } = render(SearchResultCard, {
      props: {
        result: presentationItem,
        index: 0,
        isSelected: true,
        isUsedAsReference: true,
        isSaved: true,
        hasImageError: false,
        imageRetryCount: 0,
      },
    })

    const card = screen.getByRole('article')
    expect(card).toBeInTheDocument()

    rerender({
      result: presentationItem,
      index: 0,
      isSelected: false,
      isUsedAsReference: false,
      isSaved: false,
      hasImageError: false,
      imageRetryCount: 0,
    })
    expect(card).toBeInTheDocument()
  })
})
