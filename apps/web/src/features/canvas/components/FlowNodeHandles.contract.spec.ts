/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const canvasRoot = resolve(process.cwd(), 'src/features/canvas')
const handlesComponent = readFileSync(
  resolve(canvasRoot, 'components/FlowNodeHandles.vue'),
  'utf8',
)
const handlesPresentation = readFileSync(
  resolve(canvasRoot, 'composables/useFlowNodeHandlesPresentation.ts'),
  'utf8',
)

describe('Flow Node Handle geometry contract', () => {
  it('keeps static hit geometry in CSS and runtime pointer geometry in presentation', () => {
    expect(handlesComponent).toContain(
      'width: calc(52px * var(--canvas-node-ui-scale, 1));',
    )
    expect(handlesComponent).toContain(
      'height: calc(52px * var(--canvas-node-ui-scale, 1));',
    )
    expect(handlesPresentation).not.toContain('--handle-hit-size')
    expect(handlesPresentation).toContain("'--handle-plus-x': `${x}px`")
    expect(handlesPresentation).toContain("'--handle-plus-y': `${y}px`")
  })

  it('keeps Vue Flow Handles mounted while hiding both visuals during connection drag', () => {
    expect(handlesComponent).toContain(
      "'is-connection-dragging': aggregate.connectionDragging",
    )
    expect(handlesComponent).toContain(
      '.flow-canvas-node__handle-aggregate.is-connection-dragging .flow-canvas-node__handle-plus',
    )
    expect(handlesComponent).not.toContain('v-if="!aggregate.connectionDragging"')
    expect(handlesPresentation).toContain('const { connectionStartHandle } = useVueFlow()')
    expect(handlesPresentation).not.toContain("window.addEventListener('pointerup'")
  })
})
