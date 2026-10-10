import fs from 'node:fs'
import assert from 'node:assert/strict'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

/**
 * Shared loader for the P0 seam inventory (`docs/frontend-seam-inventory.md`).
 *
 * The inventory's `json seam-inventory` fenced block is the single
 * machine/human source of truth (plan v4 §5.4 A). Both
 * `checkDependencyBoundaries` and `checkPublicApiSurface` read it from here
 * instead of hard-coding seam paths (`knip.jsonc` `entry` is no longer an
 * input to either check). Schema drift fails loudly so tables can never
 * silently diverge from the checks.
 */

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const SEAM_INVENTORY_PATH = path.join(webRoot, '..', '..', 'docs', 'frontend-seam-inventory.md')

type SeamKind = 'canonical' | 'cascading'
type SeamStatus = 'planned' | 'active' | 'retired'

export type SeamRecord = {
  path: string
  kind: SeamKind
  confidence: 'high' | 'low'
  status: SeamStatus
  owner: string
  activationPhase: string
  scope: string
  exportStyle: string
  surfaceBaseline: boolean
  allowedConsumers: ConsumerEdge[]
  measuredFanIn: number
  measuredFanOut: number
  targetFanIn: number
  targetFanOut: number
  shims: string[]
  shimTtl: string
  retirementPhase: string | null
  deletionOwner: string | null
  notes: string
  sourcePaths: string[]
  targetPath: string
  ownershipScope: string[]
  publicExports: string[]
  targetFanOutTargets: string[]
  surfaceSeverity: 'warn' | 'error'
  sourceOwnershipScope?: string[]
  exportMappings?: Array<{ source: string; name: string; exportedAs: string }>
  purity?: 'pure'
}

export type ConsumerEdge = { from: string; to: string; kind: 'runtime' | 'type-only' }
export type CruiseModule = {
  source: string
  dependencies?: Array<{ resolved?: string; dependencyTypes?: string[] }>
}

export type SeamInventory = {
  version: number
  checkpointSha: string
  countingRule: string
  defaults: { shimTtlPhases: number; shimNote: string }
  seams: SeamRecord[]
}

function fail(problem: string): never {
  throw new Error(`Seam inventory invalid (${SEAM_INVENTORY_PATH}): ${problem}`)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

/** Extract the machine source block; nothing else in the doc is parsed. */
function readInventoryBlock(source?: string): SeamInventory {
  const markdown = source ?? fs.readFileSync(SEAM_INVENTORY_PATH, 'utf8')
  const matches = [...markdown.matchAll(/```json seam-inventory\r?\n([\s\S]*?)\r?\n```/g)]
  if (matches.length !== 1) fail('expected exactly one ```json seam-inventory fenced block')
  return JSON.parse(matches[0]?.[1] ?? '{}') as SeamInventory
}

function validateStrings(value: unknown, label: string): asserts value is string[] {
  if (!Array.isArray(value) || value.some((entry) => typeof entry !== 'string' || !entry))
    fail(`${label} must contain nonempty strings`)
  if (
    new Set(value).size !== value.length ||
    value.some((entry, index) => index > 0 && entry < value[index - 1])
  )
    fail(`${label} must be unique and sorted`)
}

function validatePath(value: unknown): void {
  if (typeof value !== 'string' || !value.startsWith('src/') || /\.\.|\\|\/\//.test(value))
    fail(`invalid source path ${String(value)}`)
}

function validateContract(record: SeamRecord): void {
  if (record.purity !== undefined && record.purity !== 'pure')
    fail(`${record.path}: invalid purity`)
  validatePath(record.path)
  validatePath(record.targetPath)
  if (record.targetPath !== record.path) fail(`${record.path}: targetPath must equal path`)
  for (const field of [
    'sourcePaths',
    'ownershipScope',
    'publicExports',
    'targetFanOutTargets',
    'shims',
  ] as const)
    validateStrings(record[field], `${record.path}.${field}`)
  for (const field of ['sourcePaths', 'ownershipScope'] as const) {
    if (!record[field].length) fail(`${record.path}: ${field} cannot be empty`)
    record[field].forEach(validatePath)
  }
  if (record.kind === 'canonical' && !record.surfaceBaseline)
    fail(`${record.path}: canonical surface coverage missing`)
  if (record.surfaceSeverity !== 'warn' && record.surfaceSeverity !== 'error')
    fail(`${record.path}: invalid surfaceSeverity`)
  for (const field of ['owner', 'activationPhase', 'scope', 'exportStyle', 'shimTtl'] as const)
    if (typeof record[field] !== 'string' || !record[field] || /todo|tbd/i.test(record[field]))
      fail(`${record.path}: ${field} must be frozen`)
  if (typeof record.notes !== 'string') fail(`${record.path}: notes must be a string`)
  const keys: string[] = []
  for (const edge of record.allowedConsumers) {
    if (!isRecord(edge)) fail(`${record.path}: invalid consumer edge`)
    validatePath(edge.from)
    validatePath(edge.to)
    if (edge.to !== record.path || !['runtime', 'type-only'].includes(edge.kind))
      fail(`${record.path}: invalid consumer target/kind`)
    keys.push(edgeKey(edge))
  }
  validateStrings(keys, `${record.path}.allowedConsumers`)
  if (new Set(record.allowedConsumers.map((edge) => edge.from)).size !== record.targetFanIn)
    fail(`${record.path}: targetFanIn disagrees with consumers`)
  if (record.targetFanOutTargets.length !== record.targetFanOut)
    fail(`${record.path}: targetFanOut disagrees with targets`)
  for (const field of ['retirementPhase', 'deletionOwner'] as const)
    if (record[field] !== null && (typeof record[field] !== 'string' || !record[field]))
      fail(`${record.path}: invalid ${field}`)
  if ((record.retirementPhase !== null || record.shims.length > 0) && !record.deletionOwner)
    fail(`${record.path}: retirement/shims need a deletionOwner`)
  if (record.status === 'planned') {
    validateStrings(record.sourceOwnershipScope, `${record.path}.sourceOwnershipScope`)
    if (!record.sourceOwnershipScope.length)
      fail(`${record.path}: source ownership cannot be empty`)
    record.sourceOwnershipScope.forEach(validatePath)
    if (!Array.isArray(record.exportMappings)) fail(`${record.path}: exportMappings missing`)
    const mappingKeys: string[] = []
    for (const mapping of record.exportMappings) {
      if (
        !isRecord(mapping) ||
        !record.sourcePaths.includes(mapping.source) ||
        typeof mapping.name !== 'string' ||
        !mapping.name ||
        typeof mapping.exportedAs !== 'string' ||
        !mapping.exportedAs
      )
        fail(`${record.path}: invalid export mapping`)
      mappingKeys.push(`${mapping.source}|${mapping.name}`)
    }
    validateStrings(mappingKeys, `${record.path}.exportMappings`)
    const mappedNames = [
      ...new Set(record.exportMappings.map((mapping) => mapping.exportedAs)),
    ].sort()
    if (JSON.stringify(mappedNames) !== JSON.stringify(record.publicExports))
      fail(`${record.path}: exports disagree with mappings`)
  }
}

/** Structural validation shared by every consumer of the inventory. */
function validateInventory(inventory: SeamInventory): SeamRecord[] {
  if (!isRecord(inventory)) fail('top-level value is not an object')
  if (inventory.version !== 2) fail(`version is ${String(inventory.version)}, expected 2`)
  if (!/^[a-f0-9]{40}$/.test(inventory.checkpointSha)) fail('checkpointSha must be a full Git SHA')
  if (typeof inventory.countingRule !== 'string' || !inventory.countingRule)
    fail('countingRule missing')
  if (!Array.isArray(inventory.seams) || inventory.seams.length === 0) fail('seams[] is empty')
  if (inventory.defaults?.shimTtlPhases !== 1) fail('defaults.shimTtlPhases must be 1')

  const seen = new Set<string>()
  let previous = ''
  for (const seam of inventory.seams) {
    if (!isRecord(seam)) fail('seam entry is not an object')
    const record = seam as unknown as SeamRecord
    if (typeof record.path !== 'string' || record.path.length === 0) fail('seam.path missing')
    if (seen.has(record.path)) fail(`duplicate seam path "${record.path}"`)
    seen.add(record.path)
    if (record.path < previous) fail(`seams[] not in ascending path order at "${record.path}"`)
    previous = record.path
    if (record.kind !== 'canonical' && record.kind !== 'cascading')
      fail(`"${record.path}" has unknown kind "${String(record.kind)}"`)
    if (record.status !== 'planned' && record.status !== 'active' && record.status !== 'retired')
      fail(`"${record.path}" has unknown status "${String(record.status)}"`)
    if (record.confidence !== 'high' && record.confidence !== 'low')
      fail(`"${record.path}" has unknown confidence "${String(record.confidence)}"`)
    if (typeof record.owner !== 'string' || record.owner.length === 0)
      fail(`"${record.path}" has no assigned migration lane owner`)
    if (typeof record.activationPhase !== 'string' || record.activationPhase.length === 0)
      fail(`"${record.path}" has no activationPhase`)
    for (const field of [
      'measuredFanIn',
      'measuredFanOut',
      'targetFanIn',
      'targetFanOut',
    ] as const) {
      const value = record[field]
      if (!Number.isInteger(value) || (value as number) < 0)
        fail(`"${record.path}" has invalid ${field}`)
    }
    if (!Array.isArray(record.allowedConsumers) || !Array.isArray(record.shims))
      fail(`"${record.path}" allowedConsumers/shims must be arrays`)
    if (typeof record.surfaceBaseline !== 'boolean')
      fail(`"${record.path}" surfaceBaseline must be a boolean`)
    validateContract(record)
  }
  return inventory.seams
}

export function loadSeamInventory(source?: string): SeamInventory {
  const inventory = readInventoryBlock(source)
  return { ...inventory, seams: validateInventory(inventory) }
}

export function edgeKey(edge: ConsumerEdge): string {
  return `${edge.from}|${edge.to}|${edge.kind}`
}

export function isOwned(file: string, scopes: string[]): boolean {
  return scopes.some((scope) => (scope.endsWith('/') ? file.startsWith(scope) : file === scope))
}

export function graphEdges(modules: CruiseModule[]): ConsumerEdge[] {
  const edges = new Map<string, ConsumerEdge>()
  for (const module of modules) {
    if (!module.source.startsWith('src/')) continue
    for (const dependency of module.dependencies ?? []) {
      if (!dependency.resolved) continue
      const edge: ConsumerEdge = {
        from: module.source,
        to: dependency.resolved,
        kind: dependency.dependencyTypes?.includes('type-only') ? 'type-only' : 'runtime',
      }
      edges.set(edgeKey(edge), edge)
    }
  }
  return [...edges.values()].sort((a, b) => (edgeKey(a) < edgeKey(b) ? -1 : 1))
}

/** Dependency-cruiser owns the resolved graph; only runtime edges propagate impurity. */
export function findImpureRuntimePaths(entry: string, edges: ConsumerEdge[]): string[][] {
  const found: string[][] = []
  const queue = [[entry]]
  const visited = new Set<string>()
  while (queue.length) {
    const chain = queue.shift()!
    const current = chain.at(-1)!
    if (visited.has(current)) continue
    visited.add(current)
    for (const edge of edges.filter(
      (candidate) => candidate.from === current && candidate.kind === 'runtime',
    )) {
      const next = [...chain, edge.to]
      if (
        /^node_modules\/(?:vue(?:\/|$)|vue-router(?:\/|$)|@tanstack\/|better-auth(?:\/|$)|vue-i18n(?:\/|$))|^src\/(?:shared|core)\/(?:auth\/|query\/|i18n\/(?!localeCore\.ts$))|^src\/features\//.test(
          edge.to,
        )
      )
        found.push(next)
      else if (edge.to.startsWith('src/')) queue.push(next)
    }
  }
  return found.sort((a, b) => (a.join('|') < b.join('|') ? -1 : 1))
}

export function measureSeam(seam: SeamRecord, edges: ConsumerEdge[], publicEntries: Set<string>) {
  const consumerEdges = edges.filter(
    (edge) => edge.to === seam.path && !isOwned(edge.from, seam.ownershipScope),
  )
  const outboundEdges = edges.filter(
    (edge) => isOwned(edge.from, seam.ownershipScope) && !isOwned(edge.to, seam.ownershipScope),
  )
  const fanOutTargets = [...new Set(outboundEdges.map((edge) => edge.to))].sort()
  const allowed = new Set(seam.allowedConsumers.map(edgeKey))
  return {
    consumerEdges,
    fanIn: new Set(consumerEdges.map((edge) => edge.from)).size,
    fanOut: fanOutTargets.length,
    fanOutTargets,
    outboundEdges,
    undeclaredConsumers: consumerEdges.filter((edge) => !allowed.has(edgeKey(edge))),
    missingConsumers: seam.allowedConsumers.filter(
      (edge) => !consumerEdges.some((actual) => edgeKey(actual) === edgeKey(edge)),
    ),
    undeclaredDeepImports: edges.filter(
      (edge) =>
        !isOwned(edge.from, seam.ownershipScope) &&
        isOwned(edge.to, seam.ownershipScope) &&
        edge.to !== seam.path &&
        !publicEntries.has(edge.to) &&
        !seam.shims.includes(edge.to),
    ),
  }
}

/** Project current source edges without accepting a target that does not exist yet. */
export function projectPlannedSeam(seam: SeamRecord, seams: SeamRecord[], edges: ConsumerEdge[]) {
  const destinations = new Map<string, Set<string>>()
  for (const planned of seams.filter((record) => record.status === 'planned')) {
    if (planned.sourcePaths.length !== 1) continue
    for (const source of planned.sourcePaths) {
      const targets = destinations.get(source) ?? new Set<string>()
      targets.add(planned.path)
      destinations.set(source, targets)
    }
  }
  const relocate = (file: string) => {
    const targets = destinations.get(file)
    return targets?.size === 1 ? [...targets][0]! : file
  }
  const scopes = seam.sourceOwnershipScope ?? seam.sourcePaths
  const consumers = new Map<string, ConsumerEdge>()
  for (const edge of edges) {
    if (!seam.sourcePaths.includes(edge.to) || isOwned(edge.from, scopes)) continue
    const projected = { ...edge, from: relocate(edge.from), to: seam.path }
    consumers.set(edgeKey(projected), projected)
  }
  const consumerEdges = [...consumers.values()].sort((a, b) => (edgeKey(a) < edgeKey(b) ? -1 : 1))
  const fanOutTargets = [
    ...new Set(
      edges
        .filter((edge) => isOwned(edge.from, scopes) && !isOwned(edge.to, scopes))
        .map((edge) => relocate(edge.to)),
    ),
  ].sort()
  const actualConsumers = new Set(consumerEdges.map(edgeKey))
  const frozenConsumers = new Set(seam.allowedConsumers.map(edgeKey))
  return {
    consumerEdges,
    fanIn: new Set(consumerEdges.map((edge) => edge.from)).size,
    fanOut: fanOutTargets.length,
    fanOutTargets,
    newConsumers: consumerEdges.filter((edge) => !frozenConsumers.has(edgeKey(edge))),
    missingConsumers: seam.allowedConsumers.filter((edge) => !actualConsumers.has(edgeKey(edge))),
    newFanOutTargets: fanOutTargets.filter((target) => !seam.targetFanOutTargets.includes(target)),
    missingFanOutTargets: seam.targetFanOutTargets.filter(
      (target) => !fanOutTargets.includes(target),
    ),
    splitSourcePaths: [...destinations]
      .filter(([, targets]) => targets.size > 1)
      .map(([source]) => source)
      .sort(),
  }
}

function runSelfTests(): void {
  const inventory = loadSeamInventory()
  const fixture = (mutate: (value: SeamInventory) => void) => {
    const value = structuredClone(inventory)
    mutate(value)
    return '```json seam-inventory\n' + JSON.stringify(value) + '\n```'
  }
  assert.equal(loadSeamInventory(fixture(() => {})).seams.length, inventory.seams.length)
  const invalid: Array<(value: SeamInventory) => void> = [
    (value) => {
      value.seams[0]!.owner = 'todo'
    },
    (value) => {
      value.seams[0]!.sourcePaths = []
    },
    (value) => {
      value.seams[0]!.ownershipScope = ['../escape/']
    },
    (value) => {
      value.seams[0]!.publicExports = ['x', 'x']
    },
    (value) => {
      value.seams[0]!.targetFanIn += 1
    },
    (value) => {
      value.seams[0]!.targetFanOut += 1
    },
    (value) => {
      value.seams[0]!.surfaceBaseline = false
    },
    (value) => {
      value.seams[0]!.allowedConsumers = [{ from: 'src/a.ts', to: 'src/wrong.ts', kind: 'runtime' }]
    },
    (value) => {
      value.seams[0]!.shims = ['src/a.shim.ts']
      value.seams[0]!.deletionOwner = null
    },
    (value) => {
      value.seams.push(value.seams[0]!)
    },
    (value) => {
      value.seams.find((seam) => seam.status === 'planned')!.exportMappings = []
    },
    (value) => {
      value.seams.find((seam) => seam.status === 'planned')!.sourceOwnershipScope = []
    },
  ]
  invalid.forEach((mutate) => assert.throws(() => loadSeamInventory(fixture(mutate))))
  const seam = {
    ...inventory.seams[0]!,
    path: 'src/card/index.ts',
    ownershipScope: ['src/card/'],
    allowedConsumers: [{ from: 'src/view.ts', to: 'src/card/index.ts', kind: 'runtime' as const }],
  }
  const measured = measureSeam(
    seam,
    graphEdges([
      { source: 'src/card/index.ts', dependencies: [{ resolved: 'src/card/Card.vue' }] },
      { source: 'src/card/card.unit.spec.ts', dependencies: [{ resolved: seam.path }] },
      {
        source: 'src/card/Card.vue',
        dependencies: [
          { resolved: 'src/utils.ts' },
          { resolved: 'node_modules/vue/index.js', dependencyTypes: ['type-only'] },
        ],
      },
      { source: 'src/view.ts', dependencies: [{ resolved: seam.path }] },
      {
        source: 'src/type.ts',
        dependencies: [{ resolved: seam.path, dependencyTypes: ['type-only'] }],
      },
      { source: 'src/deep.ts', dependencies: [{ resolved: 'src/card/Card.vue' }] },
    ]),
    new Set([seam.path]),
  )
  assert.equal(measured.fanIn, 2)
  assert.equal(measured.fanOut, 2)
  assert.equal(measured.consumerEdges[0]?.kind, 'type-only')
  assert.equal(measured.undeclaredConsumers.length, 1)
  assert.equal(measured.undeclaredDeepImports.length, 1)
  assert.equal(measured.missingConsumers.length, 0)
  assert.equal(measured.fanOutTargets.includes('src/card/Card.vue'), false)
  const planned = {
    ...seam,
    path: 'src/core/card.ts',
    status: 'planned' as const,
    sourcePaths: ['src/shared/card.ts'],
    sourceOwnershipScope: ['src/shared/card.ts'],
    targetFanOutTargets: ['src/core/helpers.ts'],
    allowedConsumers: [{ from: 'src/view.ts', to: 'src/core/card.ts', kind: 'runtime' as const }],
  }
  const helper = { ...planned, path: 'src/core/helpers.ts', sourcePaths: ['src/shared/helpers.ts'] }
  const projection = projectPlannedSeam(
    planned,
    [planned, helper],
    [
      { from: 'src/view.ts', to: 'src/shared/card.ts', kind: 'runtime' },
      { from: 'src/shared/card.ts', to: 'src/shared/helpers.ts', kind: 'type-only' },
    ],
  )
  assert.equal(projection.fanIn, 1)
  assert.deepEqual(projection.fanOutTargets, ['src/core/helpers.ts'])
  assert.equal(projection.newConsumers.length + projection.missingConsumers.length, 0)
  const split = projectPlannedSeam(
    planned,
    [planned, helper, { ...helper, path: 'src/features/card/helpers.ts' }],
    [{ from: 'src/shared/card.ts', to: 'src/shared/helpers.ts', kind: 'runtime' }],
  )
  assert.deepEqual(split.fanOutTargets, ['src/shared/helpers.ts'])
  assert.deepEqual(split.splitSourcePaths, ['src/shared/helpers.ts'])
  console.log(
    '✓ Seam schema: 1 valid / 12 invalid fixtures; external runtime/type/deep-edge and planned relocation/split measurement passed.',
  )
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  if (process.argv.includes('--self-test')) runSelfTests()
  else if (process.argv.includes('--render')) {
    const source = fs.readFileSync(SEAM_INVENTORY_PATH, 'utf8')
    const rows = loadSeamInventory(source).seams.map(
      (seam) =>
        `| \`${seam.path}\` | ${seam.kind} | ${seam.status} | ${seam.owner} | ${seam.publicExports.length} | ${seam.measuredFanIn} → ${seam.targetFanIn} | ${seam.measuredFanOut} → ${seam.targetFanOut} |`,
    )
    const table = [
      '<!-- SEAM_TABLE_START -->',
      '| Entry | Kind | Status | Owner | Exports | Fan-in measured → target | Fan-out measured → target |',
      '|---|---|---|---|---|---|---|',
      ...rows,
      '<!-- SEAM_TABLE_END -->',
    ].join('\n')
    if (!source.includes('<!-- SEAM_TABLE_START -->')) fail('generated table marker missing')
    fs.writeFileSync(
      SEAM_INVENTORY_PATH,
      source.replace(/<!-- SEAM_TABLE_START -->[\s\S]*?<!-- SEAM_TABLE_END -->/, table),
    )
  } else if (process.argv.includes('--freeze-measurements')) {
    const graphArgument = process.argv.find((argument) => argument.startsWith('--graph='))
    if (!graphArgument) fail('--freeze-measurements requires --graph=<dependency-cruiser JSON>')
    const graph = JSON.parse(fs.readFileSync(graphArgument.slice('--graph='.length), 'utf8')) as {
      modules: CruiseModule[]
    }
    if (!Array.isArray(graph.modules)) fail('graph.modules missing')
    const source = fs.readFileSync(SEAM_INVENTORY_PATH, 'utf8')
    const inventory = loadSeamInventory(source)
    const edges = graphEdges(graph.modules)
    const publicEntries = new Set(
      inventory.seams.filter((seam) => seam.kind === 'canonical').map((seam) => seam.path),
    )
    for (const seam of inventory.seams) {
      if (seam.status !== 'active') continue
      const measured = measureSeam(seam, edges, publicEntries)
      seam.allowedConsumers = measured.consumerEdges
      seam.targetFanIn = measured.fanIn
      seam.targetFanOut = measured.fanOut
      seam.targetFanOutTargets = measured.fanOutTargets
      if (seam.status === 'active') {
        seam.measuredFanIn = measured.fanIn
        seam.measuredFanOut = measured.fanOut
      }
    }
    validateInventory(inventory)
    fs.writeFileSync(
      SEAM_INVENTORY_PATH,
      source.replace(
        /```json seam-inventory\r?\n[\s\S]*?\r?\n```/,
        `\`\`\`json seam-inventory\n${JSON.stringify(inventory, null, 2)}\n\`\`\``,
      ),
    )
    console.log(
      'Frozen graph measurements refreshed; review the diff and regenerate the table with --render.',
    )
  } else console.log(`✓ ${loadSeamInventory().seams.length} Seam records validated.`)
}

/**
 * Entrypoints the surface ratchet enforces: active seams whose symbols are
 * frozen in `publicApiSurface.baseline.json`. Sorted for stable output.
 */
export function listSurfaceEntrypoints(seams: SeamRecord[]): string[] {
  return seams
    .filter((seam) => seam.status === 'active' && seam.surfaceBaseline)
    .map((seam) => seam.path)
    .sort()
}
