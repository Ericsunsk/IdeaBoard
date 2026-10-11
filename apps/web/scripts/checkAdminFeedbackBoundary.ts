import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'

/**
 * Admin feedback boundary guard (`admin-feedback-adapter-only`).
 *
 * The Admin surface used to carry two feedback exits: the shared toast adapter
 * (`features/admin/shared/session/feedback/toast.ts`, the only module allowed to touch the
 * app-level `useAppToast`) and a retired global feedback tree
 * (`useAdminFeedback` / `AdminFeedback`). The tree is gone; Admin toast
 * feedback goes through `useAdminToast` / `formatAdminDisplayError` from the
 * shared adapter.
 *
 * This script replaces the source-string assertions retired from
 * `src/features/admin/adminFeedbackBoundary.contract.spec.ts`:
 *
 *   - source-4c813e0f561cdf70-1 (scan sees the shared toast adapter)
 *   - source-8fd0b3771990469b-1 (direct app toast access stays in the adapter)
 *   - source-ffcb56af525ede11-1 (retired `useAdminFeedback`/`AdminFeedback`
 *     tree never comes back)
 *
 * Replacement ledger id: `eslint/admin-feedback-adapter-only` ("Reject direct
 * useAppToast and retired AdminFeedback outside shared toast adapter; positive
 * adapter and consumer plus negative callers"). The fixtures below are the
 * positive (adapter plus a real consumer pass) and negative (missing consumer
 * or direct callers fail) cases; the eslint wiring itself is integrator
 * follow-up and intentionally not added here.
 *
 * Matching semantics are deliberately identical to the retired spec (raw source
 * match, no comment stripping): a `useAppToast` mention in a comment is still a
 * violation, so prose cannot smuggle the bypass back in. `*.spec.ts` files stay
 * exempt: specs mock the app toast (`vi.mock('.../useAppToast')`) to pin the
 * boundary without touching it.
 */

const SRC_ROOT = join(import.meta.dirname, '..', 'src')
const ADMIN_ROOT = join(SRC_ROOT, 'features/admin')
const ADAPTER_REL = 'features/admin/shared/session/feedback/toast.ts'
const RULE = 'admin-feedback-adapter-only'

type FeedbackBoundaryViolation = {
  rule: string
  file: string
  message: string
}

const DIRECT_APP_TOAST_ACCESS =
  /(?:import\s*\{[^}]*\buseAppToast\b[^}]*\}\s*from)|(?:\buseAppToast\s*\()/
const RETIRED_FEEDBACK_TREE = /(?:useAdminFeedback|AdminFeedback\b)/
const ADMIN_TOAST_CONSUMER = /\buseAdminToast\s*\(/

function isSpecFile(rel: string): boolean {
  return rel.endsWith('.spec.ts')
}

function isAdapter(rel: string): boolean {
  return rel === ADAPTER_REL
}

/**
 * Pure validator over repo-relative path -> source maps. The self-test feeds
 * fixtures here (red-then-green proof); the main scan feeds the real tree.
 */
function validateAdminFeedbackSources(
  sources: Record<string, string>,
): FeedbackBoundaryViolation[] {
  const violations: FeedbackBoundaryViolation[] = []
  if (!Object.hasOwn(sources, ADAPTER_REL)) {
    violations.push({
      rule: RULE,
      file: ADAPTER_REL,
      message: 'shared toast adapter missing from the Admin source tree',
    })
  }
  const hasConsumer = Object.entries(sources).some(
    ([rel, source]) => !isAdapter(rel) && !isSpecFile(rel) && ADMIN_TOAST_CONSUMER.test(source),
  )
  if (Object.hasOwn(sources, ADAPTER_REL) && !hasConsumer) {
    violations.push({
      rule: RULE,
      file: ADAPTER_REL,
      message: 'shared toast adapter has no Admin consumer using useAdminToast',
    })
  }
  for (const [rel, source] of Object.entries(sources)) {
    if (isAdapter(rel) || isSpecFile(rel)) continue
    if (DIRECT_APP_TOAST_ACCESS.test(source)) {
      violations.push({
        rule: RULE,
        file: rel,
        message:
          'direct useAppToast access; use useAdminToast from shared/session/feedback/toast.ts instead',
      })
    }
    if (RETIRED_FEEDBACK_TREE.test(source)) {
      violations.push({
        rule: RULE,
        file: rel,
        message:
          'retired useAdminFeedback/AdminFeedback tree; use the shared toast adapter instead',
      })
    }
  }
  return violations
}

function listAdminSources(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    if (entry === 'node_modules' || entry === 'generated-query') return []
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) return listAdminSources(full)
    return entry.endsWith('.ts') || entry.endsWith('.vue') ? [full] : []
  })
}

function relOf(file: string): string {
  return relative(SRC_ROOT, file).split(sep).join('/')
}

function runSelfTest(): void {
  const adapterSource = readFileSync(join(SRC_ROOT, ADAPTER_REL), 'utf8')
  const cleanConsumer = `import { useAdminToast } from '../../shared/session/feedback/toast'\nconst toast = useAdminToast()\ntoast.success('ok')`
  const cases: Array<[string, FeedbackBoundaryViolation[], boolean]> = [
    [
      'vertical consumer through useAdminToast passes',
      validateAdminFeedbackSources({
        [ADAPTER_REL]: adapterSource,
        'features/admin/payments/example.ts': cleanConsumer,
      }),
      true,
    ],
    [
      'adapter without a useAdminToast consumer is rejected',
      validateAdminFeedbackSources({ [ADAPTER_REL]: adapterSource }),
      false,
    ],
    [
      'spec files mocking the app toast stay exempt',
      validateAdminFeedbackSources({
        [ADAPTER_REL]: adapterSource,
        'features/admin/payments/example.ts': cleanConsumer,
        'features/admin/payments/example.component.spec.ts': `vi.mock('../../../../core/composables/useAppToast')\nuseAppToast()\nuseAdminFeedback`,
      }),
      true,
    ],
    [
      'direct useAppToast import outside the adapter is rejected',
      validateAdminFeedbackSources({
        [ADAPTER_REL]: adapterSource,
        'features/admin/prompts/offender.ts': `import { useAppToast } from '../../../core/composables/useAppToast'\nuseAdminToast()`,
      }),
      false,
    ],
    [
      'direct useAppToast() call outside the adapter is rejected',
      validateAdminFeedbackSources({
        [ADAPTER_REL]: adapterSource,
        'features/admin/settings/offender.vue': `<script setup>const toast = useAppToast()</script>`,
      }),
      false,
    ],
    [
      'retired useAdminFeedback reference is rejected',
      validateAdminFeedbackSources({
        [ADAPTER_REL]: adapterSource,
        'features/admin/payments/offender.ts': `import { useAdminFeedback } from './feedback'\nuseAdminFeedback()`,
      }),
      false,
    ],
    [
      'retired AdminFeedback word is rejected',
      validateAdminFeedbackSources({
        [ADAPTER_REL]: adapterSource,
        'features/admin/settings/offender.ts': `const boundary: AdminFeedback = createBoundary()`,
      }),
      false,
    ],
    [
      'missing shared toast adapter is rejected',
      validateAdminFeedbackSources({
        'features/admin/payments/example.ts': cleanConsumer,
      }),
      false,
    ],
  ]

  let failed = 0
  for (const [name, violations, expectPass] of cases) {
    const passed = violations.length === 0
    const hasRule = violations.some((violation) => violation.rule === RULE)
    const ok = expectPass ? passed : !passed && hasRule
    if (!ok) {
      failed += 1
      console.error(
        `[admin-feedback-boundary-self-test] ${name}: expected ${expectPass ? 'pass' : `rule ${RULE}`}, got ${JSON.stringify(violations)}`,
      )
    }
  }
  if (failed > 0) {
    console.error(`\nAdmin feedback boundary self-test failed: ${failed} case(s).`)
    process.exit(1)
  }
  console.log(`Admin feedback boundary self-test passed: ${cases.length} case(s).`)
}

runSelfTest()

/* ------------------------------------------------------------------ */

const sources: Record<string, string> = {}
for (const file of listAdminSources(ADMIN_ROOT)) {
  sources[relOf(file)] = readFileSync(file, 'utf8')
}
const violations = validateAdminFeedbackSources(sources)

if (violations.length > 0) {
  console.error('Admin feedback boundary failed.\n')
  for (const violation of violations) {
    console.error(`- ${violation.file}: ${violation.message}`)
  }
  console.error('')
  process.exit(1)
}

console.log(
  `Admin feedback boundary passed: ${Object.keys(sources).length} Admin source(s) keep direct app toast access inside the shared transport adapter.`,
)
