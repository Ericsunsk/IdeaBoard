import { readFileSync } from 'node:fs'
import { join } from 'node:path'

/**
 * Guard: interactive controls must reach the touch-target floor.
 *
 * A logged-in audit of Canvas Workspace, `/admin/monitor`, `/admin/statistics`,
 * `/admin/users`, `/admin/models`, `/admin/credits` and `/admin/settings` found
 * 30 controls under 24x24 CSS px, which fails WCAG 2.2 SC 2.5.8 (Target Size
 * Minimum, AA) outright, plus 16 more under this repo's stricter 32px floor:
 *
 *   AdminSwitch                     36x20   18 instances   switch track
 *   TaskEventsCard filter pills     ~x20     6 instances   dense chip rail
 *   canvas zoom buttons             23x23    4 instances   canvas status bar
 *   canvas catalog recovery         62x22    2 instances   canvas header
 *   admin range segments            32x24    6 instances   chart card header
 *   admin tabs triggers             ~x28      8 instances   sub-navigation rail
 *   node action buttons             ~x26      4 instances   canvas node overlay
 *   admin pager / chart collapse    28x28    5 instances   toolbar
 *   admin header scroll-to-top      ~x24      6 instances   sticky page title
 *   canvas board icon buttons       29x29    4 instances   canvas header
 *
 * The floor is `--spacing-touch-target-min` (32px). Controls that are
 * visually small by design carry `.hit-target-32` / `.hit-target-32-inline`
 * from `shared/theme/touch-target.css`, which grows the hit area past the visual
 * box without inflating the design. Controls that can afford it set a real
 * `min-h-8` / `!size-8`.
 *
 * This check fails if a control drops below the floor without opting in through
 * one of those two routes.
 */

/** The floor, in px. Must track `--spacing-touch-target-min` in style.css. */
const FLOOR_PX = 32

const SRC_ROOT = join(import.meta.dirname, '..', 'src')
const STYLE = join(SRC_ROOT, 'app/style.css')

const OPT_IN = /hit-target-32(?:-inline)?\b|hit-target-drag\b/

/**
 * Strip comments before testing. Several of these files *document* the hit-area
 * utility in a comment right next to the class, and a naive substring test reads
 * that prose as the control opting in — which silently disarmed the guard.
 */
function stripComments(source: string): string {
  return source.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')
}

/**
 * The region a match belongs to: the control that owns it.
 *
 * ## Why this exists
 *
 * The opt-in used to be tested against the **whole file**:
 *
 *     if (OPT_IN.test(source)) continue
 *
 * One `hit-target-32` anywhere in a file therefore exempted every control in
 * it. That is the wrong granularity for `lib/uiClasses.ts`, a module that
 * exports dozens of independent class tokens — opting in on the switch track
 * silently exempted every future icon button added to it, which is precisely
 * how a guard ends up green over a violation.
 *
 * The exemption now has to appear in the *same* class list as the too-small
 * token: the enclosing string literal (the overwhelmingly common case), else the
 * enclosing markup tag, else the enclosing CSS block, else the line.
 */
function enclosingLiteral(source: string, start: number, end: number): string | null {
  for (const quote of ["'", '"', '`'] as const) {
    let open = -1
    for (let index = start - 1; index >= 0; index -= 1) {
      if (source[index] !== quote) continue
      if (source[index - 1] === '\\') continue
      open = index
      break
    }
    if (open === -1) continue
    const close = source.indexOf(quote, end)
    if (close === -1) continue
    const literal = source.slice(open, close + 1)
    // A single/double-quoted JS string cannot span a raw newline, so a match
    // spanning lines means we paired the wrong quote. Fall through instead of
    // widening the exemption to two unrelated literals.
    if (quote !== '`' && literal.includes('\n')) continue
    return literal
  }
  return null
}

export function controlRegion(source: string, start: number, end: number): string {
  const literal = enclosingLiteral(source, start, end)
  if (literal) return literal

  // CSS rule block.
  const openBrace = source.lastIndexOf('{', start)
  const closeBrace = source.indexOf('}', end)
  if (openBrace !== -1 && closeBrace !== -1 && !source.slice(openBrace, start).includes('}')) {
    return source.slice(openBrace, closeBrace + 1)
  }

  // Markup tag.
  const openTag = source.lastIndexOf('<', start)
  const closeTag = source.indexOf('>', end)
  if (openTag !== -1 && closeTag !== -1 && !source.slice(openTag, start).includes('>')) {
    return source.slice(openTag, closeTag + 1)
  }

  // Last resort: the line. Never the file.
  const lineStart = source.lastIndexOf('\n', start) + 1
  const lineEnd = source.indexOf('\n', end)
  return source.slice(lineStart, lineEnd === -1 ? source.length : lineEnd)
}

type Rule = {
  /** Human label for the failure message. */
  label: string
  /** File relative to src/. */
  rel: string
  /** What to do instead. */
  fix: string
} & (
  | {
      /**
       * The control is still visually small by design: every occurrence of
       * `pattern` must carry an opt-in class in its own class list.
       */
      kind: 'opt-in'
      pattern: RegExp
    }
  | {
      /**
       * The control was remediated to a real 32px box, so the rule flipped from
       * "forbid the small token" to "require the sized one". Keeping it in this
       * shape is what preserves the regression guard; deleting it (as a naive
       * stale sweep would) would leave the fix unprotected.
       *
       * `forbidden` keeps the *old* guard alive alongside the new one: the
       * remediated token can still be reintroduced, and each occurrence still
       * has to carry a local opt-in.
       */
      kind: 'required'
      pattern: RegExp
      forbidden?: RegExp[]
    }
)

const RULES: Rule[] = [
  {
    kind: 'opt-in',
    label: 'AdminSwitch track',
    rel: 'features/admin/shared/components/AdminSwitch.vue',
    pattern: /\bh-5\b/,
    fix: 'keep the 20px track but carry `hit-target-32`, or grow the track to `h-8`',
  },
  {
    kind: 'opt-in',
    label: 'admin tab trigger',
    rel: 'features/admin/shared/components/AdminTabsTrigger.vue',
    pattern: /\bh-7\b/,
    fix: 'keep `h-7` inside the h-9 rail and carry `hit-target-32-inline`',
  },
  {
    kind: 'opt-in',
    label: 'chart range segment',
    rel: 'shared/components/RangedLineChartCard.vue',
    pattern: /\bpy-1\b/,
    fix: 'carry `hit-target-32-inline` on the segment button so the row keeps its rhythm',
  },
  {
    kind: 'opt-in',
    label: 'node action button',
    rel: 'features/canvas/presentation/nodes/primitives/NodeActionButton.vue',
    pattern: /\bmin-h-7\b/,
    fix: 'carry `hit-target-32-inline` so node chrome keeps its compact height',
  },
  {
    kind: 'opt-in',
    label: 'task filter pill',
    rel: 'features/admin/monitor/presentation/components/TaskEventsCard.vue',
    pattern: /\bpy-0\.5\b/,
    fix: 'carry `hit-target-32-inline` on the pill so the dense chip rail keeps its density',
  },
  {
    // Was `opt-in` on `!size-7`; the compact icon button is now `!size-8`.
    kind: 'required',
    label: 'compact admin icon button',
    rel: 'features/admin/shared/lib/uiClasses.ts',
    // Pinned to the compact-button token, not a bare `!size-8` occurrence, so the
    // rule still says something if a *different* token in this large module
    // happens to be 8x8.
    pattern: /flex !size-8 shrink-0 items-center justify-center !rounded-lg/,
    // The 28px box the original rule forbade. Still enforced, still opt-in based,
    // so a *new* compact button added to this module cannot quietly ship at 28px
    // just because the existing one is compliant.
    forbidden: [/!size-7\b/],
    fix: 'keep `!size-8` on the compact admin icon button token',
  },
  {
    // Was `opt-in` on `"hidden min-w-0 items-center`; the control grew to `min-h-8`.
    kind: 'required',
    label: 'admin header scroll-to-top',
    rel: 'features/admin/app/AdminHeaderBar.vue',
    pattern: /hidden min-h-8 min-w-0 items-center/,
    fix: 'keep `min-h-8` alongside `min-w-0` on the sticky-header control',
  },
  {
    kind: 'opt-in',
    label: 'retouch curve drag point',
    rel: 'features/canvas/tools/retouch/ui/curves/CurveGraph.vue',
    pattern: /retouch-curves__point/,
    fix: 'carry `hit-target-drag` (11px dot, keyboard-operable; a full 32px overlay would steal neighbouring grabs)',
  },
  {
    kind: 'required',
    label: 'admin settings section navigation',
    rel: 'features/admin/settings/shared/components/AdminSettingsOnThisPage.vue',
    pattern: /<button\b[^>]*\bclass="[^"]*\bmin-h-8\b/,
    fix: 'keep `min-h-8` on the section navigation button',
  },
  {
    // Was `opt-in` on the 11px marker width; the transparent button box grew to
    // the AA floor instead, because both pseudos already paint the marker.
    kind: 'required',
    label: 'retouch histogram marker',
    rel: 'features/canvas/tools/retouch/ui/shared/styles/retouch-histogram-plot.css',
    // Only the transparent button box counts: the 11px painted dot further down
    // the file is the visual, not the target.
    pattern:
      /\.retouch-histogram-plot__marker \{[\s\S]{0,600}?width: var\(--spacing-touch-target-aa\)/,
    fix: 'grow the transparent button box to `var(--spacing-touch-target-aa)`; both pseudos already paint the marker',
  },
]

/**
 * Canvas chrome buttons are plain CSS + plain `class=""` templates (no `cn()`),
 * so they carry the shared hit-area utilities directly. These rules assert the
 * class is present where the visual box is under the floor.
 */
const CANVAS_CHROME_RULES: Array<{
  file: string
  selector: string
  hitClass: string
  fix: string
}> = [
  {
    file: 'features/canvas/presentation/chrome/components/CanvasChrome.vue',
    selector: '.canvas-chrome-status-button',
    hitClass: 'hit-target-32',
    fix: 'the 23px zoom button needs `hit-target-32` (below WCAG AA 2.5.8 without it)',
  },
  {
    file: 'features/canvas/presentation/chrome/components/CanvasChromeBoardHeader.vue',
    selector: '.canvas-chrome-board-icon-button',
    hitClass: 'hit-target-32',
    fix: 'the 29px icon button needs `hit-target-32`',
  },
  {
    file: 'features/canvas/presentation/chrome/components/CanvasChromeBoardHeader.vue',
    selector: '.canvas-chrome-catalog-recovery-action',
    hitClass: 'hit-target-32-inline',
    fix: 'the 22px recovery button needs `hit-target-32-inline` (below WCAG AA 2.5.8 without it)',
  },
]

function matchCount(source: string, pattern: RegExp): number {
  const flags = pattern.flags.includes('g') ? pattern.flags : `${pattern.flags}g`
  return [...source.matchAll(new RegExp(pattern.source, flags))].length
}

/**
 * A rule whose pattern no longer appears at all is dead weight: it can never
 * fail, and it teaches the next reader that the list is complete. This mirrors
 * `checkStyleGovernance.checkStaleAllowances` — the guard reports it instead of
 * spinning forever.
 */
function collectStaleRules(sources: Map<string, string>, missingFiles: string[]): string[] {
  const stale: string[] = []
  for (const rule of RULES) {
    const source = sources.get(rule.rel)
    if (source === undefined) {
      if (missingFiles.includes(rule.rel)) {
        stale.push(`${rule.rel}: rule "${rule.label}" points at a file that no longer exists.`)
      }
      continue
    }
    if (matchCount(source, rule.pattern) > 0) continue
    stale.push(
      `${rule.rel}: stale rule "${rule.label}" — \`${rule.pattern}\` no longer matches. The control was fixed or renamed; update the rule or delete it.`,
    )
  }
  return stale
}

function collectControlViolations(sources: Map<string, string>): string[] {
  const violations: string[] = []

  const flagUnopted = (rule: Rule, pattern: RegExp, source: string): void => {
    const flags = pattern.flags.includes('g') ? pattern.flags : `${pattern.flags}g`
    for (const match of source.matchAll(new RegExp(pattern.source, flags))) {
      const index = match.index ?? 0
      const region = controlRegion(source, index, index + match[0].length)
      // The exemption must live in the control's own class list, not anywhere
      // in the file.
      if (OPT_IN.test(region)) continue
      violations.push(
        `${rule.rel}: ${rule.label} is under the ${FLOOR_PX}px floor without opting in — ${rule.fix}.`,
      )
    }
  }

  for (const rule of RULES) {
    const source = sources.get(rule.rel)
    if (source === undefined) continue

    if (rule.kind === 'opt-in') {
      flagUnopted(rule, rule.pattern, source)
      continue
    }
    for (const pattern of rule.forbidden ?? []) flagUnopted(rule, pattern, source)
  }

  return violations
}

/**
 * Read a rule's target, or record why it could not be read.
 *
 * A missing/renamed control must surface as a stale rule, not as an ENOENT
 * stack trace — the stack trace tells the reader nothing about which rule broke.
 */
function readRuleSource(rel: string, sources: Map<string, string>, missing: string[]): void {
  if (sources.has(rel)) return
  try {
    sources.set(rel, stripComments(readFileSync(join(SRC_ROOT, rel), 'utf8')))
  } catch {
    missing.push(rel)
  }
}

function main(): void {
  const violations: string[] = []

  // 1. The floor token must exist and match FLOOR_PX.
  const styles = readFileSync(STYLE, 'utf8')
  const token = styles.match(/--spacing-touch-target-min:\s*(\d+)px/)
  if (!token) {
    violations.push(
      'app/style.css: `--spacing-touch-target-min` is missing. The touch-target floor must be declared once, here.',
    )
  } else if (Number(token[1]) !== FLOOR_PX) {
    violations.push(
      `app/style.css: \`--spacing-touch-target-min\` is ${token[1]}px but the contract floor is ${FLOOR_PX}px. Update both together.`,
    )
  }

  // 2. The hit-area utility must be imported, or no control can opt in.
  if (!styles.includes("@import '../shared/theme/atoms/touch-target.css'")) {
    violations.push(
      'app/style.css: `touch-target.css` is not imported, so `.hit-target-32` does nothing.',
    )
  }

  const sources = new Map<string, string>()
  const missingFiles: string[] = []
  for (const rule of RULES) readRuleSource(rule.rel, sources, missingFiles)

  // 3. Per-control rules.
  violations.push(...collectControlViolations(sources))

  // 4. Every `required` rule: the remediated control must still carry its fix.
  for (const rule of RULES) {
    if (rule.kind !== 'required') continue
    const source = sources.get(rule.rel)
    if (source === undefined) continue
    if (matchCount(source, rule.pattern) > 0) continue
    violations.push(
      `${rule.rel}: ${rule.label} lost its touch-target fix — ${rule.fix}. This control is now under the ${FLOOR_PX}px floor.`,
    )
  }

  // 5. Stale rules: patterns that can no longer match anything.
  violations.push(...collectStaleRules(sources, missingFiles))

  // 6. Canvas chrome buttons carry the shared hit-area class.
  for (const { file, selector, hitClass, fix } of CANVAS_CHROME_RULES) {
    const source = stripComments(readFileSync(join(SRC_ROOT, file), 'utf8'))
    const className = selector.slice(1)
    const hasSelector = source.includes(className)
    const hasHitClass =
      source.includes(`${className} ${hitClass}`) || source.includes(`${hitClass} ${className}`)
    if (hasSelector && !hasHitClass) {
      violations.push(`${file}: ${fix}.`)
    }
  }

  if (violations.length > 0) {
    console.error(`Touch-target contract failed (floor ${FLOOR_PX}px, WCAG 2.2 AA asks 24px).\n`)
    for (const violation of violations) console.error(`- ${violation}`)
    console.error('')
    process.exit(1)
  }

  console.log(
    `Touch-target contract passed: ${RULES.length + CANVAS_CHROME_RULES.length} controls at or above the ${FLOOR_PX}px floor ` +
      `(${RULES.filter((rule) => rule.kind === 'opt-in').length} opt-in, ${RULES.filter((rule) => rule.kind === 'required').length} remediated, ${CANVAS_CHROME_RULES.length} canvas chrome).`,
  )
}

main()
