/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const canvasRoot = resolve(process.cwd(), 'src/features/canvas')
const readSource = (relativePath: string) => readFileSync(resolve(canvasRoot, relativePath), 'utf8')

const canvasScreen = readSource('screen-presentation/CanvasScreen.vue')
const screenIndex = readSource('screen-presentation/index.ts')
const screenLifecycle = readSource('screen-presentation/internal/useCanvasScreenLifecycle.ts')
const runtimeParts = readSource('workspace-runtime/internal/createCanvasWorkspaceRuntimeParts.ts')
const imageWorkflow = readSource('image-workflow/useCanvasImageWorkflow.ts')

describe('Canvas Screen architecture contract', () => {
  it('keeps CanvasScreen as the sole public screen-presentation seam', () => {
    expect(screenIndex.trim()).toBe("export { default as CanvasScreen } from './CanvasScreen.vue'")
    expect(screenIndex).not.toContain('./internal/')
  })

  it('assembles cohesive private ownership modules without temporal callback backchannels', () => {
    expect(canvasScreen).toContain('./internal/useCanvasScreenImageToolUi')
    expect(canvasScreen).toContain('./internal/useCanvasScreenSurfaceInteractions')
    expect(canvasScreen).toContain('./internal/createCanvasScreenNodePresentation')
    expect(canvasScreen).toContain('./internal/useCanvasScreenLifecycle')
    expect(canvasScreen).not.toContain('__ideaboardCanvasBenchmarkActions')
    expect(screenLifecycle).toContain('__ideaboardCanvasBenchmarkActions')
    expect(canvasScreen).not.toMatch(/let\s+\w+\s*:[^=]+?=\s*\(.*?\)\s*=>\s*\{\}/)

    for (const removedBackchannel of [
      'startNodeDragInteraction',
      'stopNodeDragInteraction',
      'refreshFlowHandleVisibility',
      'onNativeRemove',
    ]) {
      expect(runtimeParts).not.toContain(removedBackchannel)
    }
  })

  it('gets Image Workflow locks and selection from runtime seams instead of Screen callbacks', () => {
    const uiAdapter = imageWorkflow.slice(
      imageWorkflow.indexOf('export type CanvasImageWorkflowUiAdapter'),
      imageWorkflow.indexOf('export type CanvasImageWorkflowOptions'),
    )

    expect(uiAdapter).not.toContain('isCanvasInteractionLocked')
    expect(uiAdapter).not.toContain('selection:')
    expect(imageWorkflow).toContain('isCanvasInteractionLocked: graph.isCanvasInteractionLocked')
    expect(imageWorkflow).toContain(
      'selectFlowNodeForContextMenu: options.flowPaneAdapter.selectOnlyNode',
    )
  })

  it('keeps connection endpoints immutable in the Vue Flow surface', () => {
    expect(canvasScreen).toContain(':edges="renderEdges"')
    expect(canvasScreen).toContain('edgesUpdatable: false')
    expect(canvasScreen).toContain('connectOnClick: false')
    expect(canvasScreen).toContain('defaultEdgeOptions: { animated: false, updatable: false }')
  })
})
