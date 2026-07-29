import { describe, expect, it } from 'vitest'
import { shallowRef } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { ImageNodeItem } from '../../canvas-board/domain'
import type { FlowNodeData } from '../types/flowTypes'
import { useCanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import { useFlowImageNodeInteractions } from './useFlowImageNodeInteractions'

describe('useFlowImageNodeInteractions', () => {
  it('preserves output media asset ids when duplicating an image node', () => {
    const source: ImageNodeItem = {
      id: 10,
      type: 'image-node',
      x: 10,
      y: 20,
      width: 320,
      sourceImageUrl: '',
      sourceMediaAssetId: 101,
      sourceImageRatio: 1,
      outputStatus: 'ready',
      outputImages: [{ url: 'https://example.test/output.png', ratio: 0.75, mediaAssetId: 201 }],
    }
    const nodes = shallowRef<Node[]>([
      ({
        id: 'image-10',
        type: 'canvas',
        selected: true,
        position: { x: source.x, y: source.y },
        data: {
          kind: 'image',
          nodeType: 'image',
          title: 'Image #10',
          snapshotNode: source,
        } satisfies FlowNodeData,
      } as unknown as Node),
    ])
    const edges = shallowRef<Edge[]>([])
    const graphAdapter = useCanvasVueFlowGraphAdapter({ nodes, edges })
    const interactions = useFlowImageNodeInteractions({
      nodes,
      graphAdapter,
      getImageNodeDisplayPayload: () => null,
      getImageNodeSnapshot: () => null,
    })

    interactions.beginFlowNodeDragDuplicateTracking(true)
    nodes.value[0] = { ...nodes.value[0], position: { x: 30, y: 45 } }
    interactions.finishFlowNodeDragDuplicateTracking(true)

    expect(nodes.value).toHaveLength(2)
    const copied = nodes.value[1]?.data?.snapshotNode as ImageNodeItem
    expect(copied.outputImages).toEqual([
      { url: 'https://example.test/output.png', ratio: 0.75, mediaAssetId: 201 },
    ])
    expect(copied.sourceMediaAssetId).toBe(101)
  })

  it('preserves the displayed asset id when appending an output as a new image node', () => {
    const nodes = shallowRef<Node[]>([])
    const edges = shallowRef<Edge[]>([])
    const graphAdapter = useCanvasVueFlowGraphAdapter({ nodes, edges })
    const interactions = useFlowImageNodeInteractions({
      nodes,
      graphAdapter,
      getImageNodeDisplayPayload: () => ({
        url: 'https://example.test/output.png',
        ratio: 0.75,
        mediaAssetId: 201,
      }),
      getImageNodeSnapshot: () => ({ x: 10, y: 20, width: 320 }),
    })

    interactions.onFlowImageAppendToCanvas('image-10')

    const appended = nodes.value[0]?.data?.snapshotNode as ImageNodeItem
    expect(appended.sourceImageUrl).toBe('https://example.test/output.png')
    expect(appended.sourceMediaAssetId).toBe(201)
  })
})
