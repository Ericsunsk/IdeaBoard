import { h } from 'vue'
import { render } from '@testing-library/vue'
import { page } from 'vitest/browser'
import { describe, expect, it } from 'vitest'
import SearchResultViewport from './SearchResultViewport.vue'
import type { SearchResult } from '../searchResource'

function results(count: number): SearchResult[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `result-${index}`,
    provider: 'unsplash',
    title: `Result ${index}`,
    author: 'IdeaBoard',
    author_url: 'https://example.com/author',
    source_url: `https://example.com/result-${index}`,
    thumbnail_url: `https://images.example.com/thumb-${index}.jpg`,
    preview_url: `https://images.example.com/preview-${index}.jpg`,
    original_url: `https://images.example.com/original-${index}.jpg`,
    width: index % 3 === 0 ? 1200 : 900,
    height: index % 3 === 0 ? 900 : 1200,
    media_kind: 'image',
  }))
}

describe('SearchResultViewport browser contract', () => {
  it('keeps DOM bounded after loading and scrolling through 120 results', async () => {
    await page.viewport(1000, 700)
    const root = document.createElement('div')
    root.style.cssText = 'width: 1000px; height: 480px; overflow-y: auto; position: relative;'
    document.body.append(root)

    try {
      const rendered = render(SearchResultViewport, {
        container: root,
        props: { items: results(120), scrollRoot: root },
        slots: {
          default: ({ item, style }: { item: SearchResult; style: Record<string, string> }) =>
            h('article', { 'data-testid': `result-${item.id}`, style }),
        },
      })

      await expect.poll(() => rendered.getAllByRole('article').length).toBeLessThan(120)
      expect(rendered.getAllByRole('article').length).toBeGreaterThan(0)

      const firstCount = rendered.getAllByRole('article').length
      root.scrollTop = 2400
      root.dispatchEvent(new Event('scroll'))
      await new Promise((resolve) => setTimeout(resolve, 80))

      const scrolledCount = rendered.getAllByRole('article').length
      expect(scrolledCount).toBeLessThan(120)
      expect(Math.abs(scrolledCount - firstCount)).toBeLessThan(60)
    } finally {
      root.remove()
    }
  })

  it('notifies the screen runtime when its active Popover anchor unmounts', () => {
    const root = document.createElement('div')
    const anchor = document.createElement('button')
    let hiddenCount = 0
    root.append(anchor)
    document.body.append(root)

    try {
      const rendered = render(SearchResultViewport, {
        container: root,
        props: {
          items: results(1),
          activeAnchor: anchor,
          'onActive-anchor-hidden': () => {
            hiddenCount += 1
          },
        },
        slots: {
          default: ({ item, style }: { item: SearchResult; style: Record<string, string> }) =>
            h('article', { style, 'data-testid': item.id }),
        },
      })

      rendered.unmount()
      expect(hiddenCount).toBe(1)
    } finally {
      root.remove()
    }
  })

  it('reports scroll observations without owning feed state', async () => {
    const root = document.createElement('div')
    root.style.cssText = 'width: 1000px; height: 480px; overflow-y: auto; position: relative;'
    document.body.append(root)
    const observed: number[] = []

    try {
      const rendered = render(SearchResultViewport, {
        container: root,
        props: {
          items: results(120),
          scrollRoot: root,
          'onScroll-position': (scrollTop: number) => observed.push(scrollTop),
        },
        slots: {
          default: ({ item, style }: { item: SearchResult; style: Record<string, string> }) =>
            h('article', { style, 'data-testid': item.id }),
        },
      })
      Object.defineProperty(root, 'scrollTop', { configurable: true, value: 240 })
      root.dispatchEvent(new Event('scroll'))
      await new Promise((resolve) => setTimeout(resolve, 80))

      expect(observed).toContain(240)
      rendered.unmount()
    } finally {
      root.remove()
    }
  })
})
