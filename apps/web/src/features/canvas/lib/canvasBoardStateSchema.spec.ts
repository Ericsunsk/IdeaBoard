import { describe, expect, it } from 'vitest'
import * as v from 'valibot'
import { canvasBoardStateSchema } from './canvasBoardStateSchema'

function createValidState() {
  return {
    imageNodes: [
      {
        id: 2,
        type: 'image-node' as const,
        x: 0,
        y: 0,
        width: 320,
        sourceImageUrl: 'https://example.com/b.png',
        sourceImageRatio: 1,
        outputStatus: 'ready' as const,
        outputImages: [{ url: 'https://example.com/o1.png', ratio: 1 }],
      },
    ],
    promptNodes: [
      {
        id: 3,
        type: 'prompt' as const,
        x: 0,
        y: 0,
        width: 200,
        title: 'Custom Prompt',
        text: 'prompt',
      },
    ],
    fusionNodes: [
      {
        id: 8,
        type: 'fusion' as const,
        x: 0,
        y: 0,
        width: 240,
        prompt: 'fusion prompt',
        outputCount: 1,
        aspectRatio: 'auto' as const,
        blendPoint: { x: 0.5, y: 0.5 },
        inputImages: [],
        outputStatus: 'idle' as const,
        outputImages: [],
      },
    ],
    connections: [
      {
        id: 12,
        fromNodeType: 'image' as const,
        fromNodeId: 2,
        fromPort: 'image' as const,
        toNodeType: 'fusion' as const,
        toNodeId: 8,
        toPort: 'image' as const,
      },
    ],
    viewport: {
      canvasX: 0,
      canvasY: 0,
      zoomLevel: 1,
    },
    previewSnapshotDataUrl: 'data:image/webp;base64,aaa',
    previewSnapshotUpdatedAt: Date.now(),
    previewSnapshotVersion: 2,
    updatedAt: Date.now(),
  }
}

describe('canvasBoardStateSchema', () => {
  it('accepts an empty snapshot', () => {
    const parsed = v.safeParse(canvasBoardStateSchema, {
      imageNodes: [],
      promptNodes: [],
      fusionNodes: [],
      connections: [],
      viewport: { canvasX: 0, canvasY: 0, zoomLevel: 1 },
      updatedAt: 1,
    })
    expect(parsed.success).toBe(true)
  })

  it('accepts the minimum shape for every node type', () => {
    const parsed = v.safeParse(canvasBoardStateSchema, createValidState())
    expect(parsed.success).toBe(true)
    if (parsed.success) {
      expect(parsed.output.promptNodes[0]?.title).toBe('Custom Prompt')
      expect(parsed.output.previewSnapshotVersion).toBe(2)
    }
  })

  it('accepts pending task metadata on image nodes', () => {
    const state = createValidState() as Record<string, any>
    state.imageNodes[0] = {
      ...state.imageNodes[0]!,
      sourceImageUrl: '',
      pendingTaskId: 'task_pending_2',
      outputStatus: 'running',
      outputImages: [],
    }
    const parsed = v.safeParse(canvasBoardStateSchema, state)
    expect(parsed.success).toBe(true)
    if (parsed.success) {
      expect(parsed.output.imageNodes[0]?.pendingTaskId).toBe('task_pending_2')
    }
  })

  it('rejects invalid state', () => {
    const invalid = createValidState()
    invalid.connections[0]!.toNodeType = 'unknown' as any
    const parsed = v.safeParse(canvasBoardStateSchema, invalid)
    expect(parsed.success).toBe(false)
  })

  it('applies fusion defaults', () => {
    const state = createValidState() as Record<string, any>
    state.fusionNodes = [{ ...state.fusionNodes[0] }]
    delete state.fusionNodes[0].blendPoint
    delete state.fusionNodes[0].aspectRatio
    const parsed = v.safeParse(canvasBoardStateSchema, state)
    expect(parsed.success).toBe(true)
    if (parsed.success) {
      expect(parsed.output.fusionNodes[0]?.blendPoint.x).toBe(0.5)
      expect(parsed.output.fusionNodes[0]?.blendPoint.y).toBe(0.5)
      expect(parsed.output.fusionNodes[0]?.aspectRatio).toBe('auto')
    }
  })

  it('rejects unknown fields', () => {
    const parsed = v.safeParse(canvasBoardStateSchema, {
      ...createValidState(),
      futureTopLevelField: 'unexpected',
    })
    expect(parsed.success).toBe(false)

    const nestedParsed = v.safeParse(canvasBoardStateSchema, {
      ...createValidState(),
      fusionNodes: [
        {
          ...createValidState().fusionNodes[0],
          futureNestedField: 'unexpected',
        },
      ],
    })
    expect(nestedParsed.success).toBe(false)

    const imageNestedParsed = v.safeParse(canvasBoardStateSchema, {
      ...createValidState(),
      imageNodes: [
        {
          ...createValidState().imageNodes[0],
          futureImageField: 'unexpected',
        },
      ],
    })
    expect(imageNestedParsed.success).toBe(false)
  })
})
