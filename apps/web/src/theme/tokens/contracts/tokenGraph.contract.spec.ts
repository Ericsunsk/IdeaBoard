// @vitest-environment node
import { describe, expect, it } from 'vitest'
import {
  checkAcyclic,
  checkContrast,
  checkParity,
  checkTierDirection,
  checkTokenGraph,
  contrastInventoryReport,
  contrastOf,
  parseColor,
  type TokenDecl,
} from '../../../../scripts/checkTokenGraph'

/**
 * Token graph gate fixtures: positive cases prove legitimate geometry
 * passes; negative cases prove raw values, primitive leaks, broken modes,
 * cycles, and low contrast fail.
 */

const decl = (
  file: string,
  name: string,
  value: string,
  scope: TokenDecl['scope'] = 'light',
): TokenDecl => ({ file, name, value, scope, isThemeKey: false })

const themeDecl = (file: string, name: string, value: string): TokenDecl => ({
  file,
  name,
  value,
  scope: 'light',
  isThemeKey: true,
})

function rules(violations: { rule: string }[]): string[] {
  return violations.map((violation) => violation.rule)
}

describe('token color math', () => {
  it('resolves black on white to 21:1', () => {
    expect(
      contrastOf(
        parseColor('#000000', () => ''),
        parseColor('#ffffff', () => ''),
      ),
    ).toBeCloseTo(21, 0)
  })

  it('parses space-syntax rgb and follows var chains', () => {
    expect(parseColor('rgb(239 68 68 / 15%)', () => '')).toEqual([239, 68, 68])
    expect(
      parseColor('var(--primary)', (name) =>
        name === '--primary' ? 'var(--primitive-violet-650)' : '#7a6bd5',
      ),
    ).toEqual([122, 107, 213])
  })

  it('rejects unresolvable references instead of guessing', () => {
    expect(() => parseColor('var(--missing)', () => '')).toThrow()
  })

  it.each([
    'rgb(from #000 r g b)',
    'rgb(0 0)',
    'rgb(0 0 0 0)',
    'rgb(0 0 nope)',
    'rgba(0, 0, 0, NaN)',
    'rgb(0 0 0 / Infinity)',
    'rgb(0 0 0 / 0.5garbage)',
    'rgb(1e999 0 0)',
  ])('rejects unsupported or non-finite color syntax: %s', (value) => {
    expect(() => parseColor(value, () => '')).toThrow()
  })

  it('rejects non-finite contrast inputs', () => {
    expect(() => contrastOf([Number.NaN, 0, 0], [255, 255, 255])).toThrow()
    expect(() => contrastOf([0, 0, 0], [Number.POSITIVE_INFINITY, 255, 255])).toThrow()
    expect(() => contrastOf([1e308, 1e308, 1e308], [0, 0, 0])).toThrow()
  })
})

describe('token graph integrity fixtures', () => {
  it('accepts an acyclic tier-correct graph', () => {
    const decls = [
      decl('theme/tokens/primitives.css', '--primitive-neutral-0', '#ffffff'),
      decl('theme/tokens/semantic.css', '--card', 'var(--primitive-neutral-0)'),
      decl('theme/tokens/canvas.css', '--color-canvas-node-bg', 'var(--card)'),
    ]
    expect(checkAcyclic(decls)).toEqual([])
    expect(rules(checkTierDirection(decls))).toEqual([])
  })

  it('rejects cyclic references', () => {
    const decls = [
      decl('theme/tokens/semantic.css', '--card', 'var(--popover)'),
      decl('theme/tokens/semantic.css', '--popover', 'var(--card)'),
    ]
    expect(rules(checkAcyclic(decls))).toContain('token-cycle')
  })

  it('checks the runtime Canvas owner rather than its earlier theme fallback', () => {
    const decls = [
      themeDecl('theme/theme.css', '--color-canvas-node-bg', '#ffffff'),
      decl('theme/tokens/canvas.css', '--color-canvas-node-bg', 'var(--color-canvas-node-bg)'),
    ]
    expect(rules(checkAcyclic(decls))).toContain('token-cycle')
  })

  it('checks the last declaration in each runtime scope', () => {
    const decls = [
      decl('theme/tokens/semantic.css', '--card', '#ffffff'),
      decl('theme/tokens/semantic.css', '--card', 'var(--card)'),
    ]
    expect(rules(checkAcyclic(decls))).toContain('token-cycle')
  })

  it('prefers runtime assignments over theme fallbacks in both modes', () => {
    const decls = [
      decl('theme/tokens/canvas.css', '--color-canvas-node-bg', '#ffffff'),
      decl('theme/tokens/canvas.css', '--color-canvas-node-bg', '#000000', 'dark'),
      themeDecl('theme/theme.css', '--color-canvas-node-bg', 'var(--color-canvas-node-bg)'),
    ]
    expect(checkAcyclic(decls)).toEqual([])
  })

  it('keeps inline utility mappings out of the runtime graph', () => {
    const decls = [
      decl('theme/tokens/primitives.css', '--primitive-white', '#ffffff'),
      decl('theme/tokens/primitives.css', '--primitive-black', '#000000'),
      decl('theme/tokens/semantic.css', '--card', 'var(--primitive-white)'),
      decl('theme/tokens/semantic.css', '--card', 'var(--primitive-black)', 'dark'),
      {
        ...decl('theme/theme.css', '--card', 'var(--card)', 'scoped'),
        isInlineTheme: true,
      },
      {
        ...decl(
          'theme/theme.css',
          '--color-canvas-node-bg',
          'var(--color-canvas-node-bg)',
          'scoped',
        ),
        isInlineTheme: true,
      },
    ]
    expect(checkAcyclic(decls)).toEqual([])
    expect(checkParity(decls)).toEqual([])
    expect(rules(checkTierDirection(decls))).toEqual([])
  })

  it('rejects upward references out of Tier 1', () => {
    const decls = [
      decl('theme/tokens/semantic.css', '--card', '#ffffff'),
      decl('theme/tokens/primitives.css', '--primitive-neutral-0', 'var(--card)'),
    ]
    expect(rules(checkTierDirection(decls))).toContain('tier-direction')
  })

  it('rejects Tier 2 references to a Canvas runtime owner shadowing a theme fallback', () => {
    const decls = [
      themeDecl('theme/theme.css', '--color-canvas-node-bg', '#ffffff'),
      decl('theme/tokens/canvas.css', '--color-canvas-node-bg', '#ffffff'),
      decl('theme/tokens/semantic.css', '--card', 'var(--color-canvas-node-bg)'),
    ]
    expect(checkTierDirection(decls)).toContainEqual({
      rule: 'tier-direction',
      file: 'theme/tokens/semantic.css',
      message: 'Tier 2 --card must not reference --color-canvas-node-bg (tier t3).',
    })
  })

  it.each(['--primary', '--card', '--background', '--ring'])(
    'rejects bare-role raw literals: %s',
    (name) => {
      expect(
        rules(checkTierDirection([decl('theme/tokens/semantic.css', name, '#123456')])),
      ).toContain('tier-raw-literal')
    },
  )

  it('rejects Tier 3 derivations from Tier 1', () => {
    const decls = [
      decl('theme/tokens/primitives.css', '--primitive-shadow-xl', '0 1px 2px black'),
      decl('theme/tokens/canvas.css', '--color-canvas-widget', 'var(--primitive-shadow-xl)'),
    ]
    expect(rules(checkTierDirection(decls))).toContain('tier-direction')
  })

  it.each(['--primary', '--shadow-floating'])('rejects authored white and black in %s', (name) => {
    for (const value of ['#ffffff', '#000000', '0 1px 2px black']) {
      expect(rules(checkTierDirection([decl('theme/tokens/semantic.css', name, value)]))).toContain(
        'tier-raw-literal',
      )
    }
  })

  it.each([
    ['theme/tokens/semantic.css', 'app/style.css'],
    ['theme/tokens/canvas.css', 'app/style.css'],
    ['theme/tokens/canvas.css', 'theme/tokens/canvas.css'],
  ])('rejects forbidden owner edges from %s to %s', (from, to) => {
    const decls = [
      decl(to, '--test-source', '1px'),
      decl(from, '--test-target', 'var(--test-source)'),
    ]
    expect(rules(checkTierDirection(decls))).toContain('tier-direction')
  })

  it.each(['red', 'rebeccapurple', 'color-mix(in srgb, red 10%, transparent)', 'var(--card, red)'])(
    'rejects named color ingredients: %s',
    (value) => {
      expect(
        rules(checkTierDirection([decl('theme/tokens/semantic.css', '--card', value)])),
      ).toContain('tier-raw-literal')
    },
  )

  it('rejects named colors in shadow recipes while allowing referenced ink', () => {
    expect(
      rules(
        checkTierDirection([
          decl('theme/tokens/semantic.css', '--shadow-floating', '0 1px 2px rebeccapurple'),
        ]),
      ),
    ).toContain('tier-raw-literal')
    expect(
      checkTierDirection([
        decl('theme/tokens/semantic.css', '--shadow-floating', '0 1px 2px var(--ring)'),
      ]),
    ).toEqual([])
  })

  it('accepts legitimate geometry, mechanical keywords, and framework refs', () => {
    const decls = [
      decl('theme/tokens/primitives.css', '--primitive-neutral-0', '#ffffff'),
      decl('theme/tokens/semantic.css', '--card', 'var(--primitive-neutral-0)'),
      decl('theme/tokens/semantic.css', '--card', 'var(--primitive-neutral-0)', 'dark'),
      decl('theme/theme.css', '--spacing-overlay-gutter', '10px'),
      decl('theme/theme.css', '--color-scrollbar-track', 'transparent'),
      decl('theme/theme.css', '--shadow-admin-dialog', 'var(--shadow-xl)'),
      decl('theme/tokens/canvas.css', '--canvas-edge-width', '2'),
    ]
    expect(rules(checkTierDirection(decls))).toEqual([])
    expect(checkParity(decls)).toEqual([])
  })
})

describe('mode parity fixtures', () => {
  it('accepts exact color-role parity', () => {
    const decls = [
      decl('theme/tokens/semantic.css', '--card', '#ffffff', 'light'),
      decl('theme/tokens/semantic.css', '--card', '#0d0d0d', 'dark'),
    ]
    expect(checkParity(decls)).toEqual([])
  })

  it('rejects a missing dark assignment (broken mode)', () => {
    const decls = [decl('theme/theme.css', '--color-board-page', '#fbfbfb', 'light')]
    expect(rules(checkParity(decls))).toContain('mode-parity')
  })

  it.each(['--primary', '--card', '--background', '--ring'])(
    'rejects a missing dark assignment for bare role %s',
    (name) => {
      expect(rules(checkParity([decl('theme/tokens/semantic.css', name, '#123456')]))).toContain(
        'mode-parity',
      )
    },
  )

  it('rejects a dark-only role', () => {
    const decls = [decl('theme/theme.css', '--color-board-page', '#0f0f0f', 'dark')]
    expect(rules(checkParity(decls))).toContain('mode-parity')
  })
})

describe('contrast inventory fixtures', () => {
  function bodyReport(foreground: string, extra: TokenDecl[] = []) {
    return contrastInventoryReport([
      decl('theme/tokens/semantic.css', '--background', '#ffffff'),
      decl('theme/tokens/semantic.css', '--foreground', foreground),
      ...extra,
    ]).find((entry) => entry.label === 'body text' && entry.mode === 'light')!
  }

  it('reports a color parse failure instead of accepting NaN contrast', () => {
    const foreground = 'rgb(from #000000 r g b)'
    expect(bodyReport(foreground).error).toBeDefined()
    expect(
      checkContrast([
        decl('theme/tokens/semantic.css', '--background', '#ffffff'),
        decl('theme/tokens/semantic.css', '--foreground', foreground),
      ]),
    ).toContainEqual(expect.objectContaining({ rule: 'contrast-unresolvable' }))
  })

  it('resolves an existing token before its var fallback', () => {
    const report = bodyReport('var(--existing, #000000)', [
      decl('theme/tokens/semantic.css', '--existing', '#ffffff'),
    ])
    expect(report.error).toBeUndefined()
    expect(report.ratio).toBe(1)
  })

  it('uses nested fallbacks for missing token references', () => {
    const report = bodyReport('var(--missing, var(--also-missing, #000000))')
    expect(report.error).toBeUndefined()
    expect(report.ratio).toBe(21)
  })

  it('fails unsupported existing token values even when a fallback is available', () => {
    const report = bodyReport('var(--existing, #000000)', [
      decl('theme/tokens/semantic.css', '--existing', 'rgb(from #000000 r g b)'),
    ])
    expect(report.error).toBeDefined()
  })

  it.each(['rgba(0, 0, 0, 0.5)', 'rgb(0 0 0 / 50%)'])(
    'composites valid alpha syntax: %s',
    (foreground) => {
      const report = bodyReport(foreground)
      expect(report.error).toBeUndefined()
      expect(report.ratio).toBeCloseTo(contrastOf([127.5, 127.5, 127.5], [255, 255, 255]), 8)
    },
  )

  it('preserves source alpha when mixing a translucent color with transparent', () => {
    const report = bodyReport('color-mix(in srgb, rgba(0, 0, 0, 0.5) 50%, transparent)')
    expect(report.error).toBeUndefined()
    expect(report.ratio).toBeCloseTo(contrastOf([191.25, 191.25, 191.25], [255, 255, 255]), 8)
  })

  it('rejects malformed alpha and color-mix percentages', () => {
    expect(bodyReport('rgba(0, 0, 0, nope)').error).toBeDefined()
    expect(bodyReport('color-mix(in srgb, #000000 0..5%, transparent)').error).toBeDefined()
  })

  it('uses the runtime foreground and last source assignment instead of a theme fallback', () => {
    const report = bodyReport('#ffffff', [
      themeDecl('theme/theme.css', '--foreground', '#000000'),
      decl('theme/tokens/semantic.css', '--foreground', '#ffffff'),
    ])
    expect(report.error).toBeUndefined()
    expect(report.ratio).toBe(1)
  })

  it('fails a low-contrast pair instead of passing it', () => {
    const body = bodyReport('#777777')
    expect(body.error).toBeUndefined()
    expect(body.ratio).toBeLessThan(7)
  })

  it.each(['primary', 'destructive'])(
    'requires normal-size text contrast in every %s action state',
    (role) => {
      const decls = [
        decl('theme/tokens/semantic.css', '--background', '#ffffff'),
        decl('theme/tokens/semantic.css', `--${role}-foreground`, '#ffffff'),
        ...['', '-hover', '-pressed'].map((state) =>
          decl('theme/tokens/semantic.css', `--${role}${state}`, '#7a6bd5'),
        ),
      ]
      const entries = contrastInventoryReport(decls).filter(
        (entry) => entry.mode === 'light' && entry.label.startsWith(role),
      )
      expect(entries).toHaveLength(3)
      expect(
        entries.every(
          (entry) => entry.error === undefined && entry.ratio > 3 && entry.ratio < entry.floor,
        ),
      ).toBe(true)
      expect(
        checkContrast(decls).filter(
          (entry) => entry.rule === 'contrast-floor' && entry.message.startsWith('light '),
        ),
      ).toHaveLength(3)
    },
  )

  it('rejects a chart mark and a sidebar focus ring below their surface floors', () => {
    const decls = [
      decl('theme/tokens/semantic.css', '--background', '#ffffff'),
      decl('theme/tokens/semantic.css', '--card', '#ffffff'),
      decl('theme/tokens/semantic.css', '--chart-3', '#10b981'),
      decl('theme/tokens/semantic.css', '--sidebar', '#ffffff'),
      decl('theme/tokens/semantic.css', '--sidebar-ring', '#e5e5e5'),
    ]
    const violations = checkContrast(decls).filter(
      (entry) => entry.rule === 'contrast-floor' && entry.message.startsWith('light '),
    )
    expect(violations).toHaveLength(3)
    expect(violations.map((entry) => entry.message).join(' ')).toContain('chart 3 on card')
    expect(violations.map((entry) => entry.message).join(' ')).toContain('sidebar focus ring')
  })

  it('flags the low-contrast pair through the gate', () => {
    const decls = [
      decl('theme/tokens/semantic.css', '--background', '#ffffff', 'light'),
      decl('theme/tokens/semantic.css', '--background', '#000000', 'dark'),
      decl('theme/tokens/semantic.css', '--foreground', '#777777', 'light'),
      decl('theme/tokens/semantic.css', '--foreground', '#888888', 'dark'),
    ]
    const found = rules(checkContrast(decls))
    expect(found).toContain('contrast-floor')
  })
})

describe('token graph acceptance', () => {
  it('holds integrity, parity, and contrast on the real tree', () => {
    expect(checkTokenGraph()).toEqual([])
  })
})
