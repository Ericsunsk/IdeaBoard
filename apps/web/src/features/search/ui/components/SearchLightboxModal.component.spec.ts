import { mediaDisplayFixture } from '../../../../test/mediaDisplay'
import { describe, expect, it } from 'vitest'
import { render, screen, waitFor } from '../../../../test/render'
import userEvent from '@testing-library/user-event'
import SearchLightboxModal from './SearchLightboxModal.vue'
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
