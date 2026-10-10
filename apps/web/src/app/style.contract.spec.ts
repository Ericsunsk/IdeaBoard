import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, resolve } from 'node:path'
import { compile } from 'tailwindcss'
import { describe, expect, it } from 'vitest'

const stylesheet = readFileSync(resolve(process.cwd(), 'src/app/style.css'), 'utf8')
const require = createRequire(import.meta.url)

describe('admin token closure (audit gap B)', () => {
  it('compiles admin stacking utilities with their semantic z-index tokens', async () => {
    const compiler = await compile(stylesheet, {
      base: resolve(process.cwd(), 'src/app'),
      loadStylesheet: async (id, base) => {
        const path = id.startsWith('.') ? resolve(base, id) : require.resolve(`${id}/index.css`)
        return { path, base: dirname(path), content: readFileSync(path, 'utf8') }
      },
    })
    const layers = ['sticky', 'header', 'overlay', 'dialog']
    const css = compiler.build(layers.map((layer) => `z-admin-${layer}`))

    for (const layer of layers) {
      const rule = new RegExp(
        `\\.z-admin-${layer}\\s*\\{\\s*z-index: var\\(--z-admin-${layer}\\);\\s*\\}`,
      )
      expect(rule.test(css), `compiled z-admin-${layer} utility`).toBe(true)
    }
  })
})
