/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(process.cwd(), 'src')
const readSource = (relativePath: string) => readFileSync(resolve(sourceRoot, relativePath), 'utf8')

const consumers = {
  navigation: readSource('features/user-shell/components/UserWorkspaceNavigation.vue'),
  searchPage: readSource('features/search/presentation/pages/SearchPage.vue'),
  searchBatchDock: readSource('features/search/presentation/components/SearchBatchActionDock.vue'),
  boardsSkin: readSource('features/board-catalog/lib/skin.ts'),
  libraryIsland: readSource('features/library/components/LibraryFloatingIsland.vue'),
  canvasChrome: readSource('features/canvas/workspace-chrome/components/CanvasChrome.vue'),
  canvasBoardHeader: readSource(
    'features/canvas/workspace-chrome/components/CanvasChromeBoardHeader.vue',
  ),
  canvasMiniMap: readSource('features/canvas/components/CanvasMiniMap.vue'),
  userAccountMenu: readSource('shared/components/UserAccountMenu.vue'),
  creditBalance: readSource('shared/components/UserCreditBalanceButton.vue'),
}

describe('Cross-feature page chrome surface contract', () => {
  it('keeps every page chrome consumer on the shared surface class', () => {
    Object.values(consumers).forEach((source) => {
      expect(source).toContain('ib-page-chrome-surface')
    })
  })

  it('uses a command menu instead of a form select for Boards sorting', () => {
    const boardsPage = readSource('features/board-catalog/pages/BoardsPage.vue')

    expect(boardsPage).toContain(':class="catalogClasses.sortMenuTrigger"')
    expect(boardsPage).toContain('<Menu')
    expect(boardsPage).not.toContain("from 'primevue/select'")
  })

  it('does not re-declare the shared surface inside feature styles', () => {
    const featureStyles = [
      readSource('features/search/presentation/search.css'),
      readSource('features/canvas/components/styles/canvas-chrome.css'),
      readSource('features/canvas/components/styles/canvas-vue-flow-adapter-skin.css'),
    ]

    featureStyles.forEach((source) => {
      expect(source).not.toContain('box-shadow: var(--shadow-ib-page-chrome)')
      expect(source).not.toContain('border: 1px solid var(--color-ib-page-chrome-border)')
    })
  })
})
