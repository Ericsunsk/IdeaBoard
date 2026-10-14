import { describe, expect, it } from 'vitest'
import { fireEvent } from '@testing-library/vue'
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
