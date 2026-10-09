import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const searchRoot = resolve(process.cwd(), 'src/features/search')
const readSource = (relativePath: string) => readFileSync(resolve(searchRoot, relativePath), 'utf8')

const searchPage = readSource('presentation/pages/SearchPage.vue')
const searchStyles = readSource('presentation/search.css')
const searchSavePopover = readSource('presentation/components/SearchSavePopover.vue')
const searchBatchActionDock = readSource('presentation/components/SearchBatchActionDock.vue')
const searchResultCard = readSource('presentation/components/SearchResultCard.vue')

describe('Search page chrome contract', () => {
  it('uses the shared page chrome surface without a Search-only background layer', () => {
    expect(searchPage).toContain('page-chrome-surface')
    expect(searchBatchActionDock).toContain('page-chrome-surface')
    expect(searchStyles).not.toContain('var(--color-page-chrome-border)')
    expect(searchStyles).not.toContain('var(--color-page-chrome-bg)')
    expect(searchStyles).not.toContain('var(--shadow-page-chrome)')
    expect(searchStyles).not.toContain('--search-dock-shadow')
    expect(searchStyles).not.toContain('--search-glass-bg')
    expect(searchStyles).not.toContain('--search-glass-blur')
    expect(searchPage).toContain('hover:border-[var(--color-page-chrome-border-hover)]')
  })

  it('uses the shared page chrome surface while retaining Search popover treatment', () => {
    expect(searchSavePopover).toContain('<Popover')
    expect(searchSavePopover).toContain('page-chrome-surface')
    expect(searchSavePopover).toContain('search-save-popover')
    expect(searchSavePopover).toContain('rounded-control')
    expect(searchSavePopover).not.toContain('rounded-[var(--radius-control)]')
    expect(searchSavePopover).not.toContain("content: { class: '!p-0' }")
    expect(searchSavePopover).not.toContain('before:!hidden after:!hidden')
  })

  it('keeps the overlay gutter in CSS for directional alignment', () => {
    expect(searchStyles).toContain('.search-save-popover')
    expect(searchStyles).toContain('margin-block-start: var(--spacing-overlay-gutter)')
    expect(searchStyles).toContain("[data-side='top']")
    expect(searchSavePopover).not.toContain('positionPopoverAbove')
  })

  it('sources image overlay action fills from app overlay-btn tokens', () => {
    expect(searchStyles).toContain(
      '--search-overlay-btn-bg: var(--color-overlay-btn-bg, rgba(0, 0, 0, 0.42))',
    )
    expect(searchStyles).toContain(
      '--search-overlay-btn-bg-hover: var(--color-overlay-btn-bg-hover, rgba(0, 0, 0, 0.68))',
    )
    expect(searchResultCard).toContain('var(--search-overlay-btn-bg)')
    expect(searchResultCard).toContain('var(--search-overlay-btn-bg-hover)')
  })
})
