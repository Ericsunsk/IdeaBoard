/// <reference types="node" />

import { existsSync } from 'node:fs'
import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

const canvasRoot = path.resolve(process.cwd(), 'src/features/canvas')
const corePath = path.join(canvasRoot, 'execution/pending-execution/PendingImageExecutionCore.ts')
const flowRuntimePath = path.join(canvasRoot, 'workspace-runtime/internal/useFlowCanvasRuntime.ts')
const legacyRuntimePath = path.join(canvasRoot, 'execution/runtime/CanvasExecutionRuntime.ts')

describe('Pending Image Execution ownership contract', () => {
  it('keeps one Core owner and derives Flow edge animation from its snapshot', async () => {
    expect(existsSync(legacyRuntimePath)).toBe(false)
    const [core, flowRuntime] = await Promise.all([
      readFile(corePath, 'utf8'),
      readFile(flowRuntimePath, 'utf8'),
    ])

    expect(core).toMatch(/export function createPendingImageExecutionCore\(/)
    expect(core).toMatch(/captureActiveLeases/)
    expect(core).toMatch(/cancellableTaskIds/)
    expect(flowRuntime).toMatch(
      /projectPendingImageExecutionEdges\(edges, executionSnapshot\.value\)\.edges/,
    )
    expect(flowRuntime).toMatch(/const renderEdges = computed\(/)
    expect(flowRuntime).toMatch(/graphAdapter\.applyIncrementalProjection\(/)
    expect(flowRuntime).not.toMatch(/watch\(\s*\[options\.edges, executionSnapshot\]/)
    expect(flowRuntime).toMatch(/options\.executionCore\.release\(leases\)/)
    expect(flowRuntime).toMatch(/options\.cancelTasks\(release\.cancellableTaskIds\)/)
    expect(flowRuntime).not.toMatch(/runningNodeIds\s*=\s*ref/)
  })
})
