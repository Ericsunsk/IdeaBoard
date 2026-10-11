import { existsSync } from 'node:fs'
import { readdir, readFile, stat } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import ts from 'typescript'

type Exception = {
  reason: string
}

type ExceptionMap<T extends Exception> = Readonly<Record<string, readonly T[]>>

type ComponentHarnessRule =
  'raw-create-app' | 'raw-mount-call' | 'raw-unmount-call' | 'document-div-host' | 'vue-test-utils'

type ComponentHarnessException = Exception & {
  rule: ComponentHarnessRule
}

type GeneratedQueryMockException = Exception & {
  module: string
}

type ComponentClassAssertionRule =
  'to-have-class' | 'class-list' | 'class-name' | 'vue-test-utils-classes' | 'class-attribute'

type Violation = {
  file: string
  rule: string
  message: string
  details?: readonly string[]
}

type SpecFile = {
  absolutePath: string
  relativePath: string
  sourcePath: string
}

type ComponentHarnessPattern = {
  rule: ComponentHarnessRule
  pattern: RegExp
  message: string
}

type ComponentClassAssertionPattern = {
  rule: ComponentClassAssertionRule
  pattern: RegExp
  message: string
}

const allowedSpecSuffixes = [
  '.unit.spec.ts',
  '.component.spec.ts',
  '.contract.spec.ts',
  '.browser.spec.ts',
] as const

/**
 * P0 test conventions (plan §3.7, warn-only): new factories use
 * `buildX(overrides?)`. Exported `makeX` seams below are grandfathered with
 * their file and public symbol; spec-local (non-exported) `makeX` helpers stay out of
 * scope. New exported `makeX` factories print a warning and never fail the
 * lane: P0 adds no error gates (the canvas Query ban in eslint.config.js is
 * the sole new error rule).
 */
const grandfatheredMakeFactories = new Set([
  'src/test/factories/admin.ts:makeAdminUserCommandTarget',
  'src/test/factories/admin.ts:makeAdminUserListItem',
  'src/test/factories/admin.ts:makeControlUserPayload',
  'src/test/factories/admin.ts:makePaymentOrderSummary',
  'src/test/factories/admin.ts:makePaymentOrderDetail',
  'src/test/factories/user.ts:makeSessionUser',
  'src/features/board/testing/factories.ts:makePendingImageNodeSnapshot',
  'src/features/board/testing/factories.ts:makeBoardSnapshot',
  'src/features/board/testing/factories.ts:makeBoardState',
  'src/features/admin/credits/session/health/testing.ts:makeCreditHealth',
  'src/features/admin/credits/session/health/testing.ts:makeReconciliationRun',
  'src/features/admin/credits/session/health/testing.ts:makeCreditFinding',
  'src/features/admin/credits/session/pricing/testing.ts:makeCatalogItems',
  'src/features/canvas/application/testing/boardFixtures.ts:makeBoardSnapshot',
  'src/features/canvas/application/testing/boardFixtures.ts:makeBoardState',
  'src/features/canvas/tools/retouch/core/ids.ts:makeRetouchLayerId',
  'src/features/canvas/tools/retouch/core/ids.ts:makeCurvePointId',
  'src/features/canvas/tools/retouch/core/ids.ts:makeCurvePointFallbackId',
  'src/features/canvas/tools/retouch/core/layers/layerStack.ts:makeRetouchLayerId',
])

function collectNewMakeFactories(file: string, source: string): string[] {
  if (file.endsWith('.spec.ts') || file.includes('/generated-query/')) return []
  const parsed = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true)
  const exported = new Set<string>()
  const add = (name: string) => {
    if (/^make[A-Z][A-Za-z0-9_]*$/.test(name)) exported.add(name)
  }
  for (const statement of parsed.statements) {
    if (ts.isExportDeclaration(statement)) {
      if (statement.isTypeOnly || !statement.exportClause) continue
      if (ts.isNamedExports(statement.exportClause)) {
        for (const element of statement.exportClause.elements) {
          if (!element.isTypeOnly) add(element.name.text)
        }
      }
      continue
    }
    if (!ts.canHaveModifiers(statement)) continue
    if (
      !ts.getModifiers(statement)?.some((modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword)
    )
      continue
    if (ts.isFunctionDeclaration(statement) && statement.name) add(statement.name.text)
    if (ts.isVariableStatement(statement)) {
      for (const declaration of statement.declarationList.declarations) {
        if (ts.isIdentifier(declaration.name)) add(declaration.name.text)
      }
    }
  }
  return [...exported].filter((name) => !grandfatheredMakeFactories.has(`${file}:${name}`)).sort()
}

function checkMakeFactoryFixtures(): void {
  const cases: Array<[string, string, string, string[]]> = [
    [
      'legacy-and-new',
      'src/test/factories/user.ts',
      'export function makeSessionUser() {}\nexport const makeAnotherUser = () => ({})',
      ['makeAnotherUser'],
    ],
    [
      'multiple-declarations',
      'src/test/factories/new.ts',
      'export const makeFirst = () => ({}), makeSecond = () => ({})',
      ['makeFirst', 'makeSecond'],
    ],
    [
      'async-default',
      'src/test/factories/new.ts',
      'export async function makeAsync() {}\nexport default function makeDefault() {}',
      ['makeAsync', 'makeDefault'],
    ],
    [
      'alias-and-reexport',
      'src/test/factories/new.ts',
      'function local() {}\nexport { local as makeAlias }; export { buildUser as makeReexport } from "./user"',
      ['makeAlias', 'makeReexport'],
    ],
    [
      'legacy-reexport-and-new-alias',
      'src/features/canvas/tools/retouch/core/layers/layerStack.ts',
      'export { makeRetouchLayerId, makeRetouchLayerId as makeNewId } from "../ids"',
      ['makeNewId'],
    ],
    [
      'local-and-type-only',
      'src/test/factories/new.ts',
      'function makeLocal() {}\nexport type { makeType } from "./types"; export { type makeOtherType } from "./types"',
      [],
    ],
    ['spec-exclusion', 'src/test/factories/new.unit.spec.ts', 'export function makeSpec() {}', []],
    [
      'generated-exclusion',
      'src/api/generated-query/model.ts',
      'export const makeGenerated = () => ({})',
      [],
    ],
    ['build-convention', 'src/test/factories/new.ts', 'export function buildUser() {}', []],
  ]
  for (const [name, file, source, expected] of cases) {
    const actual = collectNewMakeFactories(file, source)
    if (JSON.stringify(actual) !== JSON.stringify(expected)) {
      throw new Error(
        `Factory naming fixture ${name}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`,
      )
    }
  }
  console.log(`Factory naming fixtures passed (${cases.length} cases).`)
}

const componentHarnessExceptions: ExceptionMap<ComponentHarnessException> = {
  'src/features/canvas/presentation/nodes/home/ImageActionMenu.component.spec.ts': [
    {
      rule: 'raw-unmount-call',
      reason:
        'Unmounts one Testing Library render result between multiple render() calls; the component harness still goes through src/test/render.',
    },
  ],
  'src/features/admin/settings/ui/pages/AdminSettingsOnThisPage.component.spec.ts': [
    {
      rule: 'document-div-host',
      reason:
        'Verifies anchor scrolling by observing scrollIntoView on a real in-document section target; the scroll target must exist in the document outside the rendered component.',
    },
  ],
}

const componentRenderHelperExceptions: ExceptionMap<Exception> = {}

const componentClassAssertionExceptions: ExceptionMap<Exception> = {
  'src/features/account/ui/shared/ComingSoonPane.component.spec.ts': [
    {
      reason:
        'Verifies the neutral StatusBadge carrier, card surface override, and lack of a status dot in the coming-soon capsule.',
    },
  ],
  'src/features/account/ui/profile/ProfilePane.component.spec.ts': [
    {
      reason:
        'Verifies neutral creator and info admin role colors on the shared StatusBadge carrier.',
    },
  ],
  'src/features/admin/credits/ui/pricing/BillingStatusCell.component.spec.ts': [
    {
      reason:
        'Verifies centered status pills in the fixed-width billing status slot without pulse DOM.',
    },
  ],
  'src/features/admin/credits/ui/pricing/ModelPoolEditorPanel.component.spec.ts': [
    {
      reason:
        'Verifies the reserved validation row height that prevents model pool form layout shifts.',
    },
  ],
  'src/features/admin/credits/ui/rewards/AdminRewardCodesPage.component.spec.ts': [
    {
      reason: 'Verifies centered reward status pills in the fixed-width SwitchRow slot.',
    },
  ],
  'src/features/admin/models/ui/controls/ObservedStatsPanel.component.spec.ts': [
    {
      reason: 'Verifies success and neutral on-sale status colors without processing pulse DOM.',
    },
  ],
  'src/features/admin/settings/ui/cards/image-library/AdminImageProvidersTable.component.spec.ts': [
    {
      reason:
        'Verifies provider status color transitions and absence of row opacity changes while toggles settle.',
    },
  ],
  'src/features/admin/settings/ui/cards/mail/AdminMailConfigCard.component.spec.ts': [
    {
      reason:
        'Verifies mail readiness semantic colors and centered status pills in the fixed-width header slot.',
    },
  ],
  'src/features/admin/settings/ui/cards/outbound/AdminOutboundPolicyCard.component.spec.ts': [
    {
      reason:
        'Verifies configured outbound allowances use a neutral status instead of a warning tone.',
    },
  ],
  'src/features/admin/settings/ui/cards/payment/AdminPaymentSettingsCard.component.spec.ts': [
    {
      reason:
        'Verifies payment configuration remains neutral while product and webhook health use readiness tones.',
    },
  ],
  'src/features/admin/shared/ui/feedback/AdminStatusPill.component.spec.ts': [
    {
      reason: 'Verifies the Badge carrier, semantic tone and dot colors, and fixed size tokens.',
    },
  ],
  'src/features/admin/shared/ui/form/AdminSwitch.component.spec.ts': [
    {
      reason:
        'Verifies the switch track opacity transition, shared timing token, and thumb translation classes.',
    },
  ],
  'src/features/admin/shared/ui/form/SwitchRow.component.spec.ts': [
    {
      reason:
        'Verifies centered fixed-width status geometry, absolute pulse layering, nonshrinking switches, and row gap tokens.',
    },
  ],
  'src/features/admin/settings/ui/cards/storage/AdminStoragePage.component.spec.ts': [
    {
      reason:
        'Verifies storage status color transitions and stable row opacity during toggle updates.',
    },
  ],
  'src/core/components/StatusBadge.component.spec.ts': [
    {
      reason:
        'Verifies the shared Badge carrier, semantic tone and dot colors, fixed sizes, and caller class merging.',
    },
  ],
  'src/features/account/ui/dialog/AccountDialog.component.spec.ts': [
    {
      reason:
        'Verifies the Account dialog overlay shadow class that defines the shared visual chrome for the modal surface.',
    },
  ],
  'src/features/auth/components/auth-fields/authFields.component.spec.ts': [
    {
      reason:
        'Verifies the auth field prefix-icon absolute positioning contract shared by Email and Password inputs.',
    },
  ],
  'src/features/admin/settings/ui/cards/branding/AdminBrandingCard.component.spec.ts': [
    {
      reason:
        'Verifies the brand logo preview keeps object-cover parity with the Sidebar/Login/account rendering paths instead of silently switching to object-contain.',
    },
  ],
  'src/features/admin/shared/ui/shell/AdminDrawer.component.spec.ts': [
    {
      reason:
        'Verifies the drawer positioning contract (right-anchored full-height panel) that distinguishes AdminDrawer from the centered AdminDialogShell.',
    },
  ],
  'src/features/admin/shared/ui/table/AdminTable.component.spec.ts': [
    {
      reason:
        'Verifies the table visual contracts added with sticky headers, disabled-row hover suppression, checkbox density, and stale-row opacity; class strings are the contract surface AdminTable exposes.',
    },
  ],
  'src/features/auth/pages/oauthFlow.component.spec.ts': [
    {
      reason:
        'Verifies the one-provider OAuth layout contract that makes a single login button span the full row.',
    },
  ],
  'src/features/admin/monitor/ui/AdminTaskResultDialog.component.spec.ts': [
    {
      reason:
        'Verifies the keyboard focus visibility class contract for the image result action; the DOM harness cannot compute Tailwind focus pseudo-state styles.',
    },
  ],
  'src/features/account/ui/rewards/RewardsPane.component.spec.ts': [
    {
      reason:
        'Verifies that the redemption action stays on the User Shell semantic button contract instead of inheriting the Admin primary theme.',
    },
  ],
  'src/features/account/ui/security/SecurityPane.component.spec.ts': [
    {
      reason:
        'Verifies that the password action stays on the User Shell semantic button contract instead of inheriting the Admin primary theme.',
    },
  ],
  'src/features/account/ui/billing/BillingPane.component.spec.ts': [
    {
      reason:
        'Verifies the confirmed semantic color contract for income, ordinary consumption, and system deductions.',
    },
  ],
  'src/features/account/ui/usage/UsagePane.component.spec.ts': [
    {
      reason:
        'Verifies the fixed responsive 5-column to 2x2-plus-spanning-row layout and stable loading geometry required by the usage summary contract.',
    },
  ],
  'src/features/auth/components/authModalsMotion.component.spec.ts': [
    {
      reason:
        'Verifies the auth modal transparent surface and responsive width classes that define its visual/layout contract.',
    },
  ],
  'src/features/admin/app/AdminLayout.component.spec.ts': [
    {
      reason:
        'Verifies the admin sidebar responsive collapse/expand width classes and mobile drawer transform contract.',
    },
  ],
  'src/features/admin/shared/ui/shell/AdminSurfaceFrame.component.spec.ts': [
    {
      reason:
        'Verifies the shared Admin surface height and overflow contract across Management and Table adapters.',
    },
  ],
  'src/features/admin/shared/ui/toolbar/AdminToolbarSearch.component.spec.ts': [
    {
      reason:
        'Verifies the shared Admin search fluid width contract used by the management toolbars.',
    },
  ],
  'src/features/admin/shared/ui/shell/AdminDialogShell.component.spec.ts': [
    {
      reason: 'Verifies the shared Admin dialog footer layout contract.',
    },
  ],
  'src/features/canvas/presentation/nodes/home/ImageActionMenu.component.spec.ts': [
    {
      reason:
        'Verifies the visual active/hidden state class that coordinates the strength control overlay.',
    },
  ],
  'src/features/canvas/presentation/nodes/primitives/PromptComposerControls.options.component.spec.ts':
    [
      {
        reason:
          'Verifies the stable count-picker style hooks preserved by the generic output option picker refactor.',
      },
    ],
  'src/features/canvas/tools/retouch/ui/histogram/HistogramPlot.component.spec.ts': [
    {
      reason:
        'Verifies the active marker visual state for the reusable SVG histogram interaction surface.',
    },
  ],
  'src/features/canvas/tools/retouch/ui/levels/LevelsControl.component.spec.ts': [
    {
      reason:
        'Verifies SVG marker, guide-line, and numeric control visual classes for the levels editor.',
    },
  ],
  'src/features/canvas/tools/retouch/ui/sharpen/SharpenControl.component.spec.ts': [
    {
      reason:
        'Verifies the tonal track class contract that keeps the radius and threshold sliders visible.',
    },
  ],
  'src/features/canvas/tools/retouch/ui/threshold/ThresholdControl.component.spec.ts': [
    {
      reason:
        'Verifies SVG threshold marker, guide-line, and control visual classes for the threshold editor.',
    },
  ],
  'src/features/canvas/tools/inpaint/ui/ImageInpaintOverlay.component.spec.ts': [
    {
      reason:
        'Verifies the brush cursor visibility class after pointer movement in the mask overlay.',
    },
  ],
  'src/core/components/DataTableFrame.component.spec.ts': [
    {
      reason:
        'Verifies the single table-container scroll surface and its mechanical accessibility class.',
    },
  ],
  'src/core/components/ActionButton.component.spec.ts': [
    {
      reason:
        'Verifies the shared action chrome (built-in action class, small-size token, caller-class merge) that the migrated call sites rely on; the DOM harness cannot compute Tailwind styles.',
    },
  ],
}

const contractGeneratedQueryMockExceptions: ExceptionMap<GeneratedQueryMockException> = {}

const componentHarnessPatterns: readonly ComponentHarnessPattern[] = [
  {
    rule: 'raw-create-app',
    pattern: /\bcreateApp\s*\(/,
    message: 'Use src/test/render helpers instead of createApp() in component specs.',
  },
  {
    rule: 'raw-mount-call',
    pattern: /\.\s*mount\s*\(/,
    message: 'Use src/test/render helpers instead of raw .mount() in component specs.',
  },
  {
    rule: 'raw-unmount-call',
    pattern: /\.\s*unmount\s*\(/,
    message: 'Avoid raw .unmount() in component specs unless the cleanup need is explicit.',
  },
  {
    rule: 'document-div-host',
    pattern: /document\s*\.\s*createElement\s*\(\s*['"]div['"]\s*\)/,
    message: 'Use shared render helpers instead of ad hoc document.createElement("div") hosts.',
  },
  {
    rule: 'vue-test-utils',
    pattern: /from\s+['"]@vue\/test-utils['"]/,
    message: 'Use src/test/render instead of importing @vue/test-utils directly.',
  },
]

const componentClassAssertionPatterns: readonly ComponentClassAssertionPattern[] = [
  {
    rule: 'to-have-class',
    pattern: /\.\s*toHaveClass\s*\(/,
    message:
      'Avoid class assertions in component specs unless they document a visual/layout contract.',
  },
  {
    rule: 'class-list',
    pattern: /\.\s*classList\b/,
    message:
      'Avoid classList assertions in component specs unless they document a visual/layout contract.',
  },
  {
    rule: 'class-name',
    pattern: /\.\s*className\b/,
    message:
      'Avoid className assertions in component specs unless they document a visual/layout contract.',
  },
  {
    rule: 'vue-test-utils-classes',
    pattern: /\.\s*classes\s*\(/,
    message:
      'Avoid Vue Test Utils classes() assertions in component specs unless they document a visual/layout contract.',
  },
  {
    rule: 'class-attribute',
    pattern: /\.\s*attributes\s*\(\s*['"]class['"]\s*\)/,
    message:
      'Avoid class attribute assertions in component specs unless they document a visual/layout contract.',
  },
]

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const srcRoot = path.join(webRoot, 'src')

async function listSpecFiles(dir: string, baseDir = dir): Promise<SpecFile[]> {
  if (!existsSync(dir)) return []

  const entries = await readdir(dir)
  const nestedFiles = await Promise.all(
    entries.map(async (entry) => {
      const absolutePath = path.join(dir, entry)
      const entryStat = await stat(absolutePath)

      if (entryStat.isDirectory()) return listSpecFiles(absolutePath, baseDir)
      if (!entryStat.isFile() || !entry.endsWith('.spec.ts')) return []

      const relativePath = toPosixPath(path.relative(baseDir, absolutePath))
      return [
        {
          absolutePath,
          relativePath,
          sourcePath: `src/${relativePath}`,
        },
      ]
    }),
  )

  return nestedFiles.flat().sort((left, right) => left.sourcePath.localeCompare(right.sourcePath))
}

async function listSourceFiles(dir: string, baseDir = dir): Promise<SpecFile[]> {
  if (!existsSync(dir)) return []

  const entries = await readdir(dir)
  const nestedFiles = await Promise.all(
    entries.map(async (entry) => {
      const absolutePath = path.join(dir, entry)
      const entryStat = await stat(absolutePath)

      if (entryStat.isDirectory()) return listSourceFiles(absolutePath, baseDir)
      if (!entryStat.isFile() || !entry.endsWith('.ts') || entry.endsWith('.spec.ts')) return []

      const relativePath = toPosixPath(path.relative(baseDir, absolutePath))
      return [
        {
          absolutePath,
          relativePath,
          sourcePath: `src/${relativePath}`,
        },
      ]
    }),
  )

  return nestedFiles.flat().sort((left, right) => left.sourcePath.localeCompare(right.sourcePath))
}

function toPosixPath(filePath: string): string {
  return filePath.split(path.sep).join('/')
}

function hasExceptionReason(exception: Exception): boolean {
  return exception.reason.trim().length > 0
}

function checkExceptionReasons<T extends Exception>(
  exceptions: ExceptionMap<T>,
  mapName: string,
): Violation[] {
  const violations: Violation[] = []

  for (const [file, entries] of Object.entries(exceptions)) {
    entries.forEach((entry, index) => {
      if (hasExceptionReason(entry)) return

      violations.push({
        file,
        rule: `${mapName}.reason`,
        message: `Exception entry ${index + 1} must include a reason.`,
      })
    })
  }

  return violations
}

function checkSpecNaming(specFiles: readonly SpecFile[]): Violation[] {
  return specFiles.flatMap((file) => {
    if (allowedSpecSuffixes.some((suffix) => file.relativePath.endsWith(suffix))) return []

    return [
      {
        file: file.sourcePath,
        rule: 'spec-file-name',
        message: `Spec files must end with one of: ${allowedSpecSuffixes.join(', ')}.`,
      },
    ]
  })
}

function hasComponentHarnessException(
  file: string,
  rule: ComponentHarnessRule,
  usedExceptions: Set<string>,
): boolean {
  const exception = componentHarnessExceptions[file]?.find((entry) => entry.rule === rule)
  if (!exception) return false

  usedExceptions.add(componentHarnessExceptionKey(file, rule))
  return true
}

function componentHarnessExceptionKey(file: string, rule: ComponentHarnessRule): string {
  return `${file}:${rule}`
}

function hasRenderHelperException(file: string, usedExceptions: Set<string>): boolean {
  const hasException = Boolean(componentRenderHelperExceptions[file]?.length)
  if (hasException) usedExceptions.add(file)
  return hasException
}

function hasComponentClassAssertionException(file: string, usedExceptions: Set<string>): boolean {
  const hasException = Boolean(componentClassAssertionExceptions[file]?.length)
  if (hasException) usedExceptions.add(file)
  return hasException
}

function hasGeneratedQueryMockException(
  file: string,
  module: string,
  usedExceptions: Set<string>,
): boolean {
  const exception = contractGeneratedQueryMockExceptions[file]?.find(
    (entry) => entry.module === module,
  )
  if (!exception) return false

  usedExceptions.add(generatedQueryMockExceptionKey(file, module))
  return true
}

function generatedQueryMockExceptionKey(file: string, module: string): string {
  return `${file}:${module}`
}

function checkComponentHarness(
  specFile: SpecFile,
  content: string,
  usedExceptions: Set<string>,
): Violation[] {
  if (!specFile.relativePath.endsWith('.component.spec.ts')) return []

  return componentHarnessPatterns.flatMap(({ rule, pattern, message }) => {
    if (!pattern.test(content)) return []
    if (hasComponentHarnessException(specFile.sourcePath, rule, usedExceptions)) return []

    return [
      {
        file: specFile.sourcePath,
        rule,
        message,
      },
    ]
  })
}

function checkComponentRenderHelper(
  specFile: SpecFile,
  content: string,
  usedExceptions: Set<string>,
): Violation[] {
  if (!specFile.relativePath.endsWith('.component.spec.ts')) return []
  if (!importsVueComponent(content)) return []
  if (usesSharedRenderHelper(content)) return []
  if (hasRenderHelperException(specFile.sourcePath, usedExceptions)) return []

  return [
    {
      file: specFile.sourcePath,
      rule: 'component-render-helper',
      message:
        'Component specs that import .vue files must call render() or renderWithUser() from src/test/render.',
    },
  ]
}

function checkComponentClassAssertions(
  specFile: SpecFile,
  content: string,
  usedExceptions: Set<string>,
): Violation[] {
  if (!specFile.relativePath.endsWith('.component.spec.ts')) return []

  const violatedPatterns = componentClassAssertionPatterns.filter(({ pattern }) =>
    pattern.test(content),
  )
  if (violatedPatterns.length === 0) return []
  if (hasComponentClassAssertionException(specFile.sourcePath, usedExceptions)) return []

  return violatedPatterns.map(({ rule, message }) => ({
    file: specFile.sourcePath,
    rule,
    message,
  }))
}

function importsVueComponent(content: string): boolean {
  return /\bimport\b[\s\S]*?\bfrom\s+['"][^'"]+\.vue['"]/.test(content)
}

function usesSharedRenderHelper(content: string): boolean {
  return importsSharedRenderHelper(content) && /\b(?:render|renderWithUser)\s*\(/.test(content)
}

function importsSharedRenderHelper(content: string): boolean {
  const importPattern = /\bimport\s*\{([^}]*)\}\s*from\s+['"][^'"]*\/test\/render['"]/g
  let match = importPattern.exec(content)

  while (match) {
    const importedNames = (match[1] ?? '')
      .split(',')
      .map((name) =>
        name
          .trim()
          .split(/\s+as\s+/)[0]
          ?.trim(),
      )
      .filter(Boolean)

    if (importedNames.includes('render') || importedNames.includes('renderWithUser')) return true

    match = importPattern.exec(content)
  }

  return false
}

function checkContractGeneratedQueryMocks(
  specFile: SpecFile,
  content: string,
  usedExceptions: Set<string>,
): Violation[] {
  if (!specFile.relativePath.endsWith('.contract.spec.ts')) return []

  const violations: Violation[] = []
  const mockPattern = /\bvi\.(mock|doMock)\s*\(\s*(['"`])([^'"`]*generated-query[^'"`]*)\2/g
  let match = mockPattern.exec(content)

  while (match) {
    const mockKind = match[1] ?? ''
    const module = match[3] ?? ''
    if (hasGeneratedQueryMockException(specFile.sourcePath, module, usedExceptions)) {
      match = mockPattern.exec(content)
      continue
    }

    violations.push({
      file: specFile.sourcePath,
      rule: 'contract-generated-query-mock',
      message: 'Contract specs must not mock generated-query modules.',
      details: [`vi.${mockKind}('${module}')`],
    })
    match = mockPattern.exec(content)
  }

  return violations
}

function checkStaleComponentHarnessExceptions(usedExceptions: ReadonlySet<string>): Violation[] {
  return Object.entries(componentHarnessExceptions).flatMap(([file, exceptions]) =>
    exceptions.flatMap((exception) => {
      if (usedExceptions.has(componentHarnessExceptionKey(file, exception.rule))) return []

      return [
        {
          file,
          rule: 'stale-component-harness-exception',
          message: `Remove stale component harness exception for ${exception.rule}.`,
        },
      ]
    }),
  )
}

function checkStaleRenderHelperExceptions(usedExceptions: ReadonlySet<string>): Violation[] {
  return Object.keys(componentRenderHelperExceptions).flatMap((file) => {
    if (usedExceptions.has(file)) return []

    return [
      {
        file,
        rule: 'stale-component-render-helper-exception',
        message: 'Remove stale component render helper exception.',
      },
    ]
  })
}

function checkStaleComponentClassAssertionExceptions(
  usedExceptions: ReadonlySet<string>,
): Violation[] {
  return Object.keys(componentClassAssertionExceptions).flatMap((file) => {
    if (usedExceptions.has(file)) return []

    return [
      {
        file,
        rule: 'stale-component-class-assertion-exception',
        message: 'Remove stale component class assertion exception.',
      },
    ]
  })
}

function checkStaleGeneratedQueryMockExceptions(usedExceptions: ReadonlySet<string>): Violation[] {
  return Object.entries(contractGeneratedQueryMockExceptions).flatMap(([file, exceptions]) =>
    exceptions.flatMap((exception) => {
      if (usedExceptions.has(generatedQueryMockExceptionKey(file, exception.module))) return []

      return [
        {
          file,
          rule: 'stale-generated-query-mock-exception',
          message: `Remove stale generated-query mock exception for ${exception.module}.`,
        },
      ]
    }),
  )
}

async function checkSpecFiles(specFiles: readonly SpecFile[]): Promise<Violation[]> {
  const usedComponentHarnessExceptions = new Set<string>()
  const usedRenderHelperExceptions = new Set<string>()
  const usedComponentClassAssertionExceptions = new Set<string>()
  const usedGeneratedQueryMockExceptions = new Set<string>()
  const violations = [...checkSpecNaming(specFiles)]

  for (const specFile of specFiles) {
    const content = await readFile(specFile.absolutePath, 'utf8')
    violations.push(
      ...checkComponentHarness(specFile, content, usedComponentHarnessExceptions),
      ...checkComponentRenderHelper(specFile, content, usedRenderHelperExceptions),
      ...checkComponentClassAssertions(specFile, content, usedComponentClassAssertionExceptions),
      ...checkContractGeneratedQueryMocks(specFile, content, usedGeneratedQueryMockExceptions),
    )
  }

  violations.push(
    ...checkExceptionReasons(componentHarnessExceptions, 'componentHarnessExceptions'),
    ...checkExceptionReasons(componentRenderHelperExceptions, 'componentRenderHelperExceptions'),
    ...checkExceptionReasons(
      componentClassAssertionExceptions,
      'componentClassAssertionExceptions',
    ),
    ...checkExceptionReasons(
      contractGeneratedQueryMockExceptions,
      'contractGeneratedQueryMockExceptions',
    ),
    ...checkStaleComponentHarnessExceptions(usedComponentHarnessExceptions),
    ...checkStaleRenderHelperExceptions(usedRenderHelperExceptions),
    ...checkStaleComponentClassAssertionExceptions(usedComponentClassAssertionExceptions),
    ...checkStaleGeneratedQueryMockExceptions(usedGeneratedQueryMockExceptions),
  )

  return violations.sort((left, right) => left.file.localeCompare(right.file))
}

function printViolations(violations: readonly Violation[]) {
  console.error('Frontend test guardrails failed.')
  console.error('')

  for (const violation of violations) {
    console.error(`- ${violation.file}`)
    console.error(`  [${violation.rule}] ${violation.message}`)
    for (const detail of violation.details ?? []) {
      console.error(`    - ${detail}`)
    }
  }

  console.error('')
  console.error(
    'Fix the test, rename it, or add a narrow exception with a reason in apps/web/scripts/checkTestGuardrails.ts.',
  )
}

async function warnNewMakeFactories(): Promise<void> {
  const sourceFiles = await listSourceFiles(srcRoot)
  const fresh: string[] = []

  for (const file of sourceFiles) {
    if (file.sourcePath.includes('generated-query')) continue
    const content = await readFile(file.absolutePath, 'utf8')
    for (const name of collectNewMakeFactories(file.sourcePath, content)) {
      fresh.push(`${file.sourcePath} (${name})`)
    }
  }

  for (const entry of fresh.sort()) {
    console.warn(
      `[guardrail:make-factory-warn] ${entry} :: new factories use buildX(overrides?); existing makeX seams are grandfathered by file and public symbol.`,
    )
  }
  if (fresh.length === 0) {
    console.log('Factory naming guardrail passed (no new exported makeX factories).')
  }
}

async function main() {
  checkMakeFactoryFixtures()
  await warnNewMakeFactories()
  const specFiles = await listSpecFiles(srcRoot)
  const violations = await checkSpecFiles(specFiles)

  if (violations.length > 0) {
    printViolations(violations)
    process.exitCode = 1
    return
  }

  console.log(`Frontend test guardrails passed (${specFiles.length} spec files checked).`)
}

void main().catch((error: unknown) => {
  console.error(error instanceof Error ? error.message : error)
  process.exitCode = 1
})
