import type { Edge } from '@vue-flow/core'
import { describe, expect, it } from 'vitest'
import { createEmptyBoardSnapshot } from '../../../board/domain'
import { createCanvasGraphStore, toCanvasGraphEdgeId } from '../../domain/graph'
import { createFlowEdgeProjectionIndex } from './flowConnectionIdentity'

describe('flowConnectionIdentity', () => {
  it('resolves generated projection edges in both directions through one index', () => {
    const snapshot = createEmptyBoardSnapshot()
    snapshot.imageNodes.push({
      id: 1,
      type: 'image-node',
      x: 0,
      y: 0,
      width: 280,
      sourceImageUrl: 'https://example.com/image.png',
      sourceImageRatio: 1,
      outputStatus: 'ready',
      outputImages: [],
    })
    snapshot.promptNodes.push({
      id: 4,
      type: 'prompt',
      x: 0,
      y: 0,
      width: 280,
      text: '',
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
    })
    snapshot.connections.push({
      id: 42,
      fromNodeType: 'image',
      fromNodeId: 1,
      fromPort: 'image',
      toNodeType: 'prompt',
      toNodeId: 4,
      toPort: 'image',
    })
    const generatedEdge = {
      id: 'vueflow__edge-image-1out:image-prompt-4in:image',
      source: 'image-1',
      sourceHandle: 'out:image',
      target: 'prompt-4',
      targetHandle: 'in:image',
    } as Edge

    const projectionIndex = createFlowEdgeProjectionIndex(createCanvasGraphStore(snapshot), [
      generatedEdge,
    ])
    const canvasEdgeId = toCanvasGraphEdgeId(42)

    expect(projectionIndex.resolveCanvasEdgeId(generatedEdge)).toBe(canvasEdgeId)
  })

  it('maps restored id collisions to the connections materialized for each endpoint', () => {
    const snapshot = createEmptyBoardSnapshot()
    snapshot.imageNodes.push(
      {
        id: 1,
        type: 'image-node',
        x: 0,
        y: 0,
        width: 280,
        sourceImageUrl: 'https://example.com/image-1.png',
        sourceImageRatio: 1,
        outputStatus: 'ready',
        outputImages: [],
      },
      {
        id: 2,
        type: 'image-node',
        x: 0,
        y: 0,
        width: 280,
        sourceImageUrl: 'https://example.com/image-2.png',
        sourceImageRatio: 1,
        outputStatus: 'ready',
        outputImages: [],
      },
    )
    snapshot.promptNodes.push({
      id: 4,
      type: 'prompt',
      x: 0,
      y: 0,
      width: 280,
      text: '',
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
    })
    snapshot.connections.push(
      {
        id: 7,
        fromNodeType: 'image',
        fromNodeId: 1,
        fromPort: 'image',
        toNodeType: 'prompt',
        toNodeId: 4,
        toPort: 'image',
      },
      {
        id: 43,
        fromNodeType: 'image',
        fromNodeId: 2,
        fromPort: 'image',
        toNodeType: 'prompt',
        toNodeId: 4,
        toPort: 'image',
      },
    )
    const firstEdge = {
      id: 'edge-first',
      source: 'image-1',
      sourceHandle: 'out:image',
      target: 'prompt-4',
      targetHandle: 'in:image',
      data: { connectionSnapshot: { id: 7 } },
    } as Edge
    const collidingEdge = {
      id: 'edge-colliding',
      source: 'image-2',
      sourceHandle: 'out:image',
      target: 'prompt-4',
      targetHandle: 'in:image',
      data: { connectionSnapshot: { id: 7 } },
    } as Edge

    const projectionIndex = createFlowEdgeProjectionIndex(createCanvasGraphStore(snapshot), [
      firstEdge,
      collidingEdge,
    ])

    expect(projectionIndex.resolveCanvasEdgeId(firstEdge)).toBe(toCanvasGraphEdgeId(7))
    expect(projectionIndex.resolveCanvasEdgeId(collidingEdge)).toBe(toCanvasGraphEdgeId(43))
    expect(projectionIndex.getFlowEdge(collidingEdge.id)).toBe(collidingEdge)
  })
})
