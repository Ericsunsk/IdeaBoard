import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const searchRoot = resolve(process.cwd(), 'src/features/search')
const readSource = (relativePath: string) => readFileSync(resolve(searchRoot, relativePath), 'utf8')

const searchPage = readSource('pages/SearchPage.vue')
const searchStyles = readSource('search.css')
const searchSavePopover = readSource('components/SearchSavePopover.vue')
const searchBatchActionDock = readSource('components/SearchBatchActionDock.vue')

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

  it('uses the shared overlay surface while retaining Search popover treatment', () => {
    expect(searchSavePopover).toContain(
      "import { appOverlaySurfaceClass } from '../../../shared/theme/appOverlayMenu'",
    )
    expect(searchSavePopover).toContain('`${appOverlaySurfaceClass}')
    expect(searchSavePopover).toContain('!rounded-[var(--radius-ib-control)]')
    expect(searchSavePopover).toContain('!shadow-[var(--shadow-ib-overlay-menu)]')
    expect(searchSavePopover).toContain('!backdrop-blur-[var(--ib-glass-blur)]')
  })
})
