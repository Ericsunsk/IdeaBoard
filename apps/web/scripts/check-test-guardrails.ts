import { existsSync } from 'node:fs'
import { readdir, readFile, stat } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

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

const componentHarnessExceptions: ExceptionMap<ComponentHarnessException> = {
  'src/features/canvas/components/home/HomeCanvasImageActionMenu.component.spec.ts': [
    {
      rule: 'raw-unmount-call',
      reason:
        'Unmounts one Testing Library render result between multiple render() calls; the component harness still goes through src/test/render.',
    },
  ],
}

const componentRenderHelperExceptions: ExceptionMap<Exception> = {}

const componentClassAssertionExceptions: ExceptionMap<Exception> = {
  'src/features/auth/pages/OAuthFlow.component.spec.ts': [
    {
      reason:
        'Verifies the one-provider OAuth layout contract that makes a single login button span the full row.',
    },
  ],
  'src/features/admin/task-monitor/AdminTaskResultDialog.component.spec.ts': [
    {
      reason:
        'Verifies the keyboard focus visibility class contract for the image result action; the DOM harness cannot compute Tailwind focus pseudo-state styles.',
    },
  ],
  'src/features/account-management/components/UserRewardsPane.component.spec.ts': [
    {
      reason:
        'Verifies that the redemption action stays on the User Shell semantic button contract instead of inheriting the Admin PrimeVue primary theme.',
    },
  ],
  'src/features/account-management/components/UserSecurityPane.component.spec.ts': [
    {
      reason:
        'Verifies that the password action stays on the User Shell semantic button contract instead of inheriting the Admin PrimeVue primary theme.',
    },
  ],
  'src/features/account-management/components/UserBillingRecordsPane.component.spec.ts': [
    {
      reason:
        'Verifies the confirmed semantic color contract for income, ordinary consumption, and system deductions.',
    },
  ],
  'src/features/account-management/components/UserUsagePane.component.spec.ts': [
    {
      reason:
        'Verifies the fixed responsive 5-column to 2x2-plus-spanning-row layout and stable loading geometry required by the usage summary contract.',
    },
  ],
  'src/features/admin/shared/components/AdminSurfaceFrame.component.spec.ts': [
    {
      reason:
        'Verifies the shared Admin surface height and overflow contract across Management, Table, and Resource Grid adapters.',
    },
  ],
  'src/features/admin/shared/components/AdminDialogShell.component.spec.ts': [
    {
      reason: 'Verifies the shared Admin dialog footer layout contract.',
    },
  ],
  'src/features/canvas/components/home/HomeCanvasImageActionMenu.component.spec.ts': [
    {
      reason:
        'Verifies the visual active/hidden state class that coordinates the strength control overlay.',
    },
  ],
  'src/features/canvas/components/nodes/primitives/PromptComposerControls.options.component.spec.ts': [
    {
      reason:
        'Verifies the stable count-picker style hooks preserved by the generic output option picker refactor.',
    },
  ],
  'src/features/canvas/image-editor/components/ImageEditorHistogramPlot.component.spec.ts': [
    {
      reason:
        'Verifies the active marker visual state for the reusable SVG histogram interaction surface.',
    },
  ],
  'src/features/canvas/image-editor/components/ImageEditorLevelsControl.component.spec.ts': [
    {
      reason:
        'Verifies SVG marker, guide-line, and numeric control visual classes for the levels editor.',
    },
  ],
  'src/features/canvas/image-editor/components/ImageEditorSharpenControl.component.spec.ts': [
    {
      reason:
        'Verifies the tonal track class contract that keeps the radius and threshold sliders visible.',
    },
  ],
  'src/features/canvas/image-editor/components/ImageEditorThresholdControl.component.spec.ts': [
    {
      reason:
        'Verifies SVG threshold marker, guide-line, and control visual classes for the threshold editor.',
    },
  ],
  'src/features/canvas/mask-editor/components/ImageMaskEditorOverlay.component.spec.ts': [
    {
      reason:
        'Verifies the brush cursor visibility class after pointer movement in the mask overlay.',
    },
  ],
  'src/shared/components/DataTableFrame.component.spec.ts': [
    {
      reason:
        'Verifies the single PrimeVue table-container scroll surface and its mechanical accessibility class.',
    },
  ],
}

const contractGeneratedQueryMockExceptions: ExceptionMap<GeneratedQueryMockException> = {
  'src/features/admin/task-monitor/useAdminTaskMonitor.contract.spec.ts': [
    {
      module: '../../../shared/api/generated-query/admin-tasks/admin-tasks',
      reason:
        'Hard boundary for task-list polling state; the generated query hook is mocked to drive composable states directly.',
    },
    {
      module: '../../../shared/api/generated-query/admin-monitor/admin-monitor',
      reason:
        'Hard boundary for monitor polling state; the generated query hook is mocked to drive composable states directly.',
    },
  ],
}

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
    const importedNames = match[1]
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
    const mockKind = match[1]
    const module = match[3]
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
    'Fix the test, rename it, or add a narrow exception with a reason in apps/web/scripts/check-test-guardrails.ts.',
  )
}

async function main() {
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
