/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(process.cwd(), 'src')
const readSource = (relativePath: string) => readFileSync(resolve(sourceRoot, relativePath), 'utf8')

const consumers = {
  navigation: readSource('features/workspace/shell/components/WorkspaceNavigation.vue'),
  searchPage: readSource('features/search/presentation/pages/SearchPage.vue'),
  searchBatchDock: readSource('features/search/presentation/components/SearchBatchActionDock.vue'),
  boardsSkin: readSource('features/board/presentation/catalog/skin.ts'),
  libraryIsland: readSource('features/library/components/LibraryFloatingIsland.vue'),
  canvasChrome: readSource('features/canvas/presentation/chrome/components/CanvasChrome.vue'),
  boardHeader: readSource(
    'features/canvas/presentation/chrome/components/CanvasChromeBoardHeader.vue',
  ),
  canvasMiniMap: readSource('features/canvas/presentation/chrome/CanvasMiniMap.vue'),
  userAccountMenu: readSource('shared/components/UserAccountMenu.vue'),
  creditBalance: readSource('shared/components/UserCreditBalanceButton.vue'),
}

describe('Cross-feature page chrome surface contract', () => {
  it('keeps every page chrome consumer on the shared surface class', () => {
    Object.values(consumers).forEach((source) => {
      expect(source).toContain('page-chrome-surface')
    })
  })

  it('never binds a controller viewModel field with v-model', () => {
    // BoardsPage destructures `controller.viewModel`, and every field in it is
    // readonly(readonly(ref)). `v-model` on such a field compiles and renders
    // but silently drops the write, so the page looks correct until someone
    // changes a setting and nothing happens. Selection must go through commands.
    const boardsPage = readSource('features/board/presentation/catalog/BoardsPage.vue')
    const vModelBindings = [...boardsPage.matchAll(/v-model(?:\.[\w-]+)?="([^"]+)"/g)].map(
      (match) => match[1],
    )

    expect(vModelBindings).toEqual([])
  })

  it('uses the shared Select primitive for Boards sorting, never a command menu', () => {
    const boardsPage = readSource('features/board/presentation/catalog/BoardsPage.vue')

    // Sorting picks a value and holds it, so it is a Select, not a menu of
    // commands. It used to be a DropdownMenu with `aria-haspopup="menu"`, which
    // mis-declared the widget and gave up the keyboard model. This assertion
    // originally existed to keep PrimeVue's Select out; PrimeVue is gone, and
    // `@/components/ui/select` is the neutral replacement.
    expect(boardsPage).toContain(':class="catalogClasses.sortSelectTrigger"')
    expect(boardsPage).toContain("from '@/components/ui/select'")
    expect(boardsPage).not.toContain('<DropdownMenu')
    expect(boardsPage).not.toContain("from 'primevue/select'")
  })

  it('keeps fully-skinned chrome controls on native buttons so no Button base leaks', () => {
    // pill、头像/语言触发器、游客按钮自带完整视觉：必须用原生 button，
    // shadcn Button 的 variant 基类会与 @layer components 的 chrome 样式打架（utilities 层优先）。
    for (const source of [
      readSource('shared/components/UserCreditBalanceButton.vue'),
      readSource('shared/components/UserAccountMenu.vue'),
    ]) {
      expect(source).not.toContain('<UiButton')
      expect(source).not.toContain("from '@/components/ui/button'")
    }
  })

  it('does not re-declare the shared surface inside feature styles', () => {
    const featureStyles = [
      readSource('features/search/presentation/search.css'),
      readSource('features/canvas/presentation/styles/canvas-chrome.css'),
      readSource('features/canvas/presentation/styles/canvas-vue-flow-adapter-skin.css'),
    ]

    featureStyles.forEach((source) => {
      expect(source).not.toContain('box-shadow: var(--shadow-page-chrome)')
      expect(source).not.toContain('border: 1px solid var(--color-page-chrome-border)')
    })
  })
})
