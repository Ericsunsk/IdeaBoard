/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(process.cwd(), 'src')
const readSource = (relativePath: string) => readFileSync(resolve(sourceRoot, relativePath), 'utf8')

describe('Cross-feature page chrome surface contract', () => {
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
})
