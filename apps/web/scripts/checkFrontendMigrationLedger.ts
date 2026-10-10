import { createHash } from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import ts from 'typescript'
import { loadSeamInventory } from './seamInventory.ts'

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const inventoryPath = path.join(webRoot, '../../docs/architecture/frontend-contract-inventory.md')
const phases = ['P1a', 'P1b', 'P2', 'P2.5', 'P3', 'P4', 'P5', 'P6', 'P7']
const dispositions = ['retire', 'retain-behavior', 'retain-fixture', 'frozen-i18n-candidate']
type Assertion = {
  id: string
  source: string
  line: number
  scenario: string
  assertion: string
  fingerprint: string
  occurrence: number
}
type Constraint = Assertion & {
  disposition: string
  phase: string | null
  proposedPhase: string | null
  owner: string
  risk: string
  reason: string
  retirementStatus: string
  replacement: {
    id: string
    kind: string
    requirement: string
    status: string
  }
  reviewRequired: boolean
}
type Behavior = {
  id: string
  phase: string
  owner: string
  requirement: string
  status: string
  evidence: Array<Omit<Assertion, 'id' | 'occurrence'>>
  plannedAssertion: string | null
  verifiedRun: string | null
}
type FileMapping = {
  source: string
  targets: string[]
  phase: string
  owner: string
  disposition: string
  evidence: string
  productionFeatureConsumers: string[]
  consumerEvidence: { kind: string; entries?: string[]; count?: number }
  consumers?: Array<{ from: string; kind: string }>
}
type Ledger = {
  version: number
  frozenAt: string
  checkpointSha: string
  integrator: string
  status: string
  seamInventory: string
  method: {
    unit: string
    scope: string
    sourceFiles: string[]
    directSourceMarkers: string[]
    exclusions: Array<{ path: string; reason: string }>
  }
  counts: Record<string, number>
  constraints: Constraint[]
  behaviors: Behavior[]
  quotas: Array<{
    phase: string
    retirementIds: string[]
    R_p: number
    behaviorIds: string[]
    B_p: number
    zeroRetirementReason: string | null
    zeroBehaviorReason: string | null
  }>
  budget: {
    totalPlannedNetHours: number
    authorization: string
    killSwitch: string
    plannedWorkingDays: string[]
    packages: Array<{
      id: string
      scope: string
      owner: string
      plannedNetHours: number
      actualNetHours: number | null
      plannedCompletionDate: string
      completedAt: string | null
      status: string
      checkpointSha: string
    }>
  }
  checkpoints: {
    rollingIntegrationHead: string
    latestFullyGreenSha: string | null
    immutableRollbackSha: string | null
    ciEvidence: string | null
    phaseStartRequirement: string
  }
  generatedMapping: {
    sourcePattern: string
    targetPattern: string
    measuredFileCount: number
    sourcePathSha256: string
    phase: string
    owner: string
    policy: string
  }
  sharedFileMappings: FileMapping[]
  contextMappings: Array<{ source: string; target: string; action: string }>
  pathMappings: Array<{
    source: string
    target: string
    phase: string
    owner: string
    requirement: string
    status: string
  }>
}
function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(`Migration ledger: ${message}`)
}
function nonempty(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0
}
function hash(value: string): string {
  return createHash('sha256').update(value).digest('hex').slice(0, 16)
}
function sameStrings(actual: string[], expected: string[]): boolean {
  return JSON.stringify([...actual].sort()) === JSON.stringify([...expected].sort())
}
function uniqueIds(values: Array<{ id: string }>, label: string) {
  requireCondition(
    values.every((value) => nonempty(value.id)),
    `${label} missing ID`,
  )
  requireCondition(
    new Set(values.map((value) => value.id)).size === values.length,
    `${label} duplicate ID`,
  )
}
function validateLedger(value: Ledger): Ledger {
  requireCondition(value && value.version === 1, 'unsupported schema version')
  requireCondition(/^[a-f\d]{40}$/.test(value.checkpointSha), 'checkpoint SHA')
  requireCondition(
    nonempty(value.integrator) && nonempty(value.method?.scope),
    'integrator/method scope',
  )
  requireCondition(
    Array.isArray(value.constraints) &&
      Array.isArray(value.behaviors) &&
      Array.isArray(value.quotas),
    'constraint/behavior/quota arrays',
  )
  uniqueIds(value.constraints, 'constraints')
  uniqueIds(value.behaviors, 'behaviors')
  for (const row of value.constraints) {
    requireCondition(dispositions.includes(row.disposition), `${row.id}: disposition`)
    requireCondition(
      nonempty(row.source) &&
        nonempty(row.scenario) &&
        nonempty(row.assertion) &&
        nonempty(row.owner) &&
        nonempty(row.reason),
      `${row.id}: missing source/scenario/assertion/owner/reason`,
    )
    requireCondition(
      /^[a-f\d]{16}$/.test(row.fingerprint) &&
        row.id === `source-${row.fingerprint}-${row.occurrence}` &&
        Number.isInteger(row.occurrence) &&
        row.occurrence > 0 &&
        Number.isInteger(row.line) &&
        row.line > 0,
      `${row.id}: stable call-site identity`,
    )
    requireCondition(['high', 'medium', 'low'].includes(row.risk), `${row.id}: risk`)
    requireCondition(
      row.disposition === 'frozen-i18n-candidate'
        ? row.phase === null && phases.includes(row.proposedPhase ?? '')
        : phases.includes(row.phase ?? ''),
      `${row.id}: phase assignment`,
    )
    requireCondition(
      row.replacement &&
        nonempty(row.replacement.id) &&
        nonempty(row.replacement.kind) &&
        nonempty(row.replacement.requirement) &&
        nonempty(row.replacement.status),
      `${row.id}: replacement mapping`,
    )
    requireCondition(row.reviewRequired === false, `${row.id}: unreviewed classification`)
  }
  for (const row of value.behaviors) {
    requireCondition(
      phases.includes(row.phase) && nonempty(row.owner) && nonempty(row.requirement),
      `${row.id}: behavior phase/owner/requirement`,
    )
    requireCondition(
      ['pending', 'mapped-unverified', 'verified'].includes(row.status),
      `${row.id}: behavior status`,
    )
    requireCondition(
      Array.isArray(row.evidence) &&
        (row.status === 'pending' ? nonempty(row.plannedAssertion) : row.evidence.length > 0),
      `${row.id}: behavior assertion mapping`,
    )
    if (row.status === 'verified')
      requireCondition(nonempty(row.verifiedRun), `${row.id}: verified evidence missing`)
    for (const evidence of row.evidence)
      requireCondition(
        nonempty(evidence.source) &&
          nonempty(evidence.scenario) &&
          nonempty(evidence.assertion) &&
          /^[a-f\d]{16}$/.test(evidence.fingerprint),
        `${row.id}: malformed assertion evidence`,
      )
  }
  requireCondition(
    sameStrings(
      value.quotas.map((row) => row.phase),
      phases,
    ),
    'every phase must have exactly one quota',
  )
  for (const row of value.quotas) {
    const retirementIds = value.constraints
      .filter((item) => item.disposition === 'retire' && item.phase === row.phase)
      .map((item) => item.id)
    const behaviorIds = value.behaviors
      .filter((item) => item.phase === row.phase)
      .map((item) => item.id)
    requireCondition(
      row.R_p === retirementIds.length && sameStrings(row.retirementIds, retirementIds),
      `${row.phase}: R_p/retirement IDs disagree`,
    )
    requireCondition(
      row.B_p === behaviorIds.length && sameStrings(row.behaviorIds, behaviorIds),
      `${row.phase}: B_p/behavior IDs disagree`,
    )
    requireCondition(
      row.R_p > 0 || nonempty(row.zeroRetirementReason),
      `${row.phase}: zero retirement scope reason`,
    )
    requireCondition(
      row.B_p > 0 || nonempty(row.zeroBehaviorReason),
      `${row.phase}: zero behavior scope reason`,
    )
  }
  for (const disposition of dispositions)
    requireCondition(
      value.counts[disposition] ===
        value.constraints.filter((row) => row.disposition === disposition).length,
      `${disposition}: measured count mismatch`,
    )
  requireCondition(
    sameStrings(value.method.sourceFiles, [...new Set(value.constraints.map((row) => row.source))]),
    'source file inventory',
  )
  requireCondition(
    value.budget.packages.length === 8 &&
      value.budget.packages.reduce((sum, row) => sum + row.plannedNetHours, 0) ===
        value.budget.totalPlannedNetHours,
    'P1b budget totals',
  )
  uniqueIds(value.budget.packages, 'P1b packages')
  for (const row of value.budget.packages)
    requireCondition(
      nonempty(row.scope) &&
        nonempty(row.owner) &&
        row.plannedNetHours > 0 &&
        value.budget.plannedWorkingDays.includes(row.plannedCompletionDate) &&
        /^[a-f\d]{40}$/.test(row.checkpointSha),
      `${row.id}: package scope/owner/schedule/checkpoint`,
    )
  requireCondition(
    nonempty(value.budget.authorization) && nonempty(value.budget.killSwitch),
    'P1 authorization/Kill-switch',
  )
  requireCondition(
    value.checkpoints.immutableRollbackSha === null || nonempty(value.checkpoints.ciEvidence),
    'rollback SHA needs CI evidence',
  )
  requireCondition(
    value.generatedMapping.sourcePattern === 'src/shared/api/generated-query/**' &&
      value.generatedMapping.targetPattern === 'src/api/generated-query/**' &&
      /^[a-f\d]{64}$/.test(value.generatedMapping.sourcePathSha256),
    'generated carve-out',
  )
  requireCondition(
    new Set(value.sharedFileMappings.map((row) => row.source)).size ===
      value.sharedFileMappings.length,
    'duplicate shared source mapping',
  )
  for (const row of value.sharedFileMappings)
    requireCondition(
      nonempty(row.source) &&
        row.targets.length > 0 &&
        row.targets.every(nonempty) &&
        nonempty(row.owner) &&
        nonempty(row.evidence) &&
        row.consumerEvidence &&
        nonempty(row.consumerEvidence.kind),
      `${row.source}: source/target ownership mapping`,
    )
  requireCondition(
    value.contextMappings.length === 11 &&
      value.contextMappings.every(
        (row) => nonempty(row.source) && nonempty(row.target) && nonempty(row.action),
      ),
    'canonical CONTEXT mappings',
  )
  requireCondition(
    value.pathMappings.every(
      (row) =>
        nonempty(row.source) &&
        nonempty(row.target) &&
        nonempty(row.owner) &&
        nonempty(row.requirement),
    ),
    'directory/file mappings',
  )
  return value
}
function collectAssertions(file: string, source: string): Assertion[] {
  const parsed = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true)
  const printer = ts.createPrinter({ removeComments: true })
  const rows: Assertion[] = []
  const occurrences = new Map<string, number>()
  function visit(node: ts.Node, scenario = '') {
    if (ts.isCallExpression(node) && node.arguments[0]) {
      const name = node.expression.getText(parsed)
      const isTest =
        /^(?:it|test)(?:\.(?:only|skip))?$/.test(name) ||
        (ts.isCallExpression(node.expression) &&
          /^(?:it|test)\.each$/.test(node.expression.expression.getText(parsed)))
      if (isTest)
        scenario = ts.isStringLiteralLike(node.arguments[0])
          ? node.arguments[0].text
          : node.arguments[0].getText(parsed)
    }
    if (
      ts.isCallExpression(node) &&
      ts.isPropertyAccessExpression(node.expression) &&
      /^expect\(/.test(node.expression.getText(parsed))
    ) {
      const assertion = printer.printNode(ts.EmitHint.Expression, node, parsed)
      const fingerprint = hash(`${file}\n${scenario}\n${assertion}`)
      const occurrence = (occurrences.get(fingerprint) ?? 0) + 1
      occurrences.set(fingerprint, occurrence)
      rows.push({
        id: `source-${fingerprint}-${occurrence}`,
        source: file,
        line: parsed.getLineAndCharacterOfPosition(node.getStart(parsed)).line + 1,
        scenario,
        assertion,
        fingerprint,
        occurrence,
      })
    }
    ts.forEachChild(node, (child) => visit(child, scenario))
  }
  visit(parsed)
  return rows
}
function listFiles(directory: string): string[] {
  return fs
    .readdirSync(directory, { withFileTypes: true })
    .flatMap((entry) => {
      const file = path.join(directory, entry.name)
      return entry.isDirectory()
        ? listFiles(file)
        : [path.relative(webRoot, file).split(path.sep).join('/')]
    })
    .sort()
}
function checkCurrentSources(ledger: Ledger): string[] {
  const warnings: string[] = []
  const sources = new Map<string, Assertion[]>()
  for (const file of listFiles(path.join(webRoot, 'src')).filter((file) =>
    file.endsWith('.spec.ts'),
  )) {
    const source = fs.readFileSync(path.join(webRoot, file), 'utf8')
    if (!/readFileSync|readFile\(|\?raw/.test(source)) continue
    sources.set(file, collectAssertions(file, source))
  }
  for (const [file, rows] of sources) {
    if (!ledger.method.sourceFiles.includes(file))
      warnings.push(
        `${file}: new direct-source candidate file; classify every matcher before accepting migration`,
      )
    else
      for (const row of rows)
        if (!ledger.constraints.some((constraint) => constraint.id === row.id))
          warnings.push(
            `${row.id} ${file}:${row.line}: new/changed matcher requires reviewed inventory update`,
          )
  }
  for (const row of ledger.constraints)
    if (!sources.get(row.source)?.some((current) => current.id === row.id))
      warnings.push(
        `${row.id} ${row.source}:${row.line}: frozen matcher is missing/changed; preserve denominator and supply reviewed replacement/relocation evidence`,
      )
  for (const behavior of ledger.behaviors)
    for (const evidence of behavior.evidence) {
      const sourcePath = path.join(webRoot, evidence.source)
      if (
        !fs.existsSync(sourcePath) ||
        !collectAssertions(evidence.source, fs.readFileSync(sourcePath, 'utf8')).some(
          (row) => row.fingerprint === evidence.fingerprint,
        )
      )
        warnings.push(
          `${behavior.id}: mapped behavior assertion missing/changed in ${evidence.source}`,
        )
    }
  const sharedFiles = listFiles(path.join(webRoot, 'src/shared'))
  const generated = sharedFiles.filter((file) => file.includes('/generated-query/'))
  const generatedHash = createHash('sha256').update(generated.join('\n')).digest('hex')
  if (
    generated.length !== ledger.generatedMapping.measuredFileCount ||
    generatedHash !== ledger.generatedMapping.sourcePathSha256
  )
    warnings.push(
      'Generated-query file inventory changed; run generated drift and review parent-path carve-out',
    )
  const ordinary = sharedFiles.filter((file) => !file.includes('/generated-query/'))
  if (
    !sameStrings(
      ordinary,
      ledger.sharedFileMappings.map((row) => row.source),
    )
  )
    warnings.push('Shared source/target mapping does not cover current ordinary files exactly')
  const seams = loadSeamInventory().seams
  for (const row of ledger.sharedFileMappings) {
    if (row.consumerEvidence.kind === 'seam-inventory')
      for (const entry of row.consumerEvidence.entries ?? [])
        if (!seams.some((seam) => seam.path === entry))
          warnings.push(`${row.source}: missing canonical consumer evidence Seam ${entry}`)
    for (const seam of seams.filter(
      (seam) => seam.status === 'planned' && seam.sourcePaths.includes(row.source),
    )) {
      const indexContainsTarget =
        seam.path.endsWith('/index.ts') &&
        row.targets.some((target) => target.startsWith(seam.path.slice(0, -'index.ts'.length)))
      if (!row.targets.includes(seam.path) && !indexContainsTarget)
        warnings.push(`${row.source}: target mapping disagrees with planned Seam ${seam.path}`)
    }
  }
  return warnings
}
function runFixtures(ledger: Ledger): void {
  const valid = () => structuredClone(ledger)
  validateLedger(valid())
  const invalid: Array<[string, (value: Ledger) => void]> = [
    [
      'duplicate constraint',
      (value) => {
        value.constraints.push({ ...value.constraints[0]! })
      },
    ],
    [
      'missing replacement',
      (value) => {
        value.constraints[0]!.replacement.requirement = ''
      },
    ],
    [
      'quota mismatch',
      (value) => {
        value.quotas[0]!.R_p += 1
      },
    ],
    [
      'unexplained zero quota',
      (value) => {
        value.quotas.find((row) => row.R_p === 0)!.zeroRetirementReason = null
      },
    ],
    [
      'unproved verified behavior',
      (value) => {
        const behavior = value.behaviors.find((row) => row.evidence.length > 0)!
        behavior.status = 'verified'
        behavior.verifiedRun = null
      },
    ],
    [
      'candidate silently assigned',
      (value) => {
        value.constraints.find((row) => row.disposition === 'frozen-i18n-candidate')!.phase = 'P5'
      },
    ],
    [
      'budget mismatch',
      (value) => {
        value.budget.totalPlannedNetHours += 1
      },
    ],
    [
      'rollback without CI',
      (value) => {
        value.checkpoints.immutableRollbackSha = value.checkpointSha
      },
    ],
    [
      'missing source destination',
      (value) => {
        value.sharedFileMappings[0]!.targets = []
      },
    ],
  ]
  for (const [name, mutate] of invalid) {
    const value = valid()
    mutate(value)
    let rejected = false
    try {
      validateLedger(value)
    } catch {
      rejected = true
    }
    requireCondition(rejected, `fixture did not reject ${name}`)
  }
  const parsed = collectAssertions(
    'src/example.contract.spec.ts',
    'it.each([1])("case %s", () => { expect(source).toContain("a"); expect(source).toContain("a") })',
  )
  requireCondition(
    parsed.length === 2 && parsed[0]!.scenario === 'case %s' && parsed[0]!.id !== parsed[1]!.id,
    'AST table/duplicate identity fixture',
  )
  const template = collectAssertions(
    'src/example.contract.spec.ts',
    'it.each([1])(`case ${value}`, () => { expect(source).toContain("a") })',
  )
  requireCondition(template[0]!.scenario === '`case ${value}`', 'AST template title fixture')
  console.log(`Frontend migration ledger fixtures passed (${invalid.length + 3} cases).`)
}
function main(): void {
  const markdown = fs.readFileSync(inventoryPath, 'utf8')
  const block = markdown.match(/```json migration-ledger\s*([\s\S]*?)```/)
  requireCondition(block, 'missing canonical JSON migration-ledger block')
  const ledger = validateLedger(JSON.parse(block[1]!))
  runFixtures(ledger)
  if (process.argv.includes('--self-test')) return
  const warnings = checkCurrentSources(ledger)
  for (const warning of warnings) console.warn(`[migration-ledger:review] ${warning}`)
  const retire = ledger.constraints.filter((row) => row.disposition === 'retire').length
  const frozen = ledger.constraints.filter(
    (row) => row.disposition === 'frozen-i18n-candidate',
  ).length
  console.log(
    `Frontend migration ledger: ${ledger.constraints.length} constraints; R=${retire}, frozen i18n=${frozen}; B=${ledger.behaviors.length}; ${ledger.sharedFileMappings.length} ordinary + ${ledger.generatedMapping.measuredFileCount} generated files mapped; ${warnings.length} review warnings.`,
  )
  console.log(
    'P0 ledger integrity is not migration acceptance: planned replacements/scenarios remain pending; P1 is not authorized.',
  )
  if (process.argv.includes('--strict') && warnings.length > 0) process.exitCode = 1
}
try {
  main()
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error))
  process.exitCode = 1
}
