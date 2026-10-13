import fs from 'node:fs'
import assert from 'node:assert/strict'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

/** Shared, validated source for Frontend public Seam contracts. */

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const SEAM_INVENTORY_PATH = path.join(webRoot, '..', '..', 'docs', 'frontend-seam-inventory.md')

type SeamKind = 'canonical' | 'cascading'

export type SeamRecord = {
  path: string
  kind: SeamKind
  owner: string
  surfaceBaseline: boolean
  allowedConsumers: ConsumerEdge[]
  measuredFanIn: number
  measuredFanOut: number
  targetFanIn: number
  targetFanOut: number
  notes: string
  ownershipScope: string[]
  publicExports: string[]
  targetFanOutTargets: string[]
  purity?: 'pure'
}

export type ConsumerEdge = { from: string; to: string; kind: 'runtime' | 'type-only' }
export type CruiseModule = {
  source: string
  dependencies?: Array<{ resolved?: string; dependencyTypes?: string[] }>
}

export type SeamInventory = {
  version: number
  countingRule: string
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
  for (const field of ['ownershipScope', 'publicExports', 'targetFanOutTargets'] as const)
    validateStrings(record[field], `${record.path}.${field}`)
  for (const field of ['ownershipScope'] as const) {
    if (!record[field].length) fail(`${record.path}: ${field} cannot be empty`)
    record[field].forEach(validatePath)
  }
  if (record.kind === 'canonical' && !record.surfaceBaseline)
    fail(`${record.path}: canonical surface coverage missing`)
  for (const field of ['owner'] as const)
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
}

/** Structural validation shared by every consumer of the inventory. */
function validateInventory(inventory: SeamInventory): SeamRecord[] {
  if (!isRecord(inventory)) fail('top-level value is not an object')
  if (inventory.version !== 3) fail(`version is ${String(inventory.version)}, expected 3`)
  if (typeof inventory.countingRule !== 'string' || !inventory.countingRule)
    fail('countingRule missing')
  if (!Array.isArray(inventory.seams) || inventory.seams.length === 0) fail('seams[] is empty')

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
    if (!Array.isArray(record.allowedConsumers))
      fail(`"${record.path}" allowedConsumers must be an array`)
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
        !publicEntries.has(edge.to),
    ),
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
      value.seams[0]!.ownershipScope = []
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
      value.seams.push(value.seams[0]!)
    },
    (value) => {
      value.version = 0
    },
    (value) => {
      value.seams[0]!.path = '../escape.ts'
    },
    (value) => {
      value.seams[0]!.targetFanOutTargets = ['', '']
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
  const nestedSeam = {
    ...seam,
    path: 'src/features/canvas/session/projection/index.ts',
    ownershipScope: ['src/features/canvas/session/projection/'],
    allowedConsumers: [],
  }
  const nestedEdges: ConsumerEdge[] = [
    {
      from: 'src/features/canvas/ui/Node.vue',
      to: 'src/features/canvas/session/projection/internal.ts',
      kind: 'runtime',
    },
    {
      from: 'src/features/canvas/ui/node.unit.spec.ts',
      to: 'src/features/canvas/session/projection/internal.ts',
      kind: 'runtime',
    },
  ]
  assert.deepEqual(
    measureSeam(nestedSeam, nestedEdges, new Set([nestedSeam.path])).undeclaredDeepImports,
    nestedEdges,
  )
  console.log(
    `✓ Seam schema: 1 valid / ${invalid.length} invalid fixtures; external runtime/type/deep-edge and nested ownership (including tests) passed.`,
  )
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  if (process.argv.includes('--self-test')) runSelfTests()
  else if (process.argv.includes('--render')) {
    const source = fs.readFileSync(SEAM_INVENTORY_PATH, 'utf8')
    const rows = loadSeamInventory(source).seams.map(
      (seam) =>
        `| \`${seam.path}\` | ${seam.kind} | ${seam.owner} | ${seam.publicExports.length} | ${seam.measuredFanIn} → ${seam.targetFanIn} | ${seam.measuredFanOut} → ${seam.targetFanOut} |`,
    )
    const table = [
      '<!-- SEAM_TABLE_START -->',
      '| Entry | Kind | Owner | Exports | Fan-in measured → target | Fan-out measured → target |',
      '|---|---|---|---|---|---|',
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
      const measured = measureSeam(seam, edges, publicEntries)
      seam.allowedConsumers = measured.consumerEdges
      seam.targetFanIn = measured.fanIn
      seam.targetFanOut = measured.fanOut
      seam.targetFanOutTargets = measured.fanOutTargets
      seam.measuredFanIn = measured.fanIn
      seam.measuredFanOut = measured.fanOut
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
 * Entrypoints the surface ratchet enforces: seams whose symbols are
 * frozen in `publicApiSurface.baseline.json`. Sorted for stable output.
 */
export function listSurfaceEntrypoints(seams: SeamRecord[]): string[] {
  return seams
    .filter((seam) => seam.surfaceBaseline)
    .map((seam) => seam.path)
    .sort()
}
