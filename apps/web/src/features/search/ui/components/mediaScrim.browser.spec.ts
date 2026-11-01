import { afterEach, describe, expect, it } from 'vitest'
import { cleanup } from '@testing-library/vue'
import { h } from 'vue'
import { renderWithUser, screen } from '../../../../test/render'
import { mediaDisplayFixture } from '../../../../test/mediaDisplay'
import type { SearchResult } from '../../core/models'
import SearchResultCard from './SearchResultCard.vue'
import SearchLightboxModal from './SearchLightboxModal.vue'
import '../../../../app/style.css'

function paint(color: string): number[] {
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = 1
  const context = canvas.getContext('2d')!
  context.fillStyle = color
  context.fillRect(0, 0, 1, 1)
  return Array.from(context.getImageData(0, 0, 1, 1).data)
}

const result: SearchResult = {
  id: 'scrim-image',
  provider: 'unsplash',
  title: 'Bright image',
  author: 'Image author',
  author_url: '',
  media_kind: 'image',
  display: mediaDisplayFixture(),
  source_url: 'https://example.com/image',
  thumbnail_url: '',
  preview_url: '',
  original_url: '',
  width: 800,
  height: 600,
}

afterEach(() => {
  cleanup()
  document.documentElement.classList.remove('light')
})

describe.each(['dark', 'light'])('media scrim in %s mode', (mode) => {
  it('keeps light card text and the full dark gradient over a bright image', () => {
    document.documentElement.classList.toggle('light', mode === 'light')
    renderWithUser(SearchResultCard, {
      props: {
        result,
        index: 0,
        isSelected: true,
        isUsedAsReference: false,
        isSaved: false,
        hasImageError: false,
        imageRetryCount: 0,
      },
    })
    const title = screen.getByText('Bright image')
    expect(getComputedStyle(title).color).toBe('rgb(255, 255, 255)')
    const overlay = title.closest('.absolute')!
    expect(getComputedStyle(overlay).backgroundImage).toContain('0 0 0 / 0.8)')
    expect(getComputedStyle(overlay).backgroundImage).toContain('0 0 0 / 0.2)')
  })

  it('keeps the lightbox veil and actions readable independently of the app theme', () => {
    document.documentElement.classList.toggle('light', mode === 'light')
    renderWithUser(h(SearchLightboxModal, { isVisible: true, activeResult: result }))
    expect(getComputedStyle(screen.getByRole('heading')).color).toBe('rgb(255, 255, 255)')
    expect(paint(getComputedStyle(screen.getByRole('dialog')).backgroundColor)).toEqual([
      0, 0, 0, 166,
    ])
    const close = screen.getByRole('button', { name: /close/i })
    expect(paint(getComputedStyle(close).color)).toEqual([255, 255, 255, 230])
  })
})
