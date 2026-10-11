import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { adminThemeTokenFallbacks } from './adminThemeTokens'

/**
 * Contract: every text ramp clears WCAG 2.x AA against the surfaces it rests on.
 *
 * The role vocabulary and the floors are documented in `apps/web/DESIGN.md`
 * ("Text Color Ramp Standard"). This test is the machine-checked half of that
 * standard: it pins the surface inventory per ramp, composites alpha values the
 * way a browser does, and fails loudly when a token edit drops a role below its
 * floor, collapses two rungs, or desynchronizes the theme-token bridge.
 *
 * Why a contract test rather than a review rule
 * ---------------------------------------------
 * The three ramps sit on 17+ distinct backgrounds (page, card, translucent
 * card/shell, control, muted and every alpha step of muted that components layer
 * over card). Re-auditing them by hand is not repeatable, and a well-meaning
 * token tweak silently pushes a caption below AA on a surface nobody re-checked.
 * Three real defects shipped that way before this test existed:
 *   - `--color-canvas-text-secondary` at 3.90:1 on the Canvas workspace
 *   - `--color-board-card-text` at 3.83:1 on card
 *   - `--color-admin-text-2` at 4.43:1 on `--color-admin-surface-muted`
 */

type Rgb = readonly [number, number, number]

const styleSheet = readFileSync(join(import.meta.dirname, '../../../../app/style.css'), 'utf8')

/** Reads a solid hex token from the token source of truth. */
function readToken(name: string): string {
  const match = styleSheet.match(new RegExp(`${name}:\\s*([^;]+);`))
  if (!match?.[1]) throw new Error(`Token ${name} not found in app/style.css`)
  const value = match[1].trim()
  if (value.startsWith('#')) return value
  const varMatch = value.match(/var\((--[a-z0-9-]+)\)/)
  if (varMatch?.[1]) {
    return readToken(varMatch[1])
  }
  return value
}

/** Reads a translucent token as [rgb, alpha] so the test can composite it like a browser does. */
function readRgbaToken(name: string): { rgb: Rgb; alpha: number } {
  const match = styleSheet.match(
    new RegExp(
      `${name}:\\s*rgba?\\(\\s*(\\d+)\\s*,\\s*(\\d+)\\s*,\\s*(\\d+)\\s*,\\s*([\\d.]+)\\s*\\)`,
    ),
  )
  if (!match) throw new Error(`Translucent token ${name} not found in app/style.css`)
  return {
    rgb: [Number(match[1]), Number(match[2]), Number(match[3])] as const,
    alpha: Number(match[4]),
  }
}

const parseHex = (value: string): Rgb => {
  const hex = value.trim().replace('#', '')
  if (!/^[0-9a-f]{6}$/i.test(hex)) {
    throw new Error(`Expected a 6-digit hex color, received "${value}"`)
  }
  return [
    Number.parseInt(hex.slice(0, 2), 16),
    Number.parseInt(hex.slice(2, 4), 16),
    Number.parseInt(hex.slice(4, 6), 16),
  ] as const
}

const channel = (raw: number): number => {
  const c = raw / 255
  return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
}

const relativeLuminance = ([r, g, b]: Rgb): number =>
  0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)

export const contrastRatio = (foreground: string, background: string): number => {
  const a = relativeLuminance(parseHex(foreground))
  const b = relativeLuminance(parseHex(background))
  const [lighter, darker] = a > b ? [a, b] : [b, a]
  return (lighter + 0.05) / (darker + 0.05)
}

const compositeOver = (top: Rgb, alpha: number, bottom: Rgb): Rgb => [
  top[0] * alpha + bottom[0] * (1 - alpha),
  top[1] * alpha + bottom[1] * (1 - alpha),
  top[2] * alpha + bottom[2] * (1 - alpha),
]

const toHex = ([r, g, b]: Rgb): string =>
  `#${[r, g, b].map((c) => Math.round(c).toString(16).padStart(2, '0')).join('')}`

/** Documented floors from DESIGN.md. `text-primary` is held to AAA deliberately. */
const FLOORS = {
  primary: 7,
  secondary: 4.5,
  label: 4.5,
  tertiary: 4.5,
  muted: 4.5,
} as const

type Role = keyof typeof FLOORS

/** Extra headroom required on the binding surface so future tweaks have room. */
const BINDING_MARGIN = 0.2

const role = (id: Role, value: string) => ({ id, value })
const alphaStep = (id: string, token: Rgb, alpha: number, backdrop: Rgb) => ({
  id,
  value: toHex(compositeOver(token, alpha, backdrop)),
})

/* -------------------------------------------------------------------------- */
/* Admin                                                                       */
/* -------------------------------------------------------------------------- */

const { bg, card: cardStrong, controlBg, textPrimary } = adminThemeTokenFallbacks
const bgRgb = parseHex(bg)
const cardRgb = parseHex(cardStrong)
const cardLayer = readRgbaToken('--color-admin-card')
const shellLayer = readRgbaToken('--color-admin-shell')
const surfaceMuted = readToken('--color-admin-surface-muted')

const ADMIN_RAMP = [
  role('primary', textPrimary),
  role('label', readToken('--color-admin-text-label')),
  role('secondary', readToken('--color-admin-text-2')),
  role('tertiary', readToken('--color-admin-text-3')),
] as const

/**
 * `--color-admin-surface-hover` is the binding surface: `AdminTableRow` and
 * popover rows paint `hover:bg-admin-surface-hover` without switching the label
 * to `text-1`, so secondary/tertiary text genuinely rests on it.
 * `--color-admin-surface-pressed` is darker but only appears in `active:`
 * states, which always pair with `text-1` (asserted separately below).
 */
const ADMIN_SURFACES = [
  { id: 'bg', value: bg },
  { id: 'card-strong', value: cardStrong },
  {
    id: 'card (alpha over bg)',
    value: toHex(compositeOver(cardLayer.rgb, cardLayer.alpha, bgRgb)),
  },
  {
    id: 'shell (alpha over bg)',
    value: toHex(compositeOver(shellLayer.rgb, shellLayer.alpha, bgRgb)),
  },
  { id: 'control-bg', value: controlBg },
  { id: 'surface-muted', value: surfaceMuted },
  ...[0.8, 0.6, 0.5, 0.4, 0.3, 0.2].map((alpha) =>
    alphaStep(`surface-muted/${alpha * 100} over card`, parseHex(surfaceMuted), alpha, cardRgb),
  ),
  { id: 'surface-hover', value: readToken('--color-admin-surface-hover') },
  // surface-disabled and preview-bg alias surface-muted, so they add no distinct
  // surface to this inventory; the alias assertions below guard that link.
  alphaStep('bg/50 over card', bgRgb, 0.5, cardRgb),
  alphaStep('bg/30 over card', bgRgb, 0.3, cardRgb),
] as const

const ADMIN_BINDING = 'surface-hover'

/* -------------------------------------------------------------------------- */
/* Canvas                                                                      */
/* -------------------------------------------------------------------------- */

const CANVAS_RAMP = [
  role('primary', readToken('--color-canvas-text-primary')),
  role('secondary', readToken('--color-canvas-text-secondary')),
  role('muted', readToken('--color-canvas-text-muted')),
] as const

/** The workspace backdrop is darker than any node, so it binds the Canvas ramp. */
const CANVAS_SURFACES = [
  { id: 'workspace-bg', value: readToken('--color-canvas-workspace-bg') },
  { id: 'surface-bg (node)', value: readToken('--color-canvas-surface-bg') },
] as const

const CANVAS_BINDING = 'workspace-bg'

/* -------------------------------------------------------------------------- */
/* Boards                                                                      */
/* -------------------------------------------------------------------------- */

const BOARD_RAMP = [
  role('primary', readToken('--color-board-page-text')),
  role('secondary', readToken('--color-board-page-text-muted')),
  role('muted', readToken('--color-board-card-text')),
] as const

const boardPage = readToken('--color-board-page')
const boardSurfaceLayer = readRgbaToken('--color-board-surface')
const BOARD_SURFACES = [
  { id: 'page', value: boardPage },
  {
    id: 'card-bg (alpha over page)',
    value: toHex(compositeOver(parseHex('#ffffff'), boardSurfaceLayer.alpha, parseHex(boardPage))),
  },
] as const

const BOARD_BINDING = 'page'

/* -------------------------------------------------------------------------- */

const RAMPS = [
  {
    name: 'Admin',
    ramp: ADMIN_RAMP,
    surfaces: ADMIN_SURFACES,
    binding: ADMIN_BINDING,
  },
  {
    name: 'Canvas',
    ramp: CANVAS_RAMP,
    surfaces: CANVAS_SURFACES.filter((s) => s.value !== ''),
    binding: CANVAS_BINDING,
  },
  {
    name: 'Boards',
    ramp: BOARD_RAMP,
    surfaces: BOARD_SURFACES,
    binding: BOARD_BINDING,
  },
] as const

describe('text ramp contrast contract', () => {
  for (const { name, ramp, surfaces, binding } of RAMPS) {
    describe(`${name} ramp`, () => {
      it('declares a binding surface that exists in its inventory', () => {
        expect(
          surfaces.some((s) => s.id === binding),
          `${name} binding surface "${binding}" is missing from the inventory`,
        ).toBe(true)
      })

      for (const surface of surfaces) {
        it.each(ramp)(`$id clears its floor on ${surface.id}`, (token) => {
          const ratio = contrastRatio(token.value, surface.value)
          expect(
            ratio,
            `${name} ${token.id} (${token.value}) on ${surface.id} (${surface.value}) is ${ratio.toFixed(2)}:1, below the ${FLOORS[token.id]}:1 floor`,
          ).toBeGreaterThanOrEqual(FLOORS[token.id])
        })
      }

      it('keeps headroom on the binding surface rather than sitting on the threshold', () => {
        const target = surfaces.find((s) => s.id === binding)
        if (!target) throw new Error(`${name} binding surface "${binding}" not found`)
        for (const token of ramp) {
          const ratio = contrastRatio(token.value, target.value)
          expect(
            ratio,
            `${name} ${token.id} on ${binding} is ${ratio.toFixed(2)}:1; less than ${BINDING_MARGIN} of headroom leaves no room for future surface tweaks`,
          ).toBeGreaterThanOrEqual(FLOORS[token.id] + BINDING_MARGIN)
        }
      })

      it('keeps rungs strictly monotonic and visually distinct', () => {
        for (let i = 1; i < ramp.length; i += 1) {
          const previous = ramp[i - 1]!
          const current = ramp[i]!
          const previousLuminance = relativeLuminance(parseHex(previous.value))
          const currentLuminance = relativeLuminance(parseHex(current.value))

          expect(
            currentLuminance,
            `${name} ${current.id} (L=${currentLuminance.toFixed(4)}) must be lighter than ${previous.id} (L=${previousLuminance.toFixed(4)})`,
          ).toBeGreaterThan(previousLuminance)

          const step = previousLuminance / currentLuminance
          // The primary -> secondary jump is intentionally large: primary is
          // held to AAA and reads as a different weight entirely. Only the
          // gradations below primary must stay subtle enough to be ordered but
          // distinct enough to be told apart.
          if (current.id !== 'secondary') {
            expect(
              step,
              `${name} ${previous.id} -> ${current.id} is ${step.toFixed(2)}x luminance; below 0.25 the rungs read as the same weight`,
            ).toBeGreaterThan(0.25)
          }
          expect(
            step,
            `${name} ${previous.id} -> ${current.id} is ${step.toFixed(2)}x luminance; above 0.95 the ramp collapsed into one level`,
          ).toBeLessThan(0.95)
        }
      })
    })
  }

  it('keeps text-1 readable on surface-pressed, which only active: states reach', () => {
    // surface-pressed is darker than the Admin binding surface but is only
    // reachable through `active:` states, which pair with text-1. If a future
    // change paints resting text there, add it to ADMIN_SURFACES.
    expect(
      contrastRatio(textPrimary, readToken('--color-admin-surface-pressed')),
      'text-1 must stay readable on surface-pressed',
    ).toBeGreaterThanOrEqual(FLOORS.primary)
  })
})
