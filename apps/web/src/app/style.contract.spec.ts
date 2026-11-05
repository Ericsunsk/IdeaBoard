import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, resolve } from 'node:path'
import { compile } from 'tailwindcss'
import { parse } from 'postcss'
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
    const layers = ['sticky', 'header', 'overlay', 'dialog', 'confirm']
    const css = await build(layers.map((layer) => `z-admin-${layer}`))

    for (const layer of layers) {
      const rule = new RegExp(
        `\\.z-admin-${layer}\\s*\\{\\s*z-index: var\\(--z-index-admin-${layer}\\);\\s*\\}`,
      )
      expect(rule.test(css), `compiled z-admin-${layer} utility`).toBe(true)
    }
  })

  it('pins admin confirm above the config dialog and popover tiers', async () => {
    // Regression: the global admin confirm shared the dialog tier, so same-tier
    // portal/DOM order decided the winner and the confirm often lost.
    const roles = readFileSync(resolve(process.cwd(), 'src/theme/tokens/z.css'), 'utf8')
    const roleTarget = (role: string): string => {
      const match = roles.match(new RegExp(`${role}:\\s*var\\((--[\\w-]+)\\)`))
      expect(match, `declared ${role}`).not.toBeNull()
      return match![1]!
    }

    // Compile existence only: numeric order lives in token-graph
    // (checkTokenGraph) and the motion/z stacking in overlayMenuContract.
    for (const role of [
      '--z-index-admin-dialog',
      '--z-index-popover',
      '--z-index-admin-confirm',
      '--z-index-content',
    ]) {
      expect(roleTarget(role), 'Tier 2 stacking role targets a Tier 1 step').toMatch(
        /^--primitive-z-\d+$/,
      )
    }

    // Confirm must ride the new tier; the config dialog stays on the dialog
    // tier so numeric order (not DOM order) decides the winner.
    const confirmDialog = readFileSync(
      resolve(process.cwd(), 'src/core/components/ConfirmDialog.vue'),
      'utf8',
    )
    expect(confirmDialog).toContain('z-admin-confirm')
    expect(confirmDialog).not.toContain('z-admin-dialog')
    const adminShell = readFileSync(
      resolve(process.cwd(), 'src/features/admin/shared/ui/shell/AdminDialogShell.vue'),
      'utf8',
    )
    expect(adminShell).toContain('z-admin-dialog')
    expect(adminShell).not.toContain('z-admin-confirm')
    const adminMasks = readFileSync(
      resolve(process.cwd(), 'src/features/admin/shared/ui/styling/uiClasses.ts'),
      'utf8',
    )
    expect(adminMasks).toContain('z-admin-dialog')
    expect(adminMasks).not.toContain('z-admin-confirm')

    // Computed cascade: compiled utilities resolve through the token graph.
    // Numeric order is owned by token-graph/overlayMenuContract; here only
    // assert each utility compiles to its stacking token.
    const css = await build(['z-admin-confirm', 'z-admin-dialog', 'z-popover', 'z-content'])
    for (const utility of ['z-admin-confirm', 'z-admin-dialog', 'z-popover', 'z-content']) {
      expect(css, `compiled ${utility}`).toMatch(
        new RegExp(`\\.${utility}\\s*\\{\\s*z-index: var\\(--z-index-[\\w-]+\\)`),
      )
    }
  })
})

describe('tier role native utility emission', () => {
  it('emits named overlay animations for Presence and disables them under reduced motion', async () => {
    const css = parse(await build([]))
    const animations = new Map<string, string>()
    const keyframes = new Set<string>()
    let hasReducedMotion = false

    css.walkRules((rule) => {
      if (!rule.selector.includes('.overlay-dialog-motion')) return
      rule.walkDecls('animation', (declaration) => {
        if (rule.selector.includes("[data-state='open']")) {
          animations.set('enter', declaration.value)
        }
        if (rule.selector.includes("[data-state='closed']")) {
          animations.set('exit', declaration.value)
        }
      })
    })
    css.walkAtRules('keyframes', (rule) => {
      keyframes.add(rule.params)
    })
    css.walkAtRules('media', (rule) => {
      if (rule.params !== '(prefers-reduced-motion: reduce)') return
      rule.walkRules((motionRule) => {
        if (!motionRule.selector.includes('.overlay-dialog-motion')) return
        motionRule.walkDecls('animation', (declaration) => {
          hasReducedMotion ||= declaration.value === 'none'
        })
      })
    })

    for (const intent of ['enter', 'exit']) {
      expect(keyframes.has(`overlay-${intent}`)).toBe(true)
      expect(animations.get(intent)).toBe(
        `overlay-${intent} var(--motion-${intent}-duration) var(--motion-${intent}-easing) both`,
      )
    }
    expect(hasReducedMotion).toBe(true)
  })

  it('provides shared control, panel, overlay, and pill radii', async () => {
    const roles = { control: 8, panel: 12, overlay: 16, pill: 9999 }
    const css = await build(Object.keys(roles).map((role) => `rounded-${role}`))

    for (const [role, pixels] of Object.entries(roles)) {
      expect(css).toContain(`--primitive-radius-${role === 'pill' ? role : pixels}: ${pixels}px`)
      expect(css).toContain(
        `--radius-${role}: var(--primitive-radius-${role === 'pill' ? role : pixels})`,
      )
      expect(css).toMatch(
        new RegExp(`\\.rounded-${role}\\s*\\{\\s*border-radius: var\\(--radius-${role}\\)`),
      )
    }
  })

  it('disables the framework radius size ladder', async () => {
    const sizes = ['xs', 'sm', 'md', 'lg', 'xl', '2xl', '3xl', '4xl']
    const css = await build(sizes.map((size) => `rounded-${size}`))

    for (const size of sizes) {
      expect(css).not.toMatch(new RegExp(`\\.rounded-${size}\\s*\\{`))
    }
  })

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
      'rounded-overlay',
      'shadow-board-dialog',
      'shadow-focus-ring',
      'shadow-floating',
      'text-admin-body',
      'type-body',
      'type-code',
      'bg-admin-card',
      'bg-chart-1',
    ])
    expect(css).toMatch(/\.rounded-overlay\s*\{\s*border-radius: var\(--radius-overlay\)/)
    // Mode-dependent elevation recipes resolve through live runtime roles.
    expect(css).toMatch(/\.shadow-board-dialog\s*\{[^}]*--tw-shadow: var\(--board-dialog-shadow\)/)
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
