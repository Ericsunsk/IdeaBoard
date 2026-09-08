import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const searchRoot = resolve(process.cwd(), 'src/features/search')
const readSource = (relativePath: string) => readFileSync(resolve(searchRoot, relativePath), 'utf8')

const searchPage = readSource('presentation/pages/SearchPage.vue')
const searchStyles = readSource('presentation/search.css')
const searchSavePopover = readSource('presentation/components/SearchSavePopover.vue')
const searchBatchActionDock = readSource('presentation/components/SearchBatchActionDock.vue')

describe('Search page chrome contract', () => {
  it('uses the shared page chrome surface without a Search-only background layer', () => {
    expect(searchPage).toContain('ib-page-chrome-surface')
    expect(searchBatchActionDock).toContain('ib-page-chrome-surface')
    expect(searchStyles).not.toContain('var(--color-ib-page-chrome-border)')
    expect(searchStyles).not.toContain('var(--color-ib-page-chrome-bg)')
    expect(searchStyles).not.toContain('var(--shadow-ib-page-chrome)')
    expect(searchStyles).not.toContain('--search-dock-shadow')
    expect(searchStyles).not.toContain('--search-glass-bg')
    expect(searchStyles).not.toContain('--search-glass-blur')
    expect(searchPage).toContain('hover:border-[var(--color-ib-page-chrome-border-hover)]')
  })

  it('uses the shared page chrome surface while retaining Search popover treatment', () => {
    expect(searchSavePopover).toContain('<Popover')
    expect(searchSavePopover).toContain('unstyled')
    expect(searchSavePopover).toContain('ib-page-chrome-surface')
    expect(searchSavePopover).toContain('search-save-popover')
    expect(searchSavePopover).toContain('rounded-[var(--radius-ib-control)]')
    expect(searchSavePopover).not.toContain("content: { class: '!p-0' }")
    expect(searchSavePopover).not.toContain('before:!hidden after:!hidden')
  })

  it('keeps the overlay gutter in CSS so PrimeVue can re-align safely', () => {
    expect(searchStyles).toContain('.search-save-popover')
    expect(searchStyles).toContain('margin-block-start: var(--spacing-ib-overlay-gutter)')
    expect(searchStyles).toContain("[data-p-popover-flipped='true']")
    expect(searchSavePopover).not.toContain('positionPopoverAbove')
  })
})
