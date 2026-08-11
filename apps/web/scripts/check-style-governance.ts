import { existsSync } from 'node:fs'
import { readdir, readFile, stat } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { findMotionPolicyViolations, findStaleMotionAllowances } from './motion-policy-guard'

type Violation = {
  file: string
  rule: string
  message: string
  details?: readonly string[]
}

type SourceFile = {
  absolutePath: string
  sourcePath: string
}

type CountAllowance<T extends string> = {
  counts: Partial<Record<T, number>>
  reason: string
}

type CountRule<T extends string> = {
  id: T
  pattern: RegExp
  message: string
}

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const srcRoot = path.join(webRoot, 'src')
const sourceExtensions = new Set(['.css', '.ts', '.vue'])

const styleCountRules = [
  {
    id: 'raw-color',
    pattern:
      /#[0-9A-Fa-f]{3,8}\b|\brgba?\(|\bhsla?\(|\b(?:bg|text|border|from|to|via|shadow|drop-shadow|ring|outline)-\[[^\]]*(?:#[0-9A-Fa-f]{3,8}|rgba?\(|hsla?\()[^\]]*\]/g,
    message: 'Use Admin semantic tokens/helpers instead of adding raw colors.',
  },
  {
    id: 'raw-shadow',
    pattern: /(?:shadow|drop-shadow)-\[[^\]]+\]|box-shadow\s*:/g,
    message: 'Use shared shadow tokens/helpers instead of adding raw shadows.',
  },
  {
    id: 'raw-z-index',
    pattern: /\bz-\[[^\]]+\]/g,
    message: 'Use named layering helpers/tokens instead of adding raw z-index values.',
  },
  {
    id: 'raw-motion',
    pattern: /(?:duration|ease|transition|animate)-\[[^\]]+\]|transition\s*:/g,
    message: 'Use motion tokens/helpers instead of adding raw motion values.',
  },
] as const satisfies readonly CountRule<string>[]

type StyleCountRuleId = (typeof styleCountRules)[number]['id']

const rawStyleAllowances: Record<string, CountAllowance<StyleCountRuleId>> = {
  'src/features/admin/app/AdminLayout.vue': {
    counts: {
      'raw-color': 1,
      'raw-shadow': 4,
      'raw-z-index': 3,
    },
    reason: 'Admin shell owns bespoke rail layering and visual constants.',
  },
  'src/features/admin/pages/login/AdminLoginPage.vue': {
    counts: {
      'raw-color': 1,
      'raw-shadow': 1,
    },
    reason: 'Login first-screen card composition keeps a one-off Admin entry shadow.',
  },
  'src/features/admin/pages/prompt-config/AdminPromptConfigPage.vue': {
    counts: {
      'raw-color': 4,
    },
    reason: 'Prompt variable tone chips use fixed translucent fills for domain categories.',
  },
  'src/features/admin/pages/system-settings/AdminSystemSettingsPage.vue': {
    counts: {
      'raw-shadow': 1,
    },
    reason: 'Logo upload affordance uses a focus-ring shadow that bridges an Admin token.',
  },
  'src/shared/theme/panelCardPt.ts': {
    counts: {
      'raw-shadow': 1,
    },
    reason: 'Shared panel Card pass-through owns the cross-surface panel shadow contract.',
  },
  'src/features/admin/shared/lib/adminOverlayPt.ts': {
    counts: {
      'raw-shadow': 2,
    },
    reason: 'Admin overlay pass-through owns the menu and dialog overlay shadow contracts.',
  },
  'src/features/admin/shared/lib/adminUiClasses.ts': {
    counts: {
      'raw-color': 1,
      'raw-shadow': 2,
    },
    reason: 'Admin shared UI helpers own metric/control shadow primitives.',
  },
  'src/features/admin/shared/styles/admin-user-avatar-button.css': {
    counts: {
      'raw-color': 1,
    },
    reason: 'Avatar CSS contract keeps a fallback color for the cross-feature avatar bridge.',
  },
  'src/features/admin/task-monitor/taskMonitorStatus.ts': {
    counts: {
      'raw-color': 8,
      'raw-shadow': 4,
    },
    reason: 'Task status palette is a domain visual contract shared by table and detail views.',
  },
}

const selectorCountRules = [
  {
    id: 'deep-selector',
    pattern: /:deep\s*\(/g,
    message: 'Avoid :deep() in business components; expose an explicit class or pt helper.',
  },
  {
    id: 'global-selector',
    pattern: /:global\s*\(/g,
    message: 'Avoid :global() unless the feature owns a documented cross-DOM contract.',
  },
  {
    id: 'primevue-internal-selector',
    pattern: /\.p-[a-z][\w-]*/g,
    message: 'PrimeVue internals must be styled through pt helpers or documented vendor bridges.',
  },
  {
    id: 'vue-flow-internal-selector',
    pattern: /\.vue-flow(?:__|\b)[\w-]*/g,
    message: 'Vue Flow internals must stay inside the Canvas adapter skin.',
  },
] as const satisfies readonly CountRule<string>[]

type SelectorCountRuleId = (typeof selectorCountRules)[number]['id']

const selectorAllowances: Record<string, CountAllowance<SelectorCountRuleId>> = {
  'src/app/style.css': {
    counts: {
      'primevue-internal-selector': 17,
    },
    reason: 'Admin route typography and tooltip bridge for PrimeVue vendor DOM.',
  },
  'src/features/canvas/components/nodes/NodeToolIconButton.vue': {
    counts: {
      'deep-selector': 1,
    },
    reason: 'Slot icon contract: external icon components render SVGs inside this slot.',
  },
  'src/features/canvas/components/styles/canvas-vue-flow-adapter-skin.css': {
    counts: {
      'vue-flow-internal-selector': 39,
    },
    reason: 'Central adapter skin owns Vue Flow vendor DOM styling.',
  },
  'src/features/canvas/screen-presentation/CanvasScreen.vue': {
    counts: {
      'global-selector': 2,
    },
    reason: 'Canvas route toggles html/body overscroll guard during direct manipulation.',
  },
}

async function listSourceFiles(dir: string, baseDir = dir): Promise<SourceFile[]> {
  if (!existsSync(dir)) return []

  const entries = await readdir(dir)
  const nestedFiles = await Promise.all(
    entries.map(async (entry) => {
      const absolutePath = path.join(dir, entry)
      const entryStat = await stat(absolutePath)

      if (entryStat.isDirectory()) return listSourceFiles(absolutePath, baseDir)
      if (!entryStat.isFile() || !sourceExtensions.has(path.extname(entry))) return []

      const relativePath = toPosixPath(path.relative(baseDir, absolutePath))
      return [
        {
          absolutePath,
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

function clonePattern(pattern: RegExp): RegExp {
  return new RegExp(pattern.source, pattern.flags)
}

function matchDetails(content: string, pattern: RegExp, maxDetails = 5): string[] {
  const details: string[] = []
  const matcher = clonePattern(pattern)
  let match = matcher.exec(content)

  while (match && details.length < maxDetails) {
    const lineNumber = content.slice(0, match.index).split('\n').length
    const line = content.split('\n')[lineNumber - 1]?.trim() ?? match[0]
    details.push(`line ${lineNumber}: ${line}`)
    match = matcher.exec(content)
  }

  return details
}

function countMatches(content: string, pattern: RegExp): number {
  return [...content.matchAll(clonePattern(pattern))].length
}

function isAdminProductionStyleFile(file: SourceFile): boolean {
  return file.sourcePath.startsWith('src/features/admin/') && !file.sourcePath.includes('.spec.')
}

function isCanvasTokenAllowedFile(file: SourceFile): boolean {
  return (
    file.sourcePath === 'src/app/style.css' || file.sourcePath.startsWith('src/features/canvas/')
  )
}

function isPageDotBackgroundAllowedFile(file: SourceFile): boolean {
  return (
    file.sourcePath === 'src/app/style.css' ||
    file.sourcePath.startsWith('src/features/canvas-board/preview/')
  )
}

function checkAllowanceReasons<T extends string>(
  allowances: Record<string, CountAllowance<T>>,
  mapName: string,
): Violation[] {
  return Object.entries(allowances).flatMap(([file, allowance]) => {
    if (allowance.reason.trim()) return []
    return [
      {
        file,
        rule: `${mapName}.reason`,
        message: 'Style governance allowances must include a reason.',
      },
    ]
  })
}

function checkCountRules<T extends string>(
  file: SourceFile,
  content: string,
  rules: readonly CountRule<T>[],
  allowances: Record<string, CountAllowance<T>>,
): Violation[] {
  const allowance = allowances[file.sourcePath]
  const violations: Violation[] = []

  for (const rule of rules) {
    const count = countMatches(content, rule.pattern)
    const allowed = allowance?.counts[rule.id] ?? 0
    if (count <= allowed) continue

    violations.push({
      file: file.sourcePath,
      rule: rule.id,
      message: `${rule.message} Found ${count}, allowed ${allowed}.`,
      details: matchDetails(content, rule.pattern),
    })
  }

  return violations
}

function checkStaleAllowances<T extends string>(
  filesByPath: ReadonlyMap<string, string>,
  rules: readonly CountRule<T>[],
  allowances: Record<string, CountAllowance<T>>,
  staleRuleName: string,
): Violation[] {
  return Object.entries(allowances).flatMap(([sourcePath, allowance]) => {
    const content = filesByPath.get(sourcePath) ?? ''

    return Object.entries(allowance.counts).flatMap(([ruleId, allowed]) => {
      const rule = rules.find((item) => item.id === ruleId)
      if (!rule || allowed === undefined) return []

      const count = countMatches(content, rule.pattern)
      if (count === allowed) return []

      return [
        {
          file: sourcePath,
          rule: staleRuleName,
          message: `Update stale ${ruleId} allowance: found ${count}, allowed ${allowed}.`,
        },
      ]
    })
  })
}

function checkCanvasTokenBoundaries(file: SourceFile, content: string): Violation[] {
  if (file.sourcePath.includes('.spec.') || file.sourcePath.includes('.test.')) return []

  const pattern = /(?:var\(\s*)?--(?:color-canvas|canvas)(?:-[\w-]+)?/g
  const count = countMatches(content, pattern)
  if (count === 0 || isCanvasTokenAllowedFile(file)) return []

  return [
    {
      file: file.sourcePath,
      rule: 'non-canvas-canvas-token',
      message:
        'Non-Canvas code must not depend directly on Canvas tokens or Canvas motion variables.',
      details: matchDetails(content, pattern),
    },
  ]
}

function checkPageDotBackgroundUtilityBoundary(file: SourceFile, content: string): Violation[] {
  const pattern = /radial-gradient\(\s*circle\s*,[^\]\n)]*(?:\s|_)1px\s*,\s*transparent(?:\s|_)1px/g
  const count = countMatches(content, pattern)
  if (count === 0 || isPageDotBackgroundAllowedFile(file)) return []

  return [
    {
      file: file.sourcePath,
      rule: 'page-dot-background-utility',
      message:
        'User page dot backgrounds must use ib-page-dot-bg; board preview dots stay in canvas-board/preview because previews mirror the serialized canvas thumbnail surface.',
      details: matchDetails(content, pattern),
    },
  ]
}

async function checkStyleGovernance(files: readonly SourceFile[]): Promise<Violation[]> {
  const contents = new Map<string, string>()
  const violations: Violation[] = [
    ...checkAllowanceReasons(rawStyleAllowances, 'rawStyleAllowances'),
    ...checkAllowanceReasons(selectorAllowances, 'selectorAllowances'),
  ]

  for (const file of files) {
    const content = await readFile(file.absolutePath, 'utf8')
    contents.set(file.sourcePath, content)

    violations.push(
      ...checkCanvasTokenBoundaries(file, content),
      ...checkPageDotBackgroundUtilityBoundary(file, content),
      ...checkCountRules(file, content, selectorCountRules, selectorAllowances),
      ...findMotionPolicyViolations(file.sourcePath, content).map((violation) => ({
        file: file.sourcePath,
        ...violation,
      })),
    )

    if (isAdminProductionStyleFile(file)) {
      violations.push(...checkCountRules(file, content, styleCountRules, rawStyleAllowances))
    }
  }

  violations.push(
    ...findStaleMotionAllowances(contents).map((violation) => ({
      file: 'motion-policy-guard',
      ...violation,
    })),
    ...checkStaleAllowances(
      contents,
      styleCountRules,
      rawStyleAllowances,
      'stale-raw-style-allowance',
    ),
    ...checkStaleAllowances(
      contents,
      selectorCountRules,
      selectorAllowances,
      'stale-selector-allowance',
    ),
  )

  return violations.sort((left, right) => left.file.localeCompare(right.file))
}

function printViolations(violations: readonly Violation[]) {
  console.error('Frontend style governance guardrails failed.')
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
    'Move styling behind the appropriate token/helper/adapter, or add a narrow allowance with a reason in apps/web/scripts/check-style-governance.ts.',
  )
}

async function main() {
  const sourceFiles = await listSourceFiles(srcRoot)
  const violations = await checkStyleGovernance(sourceFiles)

  if (violations.length > 0) {
    printViolations(violations)
    process.exitCode = 1
    return
  }

  console.log(`Frontend style governance guardrails passed (${sourceFiles.length} files checked).`)
}

void main().catch((error: unknown) => {
  console.error(error instanceof Error ? error.message : error)
  process.exitCode = 1
})
