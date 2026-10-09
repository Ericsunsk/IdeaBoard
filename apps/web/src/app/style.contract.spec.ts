import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, resolve } from 'node:path'
import { compile } from 'tailwindcss'
import { describe, expect, it } from 'vitest'

const stylesheet = readFileSync(resolve(process.cwd(), 'src/app/style.css'), 'utf8')
const require = createRequire(import.meta.url)

describe('global scrollbar skin', () => {
  it('keeps the idle thumb readable on admin surfaces', () => {
    expect(stylesheet).toContain(
      '--color-scrollbar-thumb: color-mix(in srgb, var(--color-foreground) 58%, transparent);',
    )
    expect(stylesheet).not.toMatch(/--color-ib-scrollbar-/)
  })
})

describe('token namespace contract', () => {
  it('rejects removed product-prefixed color, type, and generic shadow aliases', () => {
    expect(stylesheet).not.toMatch(
      /--(?:color-ib-[a-z0-9-]+|text-ib-[a-z0-9-]+|shadow-ib-(?:control|overlay|focus))\b/,
    )
  })
})

describe('admin token closure (audit gap B)', () => {
  it('declares the destructive soft tint alongside the other status softs', () => {
    expect(stylesheet).toContain('--color-destructive-soft:')
  })

  it('declares status glow/wash colors and dot-halo shadows for the monitor domain', () => {
    for (const status of ['default', 'pending', 'running', 'success', 'danger']) {
      expect(stylesheet).toContain(`--color-admin-status-${status}-glow:`)
      expect(stylesheet).toContain(`--shadow-admin-status-dot-${status}:`)
    }
    expect(stylesheet).toContain('--color-admin-status-wash:')
  })

  it('declares provider brand hues, scrims, and primary transparent tiers', () => {
    for (const provider of ['purple', 'orange', 'blue', 'rose']) {
      expect(stylesheet).toContain(`--color-provider-${provider}:`)
    }
    expect(stylesheet).toContain('--color-scrim:')
    expect(stylesheet).toContain('--color-scrim-soft:')
    for (const tier of [
      'faint',
      'faint-soft',
      'faint-strong',
      'faintest',
      'line',
      'line-strong',
      'line-emphasis',
      'line-bold',
    ]) {
      expect(stylesheet).toContain(`--color-primary-${tier}:`)
    }
  })

  it('declares admin elevation, layering, weight, radius, and spacing tokens', () => {
    for (const token of [
      '--shadow-admin-dialog:',
      '--shadow-admin-drawer:',
      '--shadow-admin-popover:',
      '--z-admin-sticky:',
      '--z-admin-header:',
      '--z-admin-overlay:',
      '--z-admin-dialog:',
      '--font-weight-admin-strong:',
      '--font-weight-admin-emphasis:',
      '--font-weight-admin-display:',
      '--radius-admin-card:',
      '--radius-admin-control:',
      '--radius-admin-chip:',
      '--spacing-admin-shell:',
      '--spacing-admin-popover:',
      '--spacing-admin-drawer:',
      '--spacing-admin-empty-state:',
      '--spacing-admin-login-card:',
    ]) {
      expect(stylesheet).toContain(token)
    }
  })

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

describe('user-side token closure (dialog reuse + overlay-btn/canvas wash)', () => {
  it('declares image overlay action fills for search result cards', () => {
    expect(stylesheet).toContain('--color-overlay-btn-bg:')
    expect(stylesheet).toContain('--color-overlay-btn-bg-hover:')
  })

  it('declares the canvas tonal-range wash', () => {
    expect(stylesheet).toContain('--color-canvas-accent-wash:')
  })

  it('keeps reusing the shared dialog mask instead of a dialog-specific overlay token', () => {
    expect(stylesheet).toContain('--color-dialog-mask:')
    expect(stylesheet).not.toMatch(/--color-dialog-overlay/)
  })
})

describe('Motion Policy CSS adapter', () => {
  it('owns semantic motion timing directly without legacy aliases', () => {
    expect(stylesheet).toContain('--motion-control-duration: 160ms;')
    expect(stylesheet).toContain('--motion-enter-duration: 220ms;')
    expect(stylesheet).toContain('--motion-exit-duration: 220ms;')
    expect(stylesheet).toContain('--motion-feedback-duration: 160ms;')

    for (const intent of ['control', 'enter', 'exit', 'feedback']) {
      expect(stylesheet).toContain(`--motion-${intent}-easing: cubic-bezier(0.2, 0.8, 0.2, 1);`)
    }

    expect(stylesheet).not.toContain('--duration-ib-')
    expect(stylesheet).not.toContain('--ease-ib-')
  })

  it('globally stops semantic motion durations when reduced-motion is preferred', () => {
    expect(stylesheet).toContain('@media (prefers-reduced-motion: reduce)')
    expect(stylesheet).toContain('--motion-control-duration: 0ms;')
    expect(stylesheet).toContain('--motion-enter-duration: 0ms;')
    expect(stylesheet).toContain('--motion-exit-duration: 0ms;')
    expect(stylesheet).toContain('--motion-feedback-duration: 0ms;')
  })
})
