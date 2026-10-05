import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'

/**
 * Guard: Card elevation must be requested, never cancelled.
 *
 * `Card.vue` used to bake `shadow` into its base class string. Flat surfaces
 * then had to pass `shadow-none` (or a token whose value happened to be `none`)
 * to opt out, which only worked because `cn()` resolves conflicts by class
 * order. In practice the opt-out was applied inconsistently: on
 * `/admin/statistics` two of the four chart cards rendered with a shadow
 * because the Admin range-chart adapter simply never passed the override.
 *
 * This check fails if any Card surface re-introduces the override idiom.
 */

const SRC_ROOT = join(import.meta.dirname, '..', 'src')

/** Surfaces that legitimately own a `none` shadow as an explicit contract. */
const ALLOWED_NONE_SHADOW_TOKENS = ['--shadow-overlay-menu', '--shadow-page-chrome'] as const

/** Files allowed to mention `shadow-none` (documenting the failure mode is fine). */
const SHADOW_NONE_ALLOWED_FILES = ['components/ui/card/Card.vue'] as const

const CARD_SURFACE_FILES = [
  'components/ui/card/Card.vue',
  'shared/components/ChartCard.vue',
  'shared/components/RangedLineChartCard.vue',
  'features/admin/shared/components/AdminChartCard.vue',
  'features/admin/shared/components/AdminRangedLineChartCard.vue',
  'features/workspace/pages/WorkspaceShareJoinPage.vue',
]

function listVueFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) return listVueFiles(full)
    return entry.endsWith('.vue') ? [full] : []
  })
}

function main(): void {
  const violations: string[] = []

  // 1. No surface may cancel a Card shadow with a class.
  for (const file of listVueFiles(SRC_ROOT)) {
    if (file.includes('.spec.')) continue
    const rel = relative(SRC_ROOT, file).split(sep).join('/')
    if (!/Card\.vue$/.test(file) && !/ChartCard\.vue$/.test(file)) continue
    if ((SHADOW_NONE_ALLOWED_FILES as readonly string[]).includes(rel)) continue

    const source = readFileSync(file, 'utf8')
    if (/\bshadow-none\b/.test(source)) {
      violations.push(
        `${rel}: uses \`shadow-none\` to cancel a Card shadow. Pass \`elevated="false"\` instead so the level is an explicit contract.`,
      )
    }
  }

  // 2. The flat Card surfaces must all declare elevation explicitly.
  for (const rel of CARD_SURFACE_FILES) {
    if (rel.includes('Card.vue') && rel.endsWith('ui/card/Card.vue')) continue
    const source = readFileSync(join(SRC_ROOT, rel), 'utf8')
    if (/:elevated=/.test(source)) continue
    // Surfaces that intentionally keep the default elevated look need no
    // declaration; only flag ones that were flat and lost their opt-out.
    if (/\bshadow-none\b/.test(source)) {
      violations.push(`${rel}: flat Card surface lost its \`elevated\` declaration.`)
    }
  }

  // 3. No shadow token may resolve to `none` outside the reviewed allowlist.
  const styleSheet = readFileSync(join(SRC_ROOT, 'app/style.css'), 'utf8')
  const noneShadowTokens = [...styleSheet.matchAll(/--([a-z0-9-]*shadow[a-z0-9-]*):\s*none\s*;/g)]
    .map((match) => `--${match[1]}`)
    .filter((token) => !(ALLOWED_NONE_SHADOW_TOKENS as readonly string[]).includes(token))

  for (const token of noneShadowTokens) {
    violations.push(
      `app/style.css: \`${token}\` resolves to \`none\`. A shadow token that is \`none\` only works as an override; use the \`elevated\` prop or give the token a real value.`,
    )
  }

  if (violations.length > 0) {
    console.error('Card elevation contract failed.\n')
    for (const violation of violations) console.error(`- ${violation}`)
    console.error('')
    process.exit(1)
  }

  console.log(
    `Card elevation contract passed: ${CARD_SURFACE_FILES.length} surfaces, no shadow overrides.`,
  )
}

main()
