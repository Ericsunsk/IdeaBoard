import { describe, expect, it } from 'vitest'
import {
  preserveCanvasRendererSelection,
  resolveCanvasRendererSelection,
  stageCanvasRendererSelection,
} from './canvasSelectionCoordinator'
import type { CanvasRendererSelectableElement } from '../types'

const element = (value: Record<string, unknown>) =>
  value as unknown as CanvasRendererSelectableElement

describe('canvasSelectionCoordinator', () => {
  it('preserves renderer selection across graph projections', () => {
    const projected = preserveCanvasRendererSelection(
      { nodes: [element({ id: 'image-1', selected: true })], edges: [] },
      {
        nodes: [element({ id: 'image-1', selected: false }), element({ id: 'prompt-2' })],
        edges: [],
      },
    )
    expect(projected.nodes.map((node) => [node.id, node.selected])).toEqual([
      ['image-1', true],
      ['prompt-2', false],
    ])
  })

  it('maps only valid renderer ids to graph selection', () => {
    const result = resolveCanvasRendererSelection([
      element({ id: 'image-3', selected: true }),
      element({ id: 'prompt-nope', selected: true }),
      element({ id: 'connection-8', source: 'image-3', target: 'prompt-2', selected: true }),
      element({ id: 'edge-invalid', source: 'a', target: 'b', selected: true }),
    ])
    expect([...result.selectedNodeIds]).toEqual(['image-3'])
    expect([...result.selectedEdgeIds]).toEqual(['connection-8'])
  })

  it('stages a complete selection state, including deselection', () => {
    const calls: string[] = []
    stageCanvasRendererSelection(
      {
        getNodeIds: () => ['image-1', 'prompt-2'],
        getEdgeIds: () => ['connection-3'],
      },
      {
        selectNode: (id, selected) => calls.push(`node:${id}:${selected}`),
        selectEdge: (id, selected) => calls.push(`edge:${id}:${selected}`),
      },
      [element({ id: 'image-1', selected: true })],
    )
    expect(calls).toEqual(['node:image-1:true', 'node:prompt-2:false', 'edge:connection-3:false'])
  })
})
