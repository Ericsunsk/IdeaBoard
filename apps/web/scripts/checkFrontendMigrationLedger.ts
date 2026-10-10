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
const frozenConstraintIdsSha256 = '4b7319de4da664fb6db10248129013a8ae521415650001773df48eba78391627'
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
    evidence?: Array<Omit<Assertion, 'id' | 'occurrence'>>
    verifiedRun?: string
  }
  reviewRequired: boolean
  currentAssertion?: { assertion: string; reason: string; source?: string; scenario?: string }
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
  supplementalConstraints?: Constraint[]
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
  const frozenIdsHash = createHash('sha256')
    .update(
      value.constraints
        .map((row) => row.id)
        .sort()
        .join('\n'),
    )
    .digest('hex')
  requireCondition(frozenIdsHash === frozenConstraintIdsSha256, 'P0 frozen constraint IDs changed')
  uniqueIds([...value.constraints, ...(value.supplementalConstraints ?? [])], 'all constraints')
  for (const row of [...value.constraints, ...(value.supplementalConstraints ?? [])]) {
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
        row.fingerprint === hash(`${row.source}\n${row.scenario}\n${row.assertion}`) &&
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
    if (row.currentAssertion)
      requireCondition(
        nonempty(row.currentAssertion.assertion) && nonempty(row.currentAssertion.reason),
        `${row.id}: reviewed current assertion mapping`,
      )
    if (row.currentAssertion?.source !== undefined || row.currentAssertion?.scenario !== undefined)
      requireCondition(
        nonempty(row.currentAssertion.source) &&
          row.currentAssertion.source.startsWith('src/') &&
          path.posix.normalize(row.currentAssertion.source) === row.currentAssertion.source &&
          !row.currentAssertion.source.includes('\\') &&
          nonempty(row.currentAssertion.scenario),
        `${row.id}: reviewed current assertion source/scenario mapping`,
      )
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
  requireCondition(value.counts.retire === 403, 'P0 retirement denominator must remain 403')
  requireCondition(
    value.quotas.find((row) => row.phase === 'P1b')?.R_p === 149,
    'P1b frozen retirement quota must remain 149',
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
      value.generatedMapping.measuredFileCount === 417 &&
      value.generatedMapping.sourcePathSha256 ===
        '883c1869b262fd82359e8c7a76be0ebf4aa445206a8a0dc80a8bd037f0fdb711',
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
      /^expect(?:\.poll)?\(/.test(node.expression.getText(parsed))
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
function mappedAssertionId(row: Constraint): string {
  const assertion = row.currentAssertion?.assertion ?? row.assertion
  const source = row.currentAssertion?.source ?? row.source
  const scenario = row.currentAssertion?.scenario ?? row.scenario
  return `source-${hash(`${source}\n${scenario}\n${assertion}`)}-${row.occurrence}`
}
function canonicalSource(file: string, ledger: Ledger): string {
  const exact = ledger.sharedFileMappings.find(
    (row) => row.disposition !== 'retired-effective-replacement' && row.targets.includes(file),
  )
  const relocation = ledger.pathMappings.find(
    (row) => row.status === 'done' && row.target === file && row.source.endsWith('.spec.ts'),
  )
  return exact?.source ?? relocation?.source ?? file
}
function projectGeneratedFiles(files: string[], mapping: Ledger['generatedMapping']): string[] {
  const sourceRoot = mapping.sourcePattern.slice(0, -2)
  const targetRoot = mapping.targetPattern.slice(0, -2)
  return files
    .map((file) =>
      file.startsWith(targetRoot) ? `${sourceRoot}${file.slice(targetRoot.length)}` : file,
    )
    .sort()
}
function generatedInventoryMatches(files: string[], mapping: Ledger['generatedMapping']): boolean {
  const projected = projectGeneratedFiles(files, mapping)
  return (
    projected.length === mapping.measuredFileCount &&
    createHash('sha256').update(projected.join('\n')).digest('hex') === mapping.sourcePathSha256
  )
}
function checkFileMappings(ledger: Ledger): string[] {
  const warnings: string[] = []
  const sourceRoot = ledger.generatedMapping.sourcePattern.slice(0, -3)
  const targetRoot = ledger.generatedMapping.targetPattern.slice(0, -3)
  const generatedRoot = fs.existsSync(path.join(webRoot, targetRoot)) ? targetRoot : sourceRoot
  const generated = fs.existsSync(path.join(webRoot, generatedRoot))
    ? listFiles(path.join(webRoot, generatedRoot))
    : []
  if (
    generatedRoot === targetRoot &&
    fs.existsSync(path.join(webRoot, sourceRoot)) &&
    listFiles(path.join(webRoot, sourceRoot)).length > 0
  )
    warnings.push('Generated-query files remain at both source and relocation target')
  if (!generatedInventoryMatches(generated, ledger.generatedMapping))
    warnings.push(
      'Generated-query file inventory changed; run generated drift and review parent-path carve-out',
    )
  for (const row of ledger.sharedFileMappings)
    for (const target of row.targets)
      if (!fs.existsSync(path.join(webRoot, target)))
        warnings.push(`${row.source}: mapped destination missing ${target}`)
  const sharedRoot = path.join(webRoot, 'src/shared')
  const ordinary = (fs.existsSync(sharedRoot) ? listFiles(sharedRoot) : []).filter(
    (file) => !file.includes('/generated-query/') && !file.endsWith('.shim.ts'),
  )
  for (const file of ordinary)
    if (!ledger.sharedFileMappings.some((row) => row.source === file || row.targets.includes(file)))
      warnings.push(`${file}: ordinary shared source has no exact ownership mapping`)
  return warnings
}
function hasAssertionEvidence(evidence: Omit<Assertion, 'id' | 'occurrence'>): boolean {
  const sourcePath = path.join(webRoot, evidence.source)
  return (
    fs.existsSync(sourcePath) &&
    collectAssertions(evidence.source, fs.readFileSync(sourcePath, 'utf8')).some(
      (row) => row.fingerprint === evidence.fingerprint,
    )
  )
}
function collectCurrentSources(ledger: Ledger): Map<string, Assertion[]> {
  const sources = new Map<string, Assertion[]>()
  const inventoriedSources = new Set([
    ...ledger.method.sourceFiles,
    ...(ledger.supplementalConstraints ?? []).map((row) => row.source),
  ])
  for (const file of listFiles(path.join(webRoot, 'src')).filter((file) =>
    file.endsWith('.spec.ts'),
  )) {
    const source = fs.readFileSync(path.join(webRoot, file), 'utf8')
    const canonical = canonicalSource(file, ledger)
    if (!/readFileSync|readFile\(|\?raw/.test(source) && !inventoriedSources.has(canonical))
      continue
    sources.set(canonical, collectAssertions(canonical, source))
  }
  return sources
}
function hasCurrentAssertion(row: Constraint, sources: Map<string, Assertion[]>): boolean {
  const source = row.currentAssertion?.source ?? row.source
  if (sources.get(source)?.some((current) => current.id === mappedAssertionId(row))) return true
  if (!row.currentAssertion?.source || !fs.existsSync(path.join(webRoot, source))) return false
  return collectAssertions(source, fs.readFileSync(path.join(webRoot, source), 'utf8')).some(
    (current) => current.id === mappedAssertionId(row),
  )
}
function checkCurrentSources(ledger: Ledger, sources: Map<string, Assertion[]>): string[] {
  const warnings = checkFileMappings(ledger)
  const constraints = [...ledger.constraints, ...(ledger.supplementalConstraints ?? [])]
  for (const [file, rows] of sources) {
    if (!constraints.some((row) => row.source === file))
      warnings.push(
        `${file}: new direct-source candidate file; classify every matcher before accepting migration`,
      )
    else
      for (const row of rows)
        if (!constraints.some((constraint) => mappedAssertionId(constraint) === row.id))
          warnings.push(
            `${row.id} ${file}:${row.line}: new/changed matcher requires reviewed inventory update`,
          )
  }
  for (const row of constraints)
    if (row.retirementStatus !== 'retired' && !hasCurrentAssertion(row, sources))
      warnings.push(
        `${row.id} ${row.source}:${row.line}: frozen matcher is missing/changed; preserve denominator and supply reviewed replacement/relocation evidence`,
      )
  for (const behavior of ledger.behaviors)
    for (const evidence of behavior.evidence) {
      if (!hasAssertionEvidence(evidence))
        warnings.push(
          `${behavior.id}: mapped behavior assertion missing/changed in ${evidence.source}`,
        )
    }
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
function phaseAcceptanceFailures(
  ledger: Ledger,
  sources = collectCurrentSources(ledger),
  phase: 'P1' | 'P2' = 'P1',
): string[] {
  const failures: string[] = []
  const acceptedPhases = phase === 'P1' ? ['P1a', 'P1b'] : ['P1a', 'P1b', 'P2']
  for (const row of [...ledger.constraints, ...(ledger.supplementalConstraints ?? [])].filter(
    (row) => row.disposition === 'retire' && acceptedPhases.includes(row.phase ?? ''),
  )) {
    if (row.retirementStatus !== 'retired') failures.push(`${row.id}: retirement pending`)
    if (
      row.replacement.status !== 'verified' ||
      !nonempty(row.replacement.verifiedRun) ||
      !row.replacement.evidence?.length ||
      !row.replacement.evidence.every(hasAssertionEvidence)
    )
      failures.push(`${row.id}: effective replacement evidence pending`)
    if (hasCurrentAssertion(row, sources))
      failures.push(`${row.id}: original implementation-source matcher still present`)
  }
  for (const row of ledger.behaviors.filter((row) => acceptedPhases.includes(row.phase)))
    if (
      row.status !== 'verified' ||
      !nonempty(row.verifiedRun) ||
      !row.evidence.length ||
      !row.evidence.every(hasAssertionEvidence)
    )
      failures.push(`${row.id}: mandatory behavior verification pending`)
  return failures
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
    [
      'frozen ID rehash',
      (value) => {
        value.constraints[0]!.id = 'source-0000000000000000-1'
        value.constraints[0]!.fingerprint = '0000000000000000'
      },
    ],
    [
      'retirement rescope',
      (value) => {
        const row = value.constraints.find(
          (row) => row.phase === 'P1b' && row.disposition === 'retire',
        )!
        row.disposition = 'retain-behavior'
        value.counts.retire! -= 1
        value.counts['retain-behavior']! += 1
        const quota = value.quotas.find((row) => row.phase === 'P1b')!
        quota.R_p -= 1
        quota.retirementIds = quota.retirementIds.filter((id) => id !== row.id)
      },
    ],
    [
      'empty old generated source rebaseline',
      (value) => {
        value.generatedMapping.measuredFileCount = 0
        value.generatedMapping.sourcePathSha256 = createHash('sha256').update('').digest('hex')
      },
    ],
    [
      'incomplete assertion relocation',
      (value) => {
        value.constraints[0]!.currentAssertion = {
          assertion: value.constraints[0]!.assertion,
          reason: 'reviewed relocation fixture',
          source: 'src/example.browser.spec.ts',
        }
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
  const polled = collectAssertions(
    'src/example.browser.spec.ts',
    'it("layout", async () => { await expect.poll(() => layout(2)).toBeUndefined() })',
  )
  requireCondition(
    polled.length === 1 && polled[0]!.scenario === 'layout',
    'AST polled browser assertion fixture',
  )
  const relocatedAssertion = {
    ...ledger.constraints[0]!,
    currentAssertion: {
      source: polled[0]!.source,
      scenario: polled[0]!.scenario,
      assertion: polled[0]!.assertion,
      reason: 'reviewed observable layout relocation',
    },
  }
  requireCondition(
    mappedAssertionId(relocatedAssertion) === polled[0]!.id &&
      hasCurrentAssertion(relocatedAssertion, new Map([[polled[0]!.source, polled]])),
    'reviewed observable source/scenario relocation resolves the exact matcher',
  )
  const mapping = {
    ...ledger.generatedMapping,
    measuredFileCount: 1,
    sourcePathSha256: createHash('sha256')
      .update('src/shared/api/generated-query/example.ts')
      .digest('hex'),
  }
  requireCondition(
    generatedInventoryMatches(['src/api/generated-query/example.ts'], mapping),
    'generated relocated target preserves source-path hash',
  )
  requireCondition(
    !generatedInventoryMatches([], mapping),
    'empty old generated source cannot pass',
  )
  requireCondition(
    !generatedInventoryMatches(['src/api/generated-query/changed.ts'], mapping),
    'changed generated target rejects frozen path hash',
  )
  requireCondition(
    canonicalSource('src/core/example.spec.ts', {
      ...ledger,
      sharedFileMappings: [
        {
          ...ledger.sharedFileMappings[0]!,
          source: 'src/shared/example.spec.ts',
          targets: ['src/core/example.spec.ts'],
        },
      ],
    }) === 'src/shared/example.spec.ts',
    'exact relocated matcher keeps canonical source identity',
  )
  const retiredMapping = {
    ...ledger.sharedFileMappings[0]!,
    source: 'src/shared/retired.spec.ts',
    targets: ['src/test/replacement.browser.spec.ts'],
    disposition: 'retired-effective-replacement',
  }
  requireCondition(
    canonicalSource('src/test/replacement.browser.spec.ts', {
      ...ledger,
      sharedFileMappings: [retiredMapping],
    }) === 'src/test/replacement.browser.spec.ts',
    'replacement evidence keeps its own identity after source retirement',
  )
  requireCondition(
    canonicalSource('src/test/replacement.browser.spec.ts', {
      ...ledger,
      sharedFileMappings: [{ ...retiredMapping, disposition: 'relocated' }],
    }) === 'src/shared/retired.spec.ts',
    'ordinary relocation must still project the original matcher identity',
  )
  requireCondition(
    canonicalSource('src/features/example/ui/example.unit.spec.ts', {
      ...ledger,
      pathMappings: [
        {
          source: 'src/features/example/components/example.unit.spec.ts',
          target: 'src/features/example/ui/example.unit.spec.ts',
          phase: 'P2',
          owner: 'fixture',
          requirement: 'exact relocation',
          status: 'done',
        },
      ],
    }) === 'src/features/example/components/example.unit.spec.ts',
    'exact feature relocation preserves frozen matcher identity',
  )
  const pending = valid()
  const pendingRetirement = pending.constraints.find(
    (row) => row.disposition === 'retire' && row.phase === 'P1b',
  )!
  pendingRetirement.retirementStatus = 'not-started'
  pendingRetirement.replacement.status = 'not-implemented'
  const pendingBehavior = pending.behaviors.find((row) => row.phase === 'P1a')!
  pendingBehavior.status = 'pending'
  const pendingSources = new Map([
    [
      pendingRetirement.source,
      [{ ...pendingRetirement, id: mappedAssertionId(pendingRetirement) }],
    ],
  ])
  const acceptance = phaseAcceptanceFailures(pending, pendingSources)
  requireCondition(
    acceptance.some((message) => message.includes('retirement pending')),
    'P1 rejects pending retirement',
  )
  requireCondition(
    acceptance.some((message) => message.includes('replacement evidence pending')),
    'P1 rejects unverified replacement',
  )
  requireCondition(
    acceptance.some((message) => message.includes('matcher still present')),
    'P1 rejects relocated source matchers still present',
  )
  requireCondition(
    acceptance.some((message) => message.includes('behavior verification pending')),
    'P1 rejects pending mandatory behavior',
  )
  const completed = valid()
  const existingEvidence = ledger.behaviors.find(
    (row) => row.status === 'verified' && row.evidence.every(hasAssertionEvidence),
  )!.evidence
  function completeFixturePhase(value: Ledger, phases: string[], verifiedRun: string) {
    for (const row of [...value.constraints, ...(value.supplementalConstraints ?? [])]) {
      if (row.disposition !== 'retire' || !phases.includes(row.phase ?? '')) continue
      row.retirementStatus = 'retired'
      row.replacement = {
        ...row.replacement,
        status: 'verified',
        evidence: existingEvidence,
        verifiedRun,
      }
    }
    for (const row of value.behaviors.filter((row) => phases.includes(row.phase))) {
      row.status = 'verified'
      row.evidence = existingEvidence
      row.verifiedRun = verifiedRun
    }
  }
  completeFixturePhase(completed, ['P1a', 'P1b'], 'synthetic phase-gate positive fixture')
  requireCondition(
    phaseAcceptanceFailures(completed, new Map()).length === 0,
    'P1 completed retirement/replacement/behavior positive fixture',
  )
  const p2Completed = structuredClone(completed)
  completeFixturePhase(p2Completed, ['P2'], 'synthetic P2 phase-gate positive fixture')
  requireCondition(
    phaseAcceptanceFailures(p2Completed, new Map(), 'P2').length === 0,
    'P2 accepts completed scopes while future phases remain pending',
  )
  const p2Invalid: Array<
    [string, (value: Ledger, sources: Map<string, Assertion[]>) => void, string]
  > = [
    [
      'pending retirement',
      (value) => {
        value.constraints.find(
          (row) => row.disposition === 'retire' && row.phase === 'P2',
        )!.retirementStatus = 'not-started'
      },
      'retirement pending',
    ],
    [
      'unverified replacement',
      (value) => {
        value.constraints.find(
          (row) => row.disposition === 'retire' && row.phase === 'P2',
        )!.replacement.status = 'not-implemented'
      },
      'replacement evidence pending',
    ],
    [
      'original matcher',
      (value, sources) => {
        const row = value.constraints.find(
          (row) => row.disposition === 'retire' && row.phase === 'P2',
        )!
        sources.set(row.source, [{ ...row, id: mappedAssertionId(row) }])
      },
      'matcher still present',
    ],
    [
      'reviewed relocated original matcher',
      (value, sources) => {
        const row = value.constraints.find(
          (row) => row.disposition === 'retire' && row.phase === 'P2',
        )!
        row.currentAssertion = {
          source: polled[0]!.source,
          scenario: polled[0]!.scenario,
          assertion: polled[0]!.assertion,
          reason: 'reviewed relocation fixture',
        }
        sources.set(polled[0]!.source, [{ ...polled[0]!, id: mappedAssertionId(row) }])
      },
      'matcher still present',
    ],
    [
      'nonexistent replacement evidence',
      (value) => {
        value.constraints.find(
          (row) => row.disposition === 'retire' && row.phase === 'P2',
        )!.replacement.evidence![0]!.source = 'src/nonexistent-evidence.spec.ts'
      },
      'replacement evidence pending',
    ],
    [
      'pending behavior',
      (value) => {
        value.behaviors.find((row) => row.phase === 'P2')!.status = 'pending'
      },
      'behavior verification pending',
    ],
    [
      'nonexistent behavior evidence',
      (value) => {
        value.behaviors.find((row) => row.phase === 'P2')!.evidence[0]!.source =
          'src/nonexistent-evidence.spec.ts'
      },
      'behavior verification pending',
    ],
  ]
  for (const [name, mutate, expected] of p2Invalid) {
    const value = structuredClone(p2Completed)
    const sources = new Map<string, Assertion[]>()
    mutate(value, sources)
    requireCondition(
      phaseAcceptanceFailures(value, sources, 'P2').some((message) => message.includes(expected)),
      `P2 rejects ${name}`,
    )
  }
  console.log(
    `Frontend migration ledger fixtures passed (${invalid.length + 18 + p2Invalid.length} cases).`,
  )
}
function main(): void {
  const markdown = fs.readFileSync(inventoryPath, 'utf8')
  const block = markdown.match(/```json migration-ledger\s*([\s\S]*?)```/)
  requireCondition(block, 'missing canonical JSON migration-ledger block')
  const ledger = validateLedger(JSON.parse(block[1]!))
  runFixtures(ledger)
  if (process.argv.includes('--self-test')) return
  const sources = collectCurrentSources(ledger)
  const warnings = checkCurrentSources(ledger, sources)
  for (const warning of warnings) console.warn(`[migration-ledger:review] ${warning}`)
  const retire = ledger.constraints.filter((row) => row.disposition === 'retire').length
  const frozen = ledger.constraints.filter(
    (row) => row.disposition === 'frozen-i18n-candidate',
  ).length
  console.log(
    `Frontend migration ledger: ${ledger.constraints.length} frozen constraints; R=${retire}, frozen i18n=${frozen}; B=${ledger.behaviors.length}; ${ledger.sharedFileMappings.length} ordinary + ${ledger.generatedMapping.measuredFileCount} generated files mapped; ${ledger.supplementalConstraints?.length ?? 0} supplemental constraints; ${warnings.length} review warnings.`,
  )
  console.log(
    'Ledger consistency is not phase acceptance. Use --phase=P1 or --phase=P2 to require completed retirements, effective replacements and mandatory behaviors through that phase.',
  )
  const phase = process.argv.find((argument) => argument.startsWith('--phase='))
  requireCondition(
    phase === undefined || phase === '--phase=P1' || phase === '--phase=P2',
    'supported acceptance phases are P1 and P2',
  )
  if (phase) {
    const acceptedPhase = phase === '--phase=P2' ? 'P2' : 'P1'
    const failures = phaseAcceptanceFailures(ledger, sources, acceptedPhase)
    for (const failure of failures) console.error(`[migration-ledger:acceptance] ${failure}`)
    console.log(`${acceptedPhase} acceptance: ${failures.length} unmet requirements.`)
    if (failures.length > 0) process.exitCode = 1
  }
  if ((process.argv.includes('--strict') || phase !== undefined) && warnings.length > 0)
    process.exitCode = 1
}
try {
  main()
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error))
  process.exitCode = 1
}
