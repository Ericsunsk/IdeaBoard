import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, resolve } from 'node:path'
import { compile } from 'tailwindcss'
import { describe, expect, it } from 'vitest'

const stylesheet = readFileSync(resolve(process.cwd(), 'src/app/style.css'), 'utf8')
const require = createRequire(import.meta.url)

async function build(candidates: string[]): Promise<string> {
  const compiler = await compile(stylesheet, {
    base: resolve(process.cwd(), 'src/app'),
    loadStylesheet: async (id, base) => {
      const path = id.startsWith('.') ? resolve(base, id) : require.resolve(`${id}/index.css`)
      return { path, base: dirname(path), content: readFileSync(path, 'utf8') }
    },
  })
  return compiler.build(candidates)
}

describe('admin token closure', () => {
  it('compiles admin stacking utilities with their native z-index tokens', async () => {
    const layers = ['sticky', 'header', 'overlay', 'dialog']
    const css = await build(layers.map((layer) => `z-admin-${layer}`))

    for (const layer of layers) {
      const rule = new RegExp(
        `\\.z-admin-${layer}\\s*\\{\\s*z-index: var\\(--z-index-admin-${layer}\\);\\s*\\}`,
      )
      expect(rule.test(css), `compiled z-admin-${layer} utility`).toBe(true)
    }
  })
})

describe('tier role native utility emission', () => {
  it('emits app stacking utilities from --z-index-* without manual definitions', async () => {
    const css = await build(['z-banner', 'z-lightbox', 'z-content', 'z-popover'])
    for (const [utility, token] of [
      ['z-banner', '--z-index-banner'],
      ['z-lightbox', '--z-index-lightbox'],
      ['z-content', '--z-index-content'],
      ['z-popover', '--z-index-popover'],
    ] as const) {
      expect(css, `compiled ${utility}`).toMatch(
        new RegExp(`\\.${utility}\\s*\\{\\s*z-index: var\\(${token}\\);\\s*\\}`),
      )
    }
  })

  it('emits radius, shadow, and type utilities from their tier roles', async () => {
    const css = await build([
      'rounded-board-dialog',
      'rounded-admin-dialog',
      'shadow-board-dialog',
      'shadow-focus-ring',
      'shadow-floating',
      'text-admin-body',
      'type-body',
      'type-code',
      'bg-admin-card',
      'bg-chart-1',
    ])
    expect(css).toMatch(/\.rounded-board-dialog\s*\{\s*border-radius: var\(--radius-board-dialog\)/)
    expect(css).toMatch(/\.rounded-admin-dialog\s*\{\s*border-radius: var\(--radius-admin-dialog\)/)
    // Authored elevation recipes resolve through physical ingredients.
    expect(css).toMatch(
      /\.shadow-board-dialog\s*\{[^}]*--tw-shadow: var\(--primitive-shadow-22-58-soft\)/,
    )
    expect(css).toMatch(
      /\.shadow-focus-ring\s*\{[^}]*--tw-shadow: 0 0 0 3px var\(--tw-shadow-color, var\(--ring\)\)/,
    )
    expect(css).toMatch(/\.shadow-floating\s*\{[^}]*--tw-shadow: var\(--primitive-shadow-xl\)/)
    expect(css).toMatch(/\.text-admin-body\s*\{\s*font-size: var\(--text-admin-body\)/)
    expect(css).toMatch(/\.type-body\s*\{\s*font-size: var\(--text-admin-body\)/)
    expect(css).toMatch(/\.type-code\s*\{[^}]*font-family: var\(--font-mono\)/)
    expect(css).toMatch(/\.bg-admin-card\s*\{\s*background-color: var\(--color-admin-card\)/)
    expect(css).toMatch(/\.bg-chart-1\s*\{\s*background-color: var\(--chart-1\)/)
  })
})
