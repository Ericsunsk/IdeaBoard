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
 *
 * Two follow-up gates live in this file (same scan, same self-test convention):
 *
 * - `admin-toast-literal-only-in-feedback`: naked `toast.error('...')` literals
 *   must only appear in session `*Feedback*` copy owners; everywhere else copy
 *   goes through a `*Feedback` method or `formatAdminDisplayError`. `format*Error(...)`
 *   wrappers, message constants, error values, and interpolated templates pass.
 *   Comment stripping applies here (unlike the adapter rule): a literal inside a
 *   comment is documentation, not a toast call.
 * - `admin-dialog-error-alert`: `*Dialog.vue` must not carry
 *   `AdminAlert severity="error"` for load/save failures; those go through toast
 *   plus `*Feedback`. Field-level validation stays inline and is exempt when the
 *   `v-if` references form errors (`form.errors` / `errors.value` /
 *   `firstValidationError` / `firstFormError` / `validation`).
 *   `AdminSettingsDialogFrame.vue` (ends with `Frame.vue`) is out of scope: it
 *   is the intentional retry carrier (`error` prop + retry button + save
 *   disable), not dialog content.
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
const RULE_TOAST_COPY = 'admin-toast-literal-only-in-feedback'
const RULE_DIALOG_ALERT = 'admin-dialog-error-alert'

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

function isFeedbackCopyOwner(rel: string): boolean {
  return rel.includes('/session/') && rel.toLowerCase().includes('feedback')
}

function stripCommentsForToastCopy(content: string): string {
  return content
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/(^|\s)\/\/.*$/gm, '$1')
}

const NAKED_TOAST_ERROR_PATTERN =
  /\btoast\.error\(\s*('(?:[^'\\\n]|\\.)*'|"(?:[^"\\\n]|\\.)*"|`(?:[^`\\]|\\.)*?`)\s*\)/g

function findNakedToastErrors(source: string): string[] {
  const hits: string[] = []
  for (const match of stripCommentsForToastCopy(source).matchAll(NAKED_TOAST_ERROR_PATTERN)) {
    const arg = match[1] ?? ''
    if (arg.startsWith('`') && arg.includes('${')) continue
    hits.push((match[0] ?? '').trim().slice(0, 120))
  }
  return hits
}

/**
 * Pure validator: naked `toast.error('literal')` outside session feedback copy
 * owners. Wrappers (`format*Error(...)`), message constants, error values, and
 * interpolated templates pass; only a plain literal first argument fails.
 */
function validateToastCopySources(sources: Record<string, string>): FeedbackBoundaryViolation[] {
  const violations: FeedbackBoundaryViolation[] = []
  for (const [rel, source] of Object.entries(sources)) {
    if (isSpecFile(rel) || isFeedbackCopyOwner(rel)) continue
    for (const hit of findNakedToastErrors(source)) {
      violations.push({
        rule: RULE_TOAST_COPY,
        file: rel,
        message: `naked toast.error literal outside feedback copy (${hit}); route copy through *Feedback or formatAdminDisplayError`,
      })
    }
  }
  return violations
}

function isAdminDialogFile(rel: string): boolean {
  const base = rel.split('/').pop() ?? ''
  return base.endsWith('Dialog.vue')
}

const FIELD_LEVEL_ERROR_HINT =
  /(form\.errors|errors\.value|firstValidationError|firstFormError|validation)/i
const ADMIN_ALERT_TAG_PATTERN = /<AdminAlert\b[^>]*>/g
const ERROR_SEVERITY_PATTERN = /\bseverity\s*=\s*('error'|"error")/
const V_IF_PATTERN = /\bv-if\s*=\s*("[^"]*"|'[^']*')/

function findDialogErrorAlerts(source: string): string[] {
  const hits: string[] = []
  for (const match of source.matchAll(ADMIN_ALERT_TAG_PATTERN)) {
    const tag = match[0] ?? ''
    if (!ERROR_SEVERITY_PATTERN.test(tag)) continue
    const condition = tag.match(V_IF_PATTERN)?.[1] ?? ''
    if (condition && FIELD_LEVEL_ERROR_HINT.test(condition)) continue
    hits.push(tag.trim().slice(0, 120))
  }
  return hits
}

/**
 * Pure validator: `*Dialog.vue` must not carry `AdminAlert severity="error"`
 * for load/save failures. Field-level validation stays exempt via the `v-if`.
 */
function validateDialogAlertSources(sources: Record<string, string>): FeedbackBoundaryViolation[] {
  const violations: FeedbackBoundaryViolation[] = []
  for (const [rel, source] of Object.entries(sources)) {
    if (isSpecFile(rel) || !isAdminDialogFile(rel)) continue
    for (const hit of findDialogErrorAlerts(source)) {
      violations.push({
        rule: RULE_DIALOG_ALERT,
        file: rel,
        message: `AdminAlert severity="error" inside dialog (${hit}); keep load/save failures in toast + *Feedback, field-level validation only`,
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

type SelfTestCase = [string, FeedbackBoundaryViolation[], boolean]

function verifySelfTestCases(label: string, rule: string, cases: SelfTestCase[]): void {
  let failed = 0
  for (const [name, violations, expectPass] of cases) {
    const passed = violations.length === 0
    const hasRule = violations.some((violation) => violation.rule === rule)
    if (expectPass ? passed : !passed && hasRule) continue
    failed += 1
    console.error(
      `[${rule}-self-test] ${name}: expected ${expectPass ? 'pass' : `rule ${rule}`}, got ${JSON.stringify(violations)}`,
    )
  }
  if (failed > 0) {
    console.error(`\n${label} self-test failed: ${failed} case(s).`)
    process.exit(1)
  }
  console.log(`${label} self-test passed: ${cases.length} case(s).`)
}

function runSelfTest(): void {
  const adapterSource = readFileSync(join(SRC_ROOT, ADAPTER_REL), 'utf8')
  const cleanConsumer = `import { useAdminToast } from '../../shared/session/feedback/toast'\nconst toast = useAdminToast()\ntoast.success('ok')`
  const cases: SelfTestCase[] = [
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

  verifySelfTestCases('Admin feedback boundary', RULE, cases)
}

function runToastCopySelfTest(): void {
  const feedbackOwner = 'features/admin/monitor/session/useAdminMonitorFeedback.ts'
  const cases: SelfTestCase[] = [
    [
      'feedback copy owner keeps naked literals',
      validateToastCopySources({
        [feedbackOwner]: `copyFailed: () => toast.error('复制失败')`,
      }),
      true,
    ],
    [
      'shared session feedback file stays exempt',
      validateToastCopySources({
        'features/admin/settings/session/storage/useStorageFeedback.ts': `toast.error('未找到可编辑的存储配置')`,
      }),
      true,
    ],
    [
      'naked single-quote literal in page is rejected',
      validateToastCopySources({
        'features/admin/credits/ui/health/AdminCreditHealthPage.vue': `toast.error('加载失败')`,
      }),
      false,
    ],
    [
      'naked double-quote literal in dialog is rejected',
      validateToastCopySources({
        'features/admin/x/AdminThingDialog.vue': `toast.error("加载失败")`,
      }),
      false,
    ],
    [
      'plain backtick literal without interpolation is rejected',
      validateToastCopySources({
        'features/admin/x/AdminThingPage.vue': 'toast.error(`加载失败`)',
      }),
      false,
    ],
    [
      'format-wrapped fallback passes',
      validateToastCopySources({
        'features/admin/x/AdminThingPage.vue': `toast.error(formatAdminDisplayError(error, '加载失败'))`,
      }),
      true,
    ],
    [
      'message constant passes',
      validateToastCopySources({
        'features/admin/x/AdminThingDialog.vue': 'toast.error(MESSAGES.loadFailed)',
      }),
      true,
    ],
    [
      'interpolated template passes',
      validateToastCopySources({
        'features/admin/x/AdminThingPage.vue': 'toast.error(`${prefix}失败`)',
      }),
      true,
    ],
    [
      'error value passthrough passes',
      validateToastCopySources({
        'features/admin/x/AdminThingPage.vue': 'toast.error(poolCatalogError.value)',
      }),
      true,
    ],
    [
      'commented literal is documentation, not a call',
      validateToastCopySources({
        'features/admin/x/AdminThingPage.vue': `// toast.error('加载失败')\ntoast.success('ok')`,
      }),
      true,
    ],
    [
      'spec file stays exempt',
      validateToastCopySources({
        'features/admin/x/AdminThingPage.component.spec.ts': `toast.error('加载失败')`,
      }),
      true,
    ],
  ]

  verifySelfTestCases('Admin toast copy', RULE_TOAST_COPY, cases)
}

function runDialogAlertSelfTest(): void {
  const cases: SelfTestCase[] = [
    [
      'field-level form.errors stays exempt',
      validateDialogAlertSources({
        'features/admin/models/ui/dialog/AdminGatewayConfigDialog.vue':
          '<AdminAlert v-if="form.errors.value.base_url" severity="error">',
      }),
      true,
    ],
    [
      'field-level firstValidationError stays exempt',
      validateDialogAlertSources({
        'features/admin/x/AdminMailDialog.vue':
          '<AdminAlert v-if="firstValidationError" severity="error">',
      }),
      true,
    ],
    [
      'dialog load carrier is rejected',
      validateDialogAlertSources({
        'features/admin/x/AdminThingDialog.vue': '<AdminAlert v-if="error" severity="error">',
      }),
      false,
    ],
    [
      'always-on dialog error alert is rejected',
      validateDialogAlertSources({
        'features/admin/x/AdminThingDialog.vue': '<AdminAlert severity="error">',
      }),
      false,
    ],
    [
      'dialog secondary background state passes',
      validateDialogAlertSources({
        'features/admin/x/AdminThingDialog.vue':
          '<AdminAlert v-if="backgroundError" severity="secondary">',
      }),
      true,
    ],
    [
      'dialog warn hint passes',
      validateDialogAlertSources({
        'features/admin/x/AdminThingDialog.vue': '<AdminAlert v-if="stale" severity="warn">',
      }),
      true,
    ],
    [
      'settings frame retry carrier is out of scope',
      validateDialogAlertSources({
        'features/admin/shared/ui/shell/AdminSettingsDialogFrame.vue':
          '<AdminAlert v-if="error" severity="error">',
      }),
      true,
    ],
    [
      'card file is out of scope',
      validateDialogAlertSources({
        'features/admin/settings/ui/cards/mail/AdminMailConfigCard.vue':
          '<AdminAlert v-if="error" severity="error">',
      }),
      true,
    ],
    [
      'spec file stays exempt',
      validateDialogAlertSources({
        'features/admin/x/AdminThingDialog.component.spec.ts':
          '<AdminAlert v-if="error" severity="error">',
      }),
      true,
    ],
  ]

  verifySelfTestCases('Admin dialog alert', RULE_DIALOG_ALERT, cases)
}

runSelfTest()
runToastCopySelfTest()
runDialogAlertSelfTest()

/* ------------------------------------------------------------------ */

const sources: Record<string, string> = {}
for (const file of listAdminSources(ADMIN_ROOT)) {
  sources[relOf(file)] = readFileSync(file, 'utf8')
}
const violations = [
  ...validateAdminFeedbackSources(sources),
  ...validateToastCopySources(sources),
  ...validateDialogAlertSources(sources),
]

if (violations.length > 0) {
  console.error('Admin feedback boundary failed.\n')
  for (const violation of violations) {
    console.error(`- [${violation.rule}] ${violation.file}: ${violation.message}`)
  }
  console.error('')
  process.exit(1)
}

console.log(
  `Admin feedback boundary passed: ${Object.keys(sources).length} Admin source(s) keep direct app toast access inside the shared transport adapter.`,
)
console.log(
  `Admin toast copy gate passed (${RULE_TOAST_COPY}): naked toast.error literals stay inside session feedback copy owners.`,
)
console.log(
  `Admin dialog alert gate passed (${RULE_DIALOG_ALERT}): no error-level AdminAlert carries dialog load/save failures.`,
)
