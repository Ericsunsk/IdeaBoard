/// <reference types="node" />

import { existsSync } from 'node:fs'
import { readdir, readFile } from 'node:fs/promises'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

const canvasRoot = path.resolve(process.cwd(), 'src/features/canvas')
const runtimeSourcePath = path.join(canvasRoot, 'execution/runtime/CanvasExecutionRuntime.ts')
const flowRuntimeSourcePath = path.join(canvasRoot, 'composables/useFlowCanvasRuntime.ts')
const flowNodeAdapterSourcePath = path.join(canvasRoot, 'composables/useFlowCanvasNodeAdapter.ts')
const connectionLifecycleSourcePath = path.join(
  canvasRoot,
  'flow-pane/connectionCommandLifecycle.ts',
)
const legacyEdgeAnimationPath = path.join(canvasRoot, 'execution/flowEdgeAnimation.ts')
const legacyDeletedRunningTasksPath = path.join(canvasRoot, 'utils/flowDeletedRunningTasks.ts')

const forbiddenProductionPatterns = [
  { label: 'legacy edge-animation module', pattern: /\bflowEdgeAnimation\b/ },
  { label: 'imperative edge-animation sync', pattern: /\bsyncImageInputEdgeAnimation\b/ },
  { label: 'execution-state callback relay', pattern: /\bonExecutionStateChange\b/ },
  { label: 'legacy execution-change DTO', pattern: /\bCanvasNodeExecutionChange\b/ },
  { label: 'dead raw execution-state API', pattern: /\bgetNodeExecutionState\b/ },
  { label: 'legacy pending-action reuse key', pattern: /\bpendingClientActionKey\b/ },
  { label: 'legacy pending-action relay', pattern: /\bclientActionKey\b/ },
  { label: 'duplicate running-state mutation', pattern: /\bsetNodeRunning\b/ },
  { label: 'snapshot-owned delete lifecycle', pattern: /\bcollectDeletedRunningTaskIds\b/ },
  { label: 'dead per-node clear API', pattern: /\bclearNode\b/ },
  { label: 'optional Board identity fallback', pattern: /\bgetCurrentBoardId\?\s*:/ },
  {
    label: 'duplicate running-state store',
    pattern: /\brunningNodeIds\s*=\s*ref\s*<\s*Set\s*</,
  },
] as const

async function listProductionTypeScriptFiles(directory: string): Promise<string[]> {
  const entries = await readdir(directory, { withFileTypes: true })
  const nestedFiles = await Promise.all(
    entries.map(async (entry) => {
      const absolutePath = path.join(directory, entry.name)
      if (entry.isDirectory()) return listProductionTypeScriptFiles(absolutePath)
      if (!entry.isFile() || !entry.name.endsWith('.ts') || entry.name.endsWith('.spec.ts')) {
        return []
      }
      return [absolutePath]
    }),
  )
  return nestedFiles.flat().sort()
}

function relativeCanvasPath(filePath: string): string {
  return path.relative(canvasRoot, filePath).split(path.sep).join('/')
}

describe('Canvas Execution Runtime ownership contract', () => {
  it('has no legacy running-state owner, callback relay, or imperative animation path', async () => {
    expect(existsSync(legacyEdgeAnimationPath)).toBe(false)
    expect(existsSync(legacyDeletedRunningTasksPath)).toBe(false)

    const sourceFiles = await listProductionTypeScriptFiles(canvasRoot)
    const violations = (
      await Promise.all(
        sourceFiles.map(async (filePath) => {
          const source = await readFile(filePath, 'utf8')
          return forbiddenProductionPatterns.flatMap(({ label, pattern }) =>
            pattern.test(source) ? [`${relativeCanvasPath(filePath)}: ${label}`] : [],
          )
        }),
      )
    ).flat()

    expect(violations).toEqual([])
  })

  it('keeps execution state and image-input edge projection owned by CanvasExecutionRuntime', async () => {
    const [runtimeSource, flowRuntimeSource, flowNodeAdapterSource, connectionLifecycleSource] =
      await Promise.all([
        readFile(runtimeSourcePath, 'utf8'),
        readFile(flowRuntimeSourcePath, 'utf8'),
        readFile(flowNodeAdapterSourcePath, 'utf8'),
        readFile(connectionLifecycleSourcePath, 'utf8'),
      ])

    expect(runtimeSource).toMatch(/export function createCanvasExecutionRuntime\(/)
    expect(runtimeSource).toMatch(/export function projectCanvasExecutionEdgeAnimation\(/)
    expect(runtimeSource).toMatch(
      /const shouldAnimate = isExecutionActive\(nodeStates\.get\(edge\.target\)\)/,
    )
    expect(flowRuntimeSource).toMatch(
      /projectCanvasExecutionEdgeAnimation\(edges, options\.executionRuntime\.nodeStates\.value\)/,
    )
    expect(flowRuntimeSource).toMatch(
      /watch\(\s*\[options\.edges, options\.executionRuntime\.nodeStates\]/,
    )
    expect(flowNodeAdapterSource).not.toMatch(/outputStatus\s*===\s*['"]running['"]/)
    expect(connectionLifecycleSource).not.toMatch(/\banimated:\s*nextEdge\.animated\b/)
    expect(runtimeSource).not.toMatch(/\bclear:\s*\(\)\s*=>\s*void/)
    expect(runtimeSource).not.toMatch(/\bfunction clear\(/)
  })
})
