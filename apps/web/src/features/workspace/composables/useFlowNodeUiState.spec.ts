import { describe, expect, it } from 'vitest'
import { shallowRef } from 'vue'
import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { FusionNodeItem, ImageNodeItem } from '../../canvas-board/domain'
import type { FlowNodeData } from '../types/flowTypes'
import { useFlowNodeUiState } from './useFlowNodeUiState'

function makeImageSnapshot(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    title: 'Image',
    sourceImageUrl: 'https://example.com/source.png',
    sourceImageRatio: 1,
    outputStatus: 'ready',
    outputImages: [],
    ...overrides,
  }
}

function makeFusionSnapshot(overrides: Partial<FusionNodeItem> = {}): FusionNodeItem {
  return {
    id: 2,
    type: 'fusion',
    x: 320,
    y: 0,
    width: 286,
    title: 'Fusion',
    prompt: '',
    outputCount: 1,
    aspectRatio: '1:1',
    blendPoint: { x: 0.5, y: 0.5 },
    inputImages: [],
    outputStatus: 'idle',
    outputImages: [],
    ...overrides,
  }
}

function makeFlowNode(
  id: string,
  kind: FlowNodeData['kind'],
  snapshotNode: ImageNodeItem | FusionNodeItem,
): Node<FlowNodeData> {
  return {
    id,
    type: 'canvas',
    position: { x: snapshotNode.x, y: snapshotNode.y },
    data: {
      kind,
      nodeType: kind,
      title: snapshotNode.title || kind,
      snapshotNode,
    },
  }
}

describe('useFlowNodeUiState', () => {
  it('treats an image node latest output as ready when its source url is empty', () => {
    const nodes: Ref<Node[]> = shallowRef([
      makeFlowNode(
        'image-1',
        'image',
        makeImageSnapshot({
          sourceImageUrl: '',
          outputStatus: 'ready',
          outputImages: [{ url: 'https://example.com/generated.png', ratio: 1.4 }],
        }),
      ),
      makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot()),
    ] as Node[])
    const edges: Ref<Edge[]> = shallowRef([
      {
        id: 'edge-1',
        source: 'image-1',
        sourceHandle: 'out:image',
        target: 'fusion-1',
        targetHandle: 'in:image',
      },
    ] as Edge[])

    const { getFlowNodeUiStateCached } = useFlowNodeUiState({ nodes, edges })
    const fusionState = getFlowNodeUiStateCached('fusion-1')
    const pendingInputs = fusionState.connection.pending as
      | Record<string, boolean | undefined>
      | undefined

    expect(pendingInputs?.image).toBe(false)
    expect(fusionState.primaryAction).toEqual({ label: '融合', disabled: false })
  })
})
