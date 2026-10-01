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
 *   node action buttons             ~x26     4 instances   canvas node overlay
 *   admin pager / chart collapse    28x28    5 instances   toolbar
 *   admin header scroll-to-top      ~x24     6 instances   sticky page title
 *   canvas board icon buttons       29x29    4 instances   canvas header
 *
 * The floor is `--spacing-ib-touch-target-min` (32px). Controls that are
 * visually small by design carry `.ib-hit-target-32` / `.ib-hit-target-32-inline`
 * from `shared/theme/touch-target.css`, which grows the hit area past the visual
 * box without inflating the design. Controls that can afford it set a real
 * `min-h-8` / `!size-8`.
 *
 * This check fails if a control drops below the floor without opting in through
 * one of those two routes.
 */

/** The floor, in px. Must track `--spacing-ib-touch-target-min` in style.css. */
const FLOOR_PX = 32

const SRC_ROOT = join(import.meta.dirname, '..', 'src')
const STYLE = join(SRC_ROOT, 'app/style.css')

const OPT_IN = /ib-hit-target-32(?:-inline)?\b|ib-hit-target-drag\b/

/**
 * Strip comments before testing. Several of these files *document* the hit-area
 * utility in a comment right next to the class, and a naive substring test reads
 * that prose as the control opting in — which silently disarmed the guard.
 */
function stripComments(source: string): string {
  return source.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')
}

type Rule = {
  /** Human label for the failure message. */
  label: string
  /** File relative to src/. */
  rel: string
  /**
   * Matches a class string that is too small on its own. Callers that also
   * match OPT_IN are exempt.
   */
  pattern: RegExp
  /** What to do instead. */
  fix: string
}

const RULES: Rule[] = [
  {
    label: 'AdminSwitch track',
    rel: 'features/admin/shared/components/AdminSwitch.vue',
    pattern: /\bh-5\b/,
    fix: 'keep the 20px track but carry `ib-hit-target-32`, or grow the track to `h-8`',
  },
  {
    label: 'admin tab trigger',
    rel: 'features/admin/shared/components/AdminTabsTrigger.vue',
    pattern: /\bpy-1\b(?![^\n]*\bmin-h-8\b)/,
    fix: 'declare `min-h-8` (the AdminTabsList rail is h-9 and has room)',
  },
  {
    label: 'chart range segment',
    rel: 'shared/components/RangedLineChartCard.vue',
    pattern: /\bpy-1\b/,
    fix: 'carry `ib-hit-target-32-inline` so the segment row keeps its rhythm',
  },
  {
    label: 'node action button',
    rel: 'features/canvas/presentation/nodes/primitives/NodeActionButton.vue',
    pattern: /\bmin-h-7\b/,
    fix: 'carry `ib-hit-target-32-inline` so node chrome keeps its compact height',
  },
  {
    label: 'task filter pill',
    rel: 'features/admin/monitor/presentation/components/TaskEventsCard.vue',
    pattern: /\bpy-0\.5\b/,
    fix: 'carry `ib-hit-target-32-inline` so the dense chip rail keeps its density',
  },
  {
    label: 'compact admin icon button',
    rel: 'features/admin/shared/lib/uiClasses.ts',
    pattern: /!size-7\b/,
    fix: 'use `!size-8`',
  },
  {
    label: 'admin header scroll-to-top',
    rel: 'features/admin/app/AdminHeaderBar.vue',
    pattern: /"hidden min-w-0 items-center/,
    fix: 'declare `min-h-8` alongside `min-w-0`',
  },
  {
    label: 'retouch curve drag point',
    rel: 'features/canvas/tools/retouch/components/CurveGraph.vue',
    pattern: /retouch-curves__point/,
    fix: 'carry `ib-hit-target-drag` (11px dot, keyboard-operable; a full 32px overlay would steal neighbouring grabs)',
  },
  {
    label: 'retouch histogram marker',
    rel: 'features/canvas/tools/retouch/components/styles/retouch-histogram-plot.css',
    // Only the transparent button box counts: the 11px painted dot further down
    // the file is the visual, not the target.
    pattern: /\.retouch-histogram-plot__marker \{\s*[^}]*?width: var\(--retouch-histogram-marker-size/,
    fix: 'grow the transparent button box to `var(--spacing-ib-touch-target-aa)`; both pseudos already paint the marker',
  },
]

/**
 * Canvas chrome buttons are plain CSS + plain `class=""` templates (no `cn()`),
 * so they carry the shared hit-area utilities directly. These rules assert the
 * class is present where the visual box is under the floor.
 */
const CANVAS_CHROME_RULES: Array<{ file: string; selector: string; hitClass: string; fix: string }> = [
  {
    file: 'features/canvas/presentation/chrome/components/CanvasChrome.vue',
    selector: '.canvas-chrome-status-button',
    hitClass: 'ib-hit-target-32',
    fix: 'the 23px zoom button needs `ib-hit-target-32` (below WCAG AA 2.5.8 without it)',
  },
  {
    file: 'features/canvas/presentation/chrome/components/CanvasChromeBoardHeader.vue',
    selector: '.canvas-chrome-board-icon-button',
    hitClass: 'ib-hit-target-32',
    fix: 'the 29px icon button needs `ib-hit-target-32`',
  },
  {
    file: 'features/canvas/presentation/chrome/components/CanvasChromeBoardHeader.vue',
    selector: '.canvas-chrome-catalog-recovery-action',
    hitClass: 'ib-hit-target-32-inline',
    fix: 'the 22px recovery button needs `ib-hit-target-32-inline` (below WCAG AA 2.5.8 without it)',
  },
]

function main(): void {
  const violations: string[] = []

  // 1. The floor token must exist and match FLOOR_PX.
  const styles = readFileSync(STYLE, 'utf8')
  const token = styles.match(/--spacing-ib-touch-target-min:\s*(\d+)px/)
  if (!token) {
    violations.push(
      'app/style.css: `--spacing-ib-touch-target-min` is missing. The touch-target floor must be declared once, here.',
    )
  } else if (Number(token[1]) !== FLOOR_PX) {
    violations.push(
      `app/style.css: \`--spacing-ib-touch-target-min\` is ${token[1]}px but the contract floor is ${FLOOR_PX}px. Update both together.`,
    )
  }

  // 2. The hit-area utility must be imported, or no control can opt in.
  if (!styles.includes("@import '../shared/theme/touch-target.css'")) {
    violations.push(
      "app/style.css: `touch-target.css` is not imported, so `.ib-hit-target-32` does nothing.",
    )
  }

  // 3. Per-control rules.
  for (const rule of RULES) {
    const source = stripComments(readFileSync(join(SRC_ROOT, rule.rel), 'utf8'))
    if (!rule.pattern.test(source)) continue
    if (OPT_IN.test(source)) continue
    violations.push(`${rule.rel}: ${rule.label} is under the ${FLOOR_PX}px floor without opting in — ${rule.fix}.`)
  }

  // 4. Canvas chrome buttons carry the shared hit-area class.
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
    `Touch-target contract passed: ${RULES.length + CANVAS_CHROME_RULES.length} controls at or above the ${FLOOR_PX}px floor.`,
  )
}

main()
