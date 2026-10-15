import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { arch, cpus, platform } from 'node:os'
import { performance } from 'node:perf_hooks'
import { fileURLToPath } from 'node:url'
import { parseArgs } from 'node:util'
import {
  CanvasGraphStore,
  toCanvasGraphNodeId,
} from '../src/features/canvas/core/graph/canvasGraphStore.ts'
import { createEmptyBoardSnapshot, type BoardSnapshot } from '../src/features/board/ports/canvas.ts'

import { boardSnapshotToFlow } from '../src/features/canvas/session/flow-adapter/projection/flowSnapshotAdapter.ts'

type ScenarioName = 'hydrate' | 'export' | 'export-and-clone' | 'node-drag' | 'flow-projection'
const SCENARIOS: readonly ScenarioName[] = [
  'hydrate',
  'export',
  'export-and-clone',
  'node-drag',
  'flow-projection',
]

type Options = {
  nodes: number[]
  samples: number
  warmup: number
  seed: number
  scenarios: ScenarioName[]
  isJson: boolean
}

function integer(value: string, name: string, minimum: number, maximum: number): number {
  const parsed = Number(value)
  if (!Number.isSafeInteger(parsed) || parsed < minimum || parsed > maximum) {
    throw new Error(`${name} must be an integer between ${minimum} and ${maximum}`)
  }
  return parsed
}

function readOptions(): Options {
  const { values } = parseArgs({
    options: {
      nodes: { type: 'string', default: '100,500,1000' },
      samples: { type: 'string', default: '30' },
      warmup: { type: 'string', default: '5' },
      seed: { type: 'string', default: '42' },
      scenarios: { type: 'string', default: SCENARIOS.join(',') },
      json: { type: 'boolean', default: false },
    },
    allowPositionals: false,
  })
  const scenarios = values.scenarios.split(',').map((name) => {
    const scenario = SCENARIOS.find((candidate) => candidate === name)
    if (!scenario) throw new Error(`Unknown scenario: ${name}; choose ${SCENARIOS.join(',')}`)
    return scenario
  })
  return {
    nodes: [...new Set(values.nodes.split(',').map((n) => integer(n, 'nodes', 10, 10000)))],
    samples: integer(values.samples, 'samples', 5, 10000),
    warmup: integer(values.warmup, 'warmup', 1, 1000),
    seed: integer(values.seed, 'seed', 0, 0xffffffff),
    scenarios: [...new Set(scenarios)],
    isJson: values.json,
  }
}

function createFixture(count: number, seed: number): BoardSnapshot {
  let state = seed >>> 0
  const random = () => {
    state = (Math.imul(state, 1664525) + 1013904223) >>> 0
    return state / 0x100000000
  }
  const fixture = createEmptyBoardSnapshot()
  for (let index = 0; index < count; index += 1) {
    const id = index + 1
    const geometry = {
      id,
      x: (index % 20) * 360 + Math.floor(random() * 20),
      y: Math.floor(index / 20) * 420 + Math.floor(random() * 20),
      width: 280,
    }
    if (index % 10 < 6) {
      fixture.imageNodes.push({
        ...geometry,
        type: 'image-node',
        sourceImageUrl: `https://benchmark.invalid/image-${id}.webp`,
        sourceImageRatio: 0.5 + random() * 1.5,
        outputCount: 1,
        outputStatus: 'ready',
        outputImages: [],
      })
    } else if (index % 10 < 9) {
      fixture.promptNodes.push({
        ...geometry,
        type: 'prompt',
        text: `Prompt ${id}: ${'deterministic canvas workload. '.repeat(12)}`,
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      })
    } else {
      fixture.fusionNodes.push({
        ...geometry,
        type: 'fusion',
        prompt: `Blend ${id}`,
        outputCount: 1,
        aspectRatio: 'auto',
        blendPoint: { x: 0.5, y: 0.5 },
        inputImages: [],
        outputStatus: 'idle',
        outputImages: [],
      })
    }
  }
  appendRelations(fixture)
  return fixture
}

function appendRelations(fixture: BoardSnapshot) {
  fixture.promptNodes.forEach((node, index) => {
    const source = fixture.imageNodes[index % fixture.imageNodes.length]
    assert.ok(source)
    fixture.connections.push({
      id: index + 1,
      fromNodeType: 'image',
      fromNodeId: source.id,
      fromPort: 'image',
      toNodeType: 'prompt',
      toNodeId: node.id,
      toPort: 'image',
    })
  })
  for (let index = 0; index + 2 < fixture.imageNodes.length; index += 10) {
    const members = fixture.imageNodes.slice(index, index + 3)
    const first = members[0]
    assert.ok(first)
    fixture.groups.push({
      id: `group-${index + 1}`,
      nodeIds: members.map((node) => toCanvasGraphNodeId('image', node.id)),
      x: first.x - 20,
      y: first.y - 20,
      width: 1040,
      height: 400,
    })
  }
}

function validateFixture(fixture: BoardSnapshot, count: number) {
  const store = CanvasGraphStore.fromSnapshot(fixture)
  assert.equal(store.readCounts().nodes, count)
  assert.equal(store.readCounts().edges, fixture.connections.length)
  assert.deepEqual(store.exportSnapshot(fixture), fixture)
  const first = fixture.imageNodes[0]
  assert.ok(first)
  const originalX = first.x
  const nodeId = toCanvasGraphNodeId('image', first.id)
  const summary = store
    .beginTransaction('node-drag')
    .updateNode(nodeId, { x: first.x + 1 })
    .commit()
  assert.equal(summary.didChange, true)
  const projected = boardSnapshotToFlow(fixture)
  assert.equal(projected.nodes.length, count)
  assert.equal(projected.edges.length, fixture.connections.length)
  assert.equal(store.getNode(nodeId)?.snapshot.x, originalX + 1)
  assert.equal(fixture.imageNodes[0]?.x, originalX)
}

function createScenario(name: ScenarioName, fixture: BoardSnapshot): () => unknown {
  if (name === 'hydrate') return () => CanvasGraphStore.fromSnapshot(fixture)
  if (name === 'flow-projection') return () => boardSnapshotToFlow(fixture)
  const store = CanvasGraphStore.fromSnapshot(fixture)
  if (name === 'export') return () => store.exportSnapshot(fixture)
  if (name === 'export-and-clone') return () => structuredClone(store.exportSnapshot(fixture))
  const first = fixture.imageNodes[0]
  assert.ok(first)
  const nodeId = toCanvasGraphNodeId('image', first.id)
  let x = first.x
  return () => {
    x += 1
    return store.beginTransaction('node-drag').updateNode(nodeId, { x }).commit()
  }
}

function measure(task: () => unknown, options: Options) {
  for (let index = 0; index < options.warmup; index += 1) task()
  const samples: number[] = []
  for (let index = 0; index < options.samples; index += 1) {
    const startedAt = performance.now()
    task()
    samples.push(performance.now() - startedAt)
  }
  samples.sort((left, right) => left - right)
  const percentile = (value: number) => samples[Math.ceil(samples.length * value) - 1] ?? 0
  return {
    sampleCount: samples.length,
    mean: samples.reduce((sum, value) => sum + value, 0) / samples.length,
    p50: percentile(0.5),
    p95: percentile(0.95),
    max: samples.at(-1) ?? 0,
  }
}

async function main() {
  const options = readOptions()
  const results = options.nodes.flatMap((nodeCount) => {
    const fixture = createFixture(nodeCount, options.seed)
    validateFixture(fixture, nodeCount)
    const fixtureSha256 = createHash('sha256').update(JSON.stringify(fixture)).digest('hex')
    return options.scenarios.map((scenario) => ({
      scenario,
      nodes: nodeCount,
      edges: fixture.connections.length,
      groups: fixture.groups.length,
      fixtureSha256,
      timing: measure(createScenario(scenario, fixture), options),
    }))
  })
  const report = {
    schemaVersion: 1,
    kind: 'canvas-logic',
    units: 'ms',
    createdAt: new Date().toISOString(),
    revision: execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim(),
    hasWorkingTreeChanges:
      execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim().length > 0,
    trackedDiffSha256: createHash('sha256')
      .update(execFileSync('git', ['diff', 'HEAD']))
      .digest('hex'),
    runnerSha256: createHash('sha256')
      .update(await readFile(fileURLToPath(import.meta.url)))
      .digest('hex'),
    environment: {
      node: process.version,
      platform: platform(),
      arch: arch(),
      cpu: cpus()[0]?.model,
    },
    seed: options.seed,
    warmup: options.warmup,
    results,
  }
  const directory = new URL('../.benchmark-reports/', import.meta.url)
  const output = new URL(`canvas-interactions-${Date.now()}.json`, directory)
  await mkdir(directory, { recursive: true })
  const json = JSON.stringify(report, null, 2)
  await writeFile(output, `${json}\n`)
  if (options.isJson) console.log(json)
  else {
    console.table(results.map(({ scenario, nodes, timing }) => ({ scenario, nodes, ...timing })))
    console.log(`Report: ${fileURLToPath(output)}`)
    console.log('Node logic timings exclude browser rendering, image decoding, network and FPS.')
  }
}

await main()
