// @vitest-environment node
import { describe, expect, it } from 'vitest'
import {
  contrastOf,
  parseOwnerDecls,
  resolveAlphaColor,
  scopeValues,
} from '../../../../../scripts/checkTokenGraph'

/**
 * Contract: every text ramp clears WCAG 2.x AA against the surfaces it rests on.
 *
 * The role vocabulary and the floors are documented in `apps/web/DESIGN.md`
 * ("Text Color Ramp Standard"). This test is the machine-checked half of that
 * standard: it pins the surface inventory per ramp, composites alpha values the
 * way a browser does, and fails loudly when a token edit drops a role below its
 * floor, collapses two rungs, or desynchronizes the theme-token bridge.
 */

type Rgb = readonly [number, number, number]

/** Read authored light values with base-layer declarations taking precedence. */
const tokenValues = scopeValues(parseOwnerDecls(), 'light')

function tokenValue(name: string): string {
  const value = tokenValues.get(name)?.value
  if (value === undefined) throw new Error(`Token ${name} is not assigned in light mode`)
  return value
}

/** Resolve references and composition without discarding authored alpha. */
function readToken(name: string): string {
  const color = resolveAlphaColor(tokenValue(name), tokenValue)
  if (color.alpha !== 1) throw new Error(`Expected opaque token ${name}`)
  return toHex(color.rgb)
}

function readRgbaToken(name: string): { rgb: Rgb; alpha: number } {
  return resolveAlphaColor(tokenValue(name), tokenValue)
}

const parseHex = (value: string): Rgb => {
  const color = resolveAlphaColor(value, tokenValue)
  if (color.alpha !== 1) throw new Error(`Expected an opaque color, received "${value}"`)
  return color.rgb
}

const channel = (raw: number): number => {
  const c = raw / 255
  return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
}

const relativeLuminance = ([r, g, b]: Rgb): number =>
  0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)

export const contrastRatio = (foreground: string, background: string): number =>
  contrastOf(parseHex(foreground), parseHex(background))

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

const bg = readToken('--color-admin-bg')
const cardStrong = readToken('--color-admin-card-strong')
const controlBg = readToken('--color-admin-control-bg')
const textPrimary = readToken('--color-admin-text-1')
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
  // Folded duplicates (surface-disabled, preview-bg, control-bg-hover,
  // control-border*) resolve through surface-muted/surface-hover/border, so
  // they add no distinct surface to this inventory.
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
