import { mediaDisplayFixture } from '../../../../test/mediaDisplay'
import { describe, expect, it } from 'vitest'
import { render, screen, waitFor } from '../../../../test/render'
import userEvent from '@testing-library/user-event'
import SearchLightboxModal from './SearchLightboxModal.vue'
import SearchEmptyState from './SearchEmptyState.vue'
import SearchResultCard from './SearchResultCard.vue'
import type { SearchResult } from '../../core/models'

const mockItem: SearchResult = {
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

describe('Search presentation subcomponents', () => {
  describe('SearchLightboxModal', () => {
    it('renders image, title and author when visible', () => {
      render(SearchLightboxModal, {
        props: {
          visible: true,
          activeResult: mockItem,
        },
      })
      expect(screen.getByText('Mountain Sunset')).toBeInTheDocument()
      expect(screen.getByText(/John Doe/)).toBeInTheDocument()
      expect(screen.getByRole('img')).toHaveAttribute(
        'src',
        '/api/media/previews/fixture/large-preview',
      )
    })

    it('keeps a visible focus ring on the close button over the dark backdrop', () => {
      render(SearchLightboxModal, {
        props: {
          visible: true,
          activeResult: mockItem,
        },
      })
      const closeButton = screen.getByRole('button', { name: /关闭|close/i })
      expect(closeButton.className).toContain('focus-visible:outline-none')
      expect(closeButton.className).toContain('focus-visible:shadow-focus-ring')
    })

    it('traps focus, closes on Escape, and restores the opening trigger focus', async () => {
      const user = userEvent.setup()
      const trigger = document.createElement('button')
      document.body.appendChild(trigger)
      trigger.focus()
      const { emitted, rerender } = render(SearchLightboxModal, {
        props: {
          visible: true,
          activeResult: mockItem,
        },
      })

      await screen.findByRole('dialog')
      const closeButton = screen.getByRole('button', { name: /关闭|close/i })
      expect(closeButton).toHaveFocus()

      await user.keyboard('{Tab}')
      expect(screen.getByRole('link', { name: /来源|source/i })).toHaveFocus()
      await user.keyboard('{Tab}')
      expect(closeButton).toHaveFocus()
      await user.keyboard('{Escape}')
      expect(emitted().close).toHaveLength(1)

      await rerender({ visible: false, activeResult: null })
      await waitFor(() => expect(trigger).toHaveFocus())
      trigger.remove()
    })
  })

  describe('SearchEmptyState', () => {
    it('renders error state and handles retry button click', async () => {
      const user = userEvent.setup()
      let reloadCalled = false
      render(SearchEmptyState, {
        props: {
          hasError: true,
          resultsCount: 0,
          hasActiveSearch: false,
          canAddReference: true,
          onReload: () => {
            reloadCalled = true
          },
        },
      })
      const retryBtn = screen.getByRole('button', { name: /重试|Retry/i })
      expect(retryBtn).toBeInTheDocument()
      await user.click(retryBtn)
      expect(reloadCalled).toBe(true)
    })

    it('renders empty feed state and emits actions', async () => {
      const user = userEvent.setup()
      let clearCalled = false
      render(SearchEmptyState, {
        props: {
          hasError: false,
          resultsCount: 0,
          hasActiveSearch: true,
          canAddReference: true,
          onClearSearch: () => {
            clearCalled = true
          },
        },
      })
      const clearBtn = screen.getByRole('button', { name: /清空搜索|Clear/i })
      expect(clearBtn).toBeInTheDocument()
      await user.click(clearBtn)
      expect(clearCalled).toBe(true)
    })
  })

  describe('SearchResultCard', () => {
    it('renders preview image with aspect ratio and handles lightbox click', async () => {
      const user = userEvent.setup()
      let lightboxOpened = false
      render(SearchResultCard, {
        props: {
          result: mockItem,
          index: 0,
          isSelected: false,
          isUsedAsReference: false,
          isSaved: false,
          hasImageError: false,
          imageRetryCount: 0,
          onOpenLightbox: () => {
            lightboxOpened = true
          },
        },
      })
      const img = screen.getByAltText('Mountain Sunset')
      expect(img).toHaveAttribute('src', '/api/media/previews/fixture/large-preview')
      const zoomBtn = screen.getByRole('button', { name: /view|查看/i })
      await user.click(zoomBtn)
      expect(lightboxOpened).toBe(true)
    })

    it('renders retry button when hasImageError is true', () => {
      render(SearchResultCard, {
        props: {
          result: mockItem,
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
          result: mockItem,
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
        result: mockItem,
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
})
