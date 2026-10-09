import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  attachSearchDragImage,
  createSearchDragGhost,
  resolveDragAnchor,
  SEARCH_LIBRARY_AUTO_OPEN_DELAY_MS,
} from './dragGhost'
import type { SearchResult } from '../../domain/models'

function searchResult(overrides: Partial<SearchResult> = {}): SearchResult {
  return {
    id: 'photo-1',
    provider: 'unsplash',
    title: 'Warm light',
    author: 'IdeaBoard',
    author_url: 'https://example.com/author',
    source_url: 'https://example.com/photo-1',
    thumbnail_url: 'https://images.example/photo-1-thumb.jpg',
    preview_url: 'https://images.example/photo-1-preview.jpg',
    original_url: 'https://images.example/photo-1-original.jpg',
    width: 1200,
    height: 800,
    media_kind: 'image',
    ...overrides,
  }
}

function fakeDataTransfer() {
  return { setDragImage: vi.fn() } as unknown as DataTransfer & {
    setDragImage: ReturnType<typeof vi.fn>
  }
}

describe('search drag ghost browser adapter', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.useRealTimers()
  })

  it('keeps the 280ms Library auto-open delay', () => {
    expect(SEARCH_LIBRARY_AUTO_OPEN_DELAY_MS).toBe(280)
  })

  it('resolves the save anchor from the event currentTarget', () => {
    const anchor = document.createElement('button')
    expect(resolveDragAnchor({ currentTarget: anchor } as unknown as Event)).toBe(anchor)
    expect(resolveDragAnchor({ currentTarget: null } as unknown as Event)).toBeNull()
  })

  it('builds the ghost with the stable presentation style', () => {
    const ghost = createSearchDragGhost(searchResult())
    try {
      expect(ghost.style.position).toBe('fixed')
      expect(ghost.style.width).toBe('58px')
      expect(ghost.style.height).toBe('44px')
      expect(ghost.style.borderRadius).toBe('var(--primitive-radius-lg)')
      expect(ghost.style.overflow).toBe('hidden')
      expect(ghost.style.zIndex).toBe('9999')
      const image = ghost.querySelector('img')
      expect(image?.getAttribute('src')).toBe('https://images.example/photo-1-thumb.jpg')
      expect(image?.style.objectFit).toBe('cover')
    } finally {
      ghost.remove()
    }
  })

  it('falls back to the preview URL when no thumbnail exists', () => {
    const ghost = createSearchDragGhost(searchResult({ thumbnail_url: '' }))
    try {
      expect(ghost.querySelector('img')?.getAttribute('src')).toBe(
        'https://images.example/photo-1-preview.jpg',
      )
    } finally {
      ghost.remove()
    }
  })

  it('attaches the drag image at the stable offset and removes the ghost', () => {
    vi.useFakeTimers()
    const transfer = fakeDataTransfer()
    attachSearchDragImage(transfer, searchResult())

    expect(transfer.setDragImage).toHaveBeenCalledOnce()
    const [ghost, offsetX, offsetY] = transfer.setDragImage.mock.calls[0] as [
      HTMLElement,
      number,
      number,
    ]
    expect(offsetX).toBe(29)
    expect(offsetY).toBe(22)
    expect(ghost.isConnected).toBe(true)

    vi.runAllTimers()
    expect(ghost.isConnected).toBe(false)
  })

  it('keeps ghost instances independent without global DOM state', () => {
    vi.useFakeTimers()
    const first = fakeDataTransfer()
    const second = fakeDataTransfer()
    attachSearchDragImage(first, searchResult({ id: 'first' }))
    attachSearchDragImage(second, searchResult({ id: 'second' }))

    const firstGhost = first.setDragImage.mock.calls[0]?.[0] as HTMLElement
    const secondGhost = second.setDragImage.mock.calls[0]?.[0] as HTMLElement
    expect(firstGhost).not.toBe(secondGhost)

    vi.runAllTimers()
    expect(firstGhost.isConnected).toBe(false)
    expect(secondGhost.isConnected).toBe(false)
  })
})
