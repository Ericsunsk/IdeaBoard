import { describe, expect, it } from 'vitest'
import * as v from 'valibot'
import { canvasBoardStateSchema } from './canvasBoardStateSchema'
import { makeCanvasBoardState, makePendingImageNodeSnapshot } from '../testing'

describe('canvasBoardStateSchema', () => {
  it('accepts an empty snapshot', () => {
    const parsed = v.safeParse(canvasBoardStateSchema, makeCanvasBoardState())
    expect(parsed.success).toBe(true)
  })

  it('accepts the minimum shape for every node type', () => {
    const parsed = v.safeParse(
      canvasBoardStateSchema,
      makeCanvasBoardState({
        promptNodes: [
          {
            id: 3,
            type: 'prompt',
            x: 0,
            y: 0,
            width: 200,
            title: 'Custom Prompt',
            text: 'prompt',
            outputCount: 1,
            aspectRatio: 'auto',
            inputImages: [],
          },
        ],
        previewSnapshotVersion: 2,
      }),
    )
    expect(parsed.success).toBe(true)
    if (parsed.success) {
      expect(parsed.output.promptNodes[0]?.title).toBe('Custom Prompt')
      expect(parsed.output.previewSnapshotVersion).toBe(2)
    }
  })

  it('accepts pending task metadata on image nodes', () => {
    const state = makeCanvasBoardState({
      imageNodes: [makePendingImageNodeSnapshot({ pendingTaskId: 'task_pending_2' })],
    })
    const parsed = v.safeParse(canvasBoardStateSchema, state)
    expect(parsed.success).toBe(true)
    if (parsed.success) {
      expect(parsed.output.imageNodes[0]?.pendingTaskId).toBe('task_pending_2')
    }
  })

  it('rejects invalid state', () => {
    const invalid = makeCanvasBoardState() as Record<string, any>
    invalid.connections = [{ id: 1, fromNodeType: 'image', fromNodeId: 1, fromPort: 'image' }]
    invalid.connections[0]!.toNodeType = 'unknown'
    const parsed = v.safeParse(canvasBoardStateSchema, invalid)
    expect(parsed.success).toBe(false)
  })

  it('applies fusion defaults', () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test mutates fields not exposed by the typed interface to simulate partial data
    const state = makeCanvasBoardState({
      fusionNodes: [
        {
          id: 8,
          type: 'fusion',
          x: 0,
          y: 0,
          width: 240,
          prompt: 'fusion prompt',
          outputCount: 1,
          aspectRatio: 'auto',
          blendPoint: { x: 0.5, y: 0.5 },
          inputImages: [],
          outputStatus: 'idle',
          outputImages: [],
        },
      ],
    }) as Record<string, any>
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
      ...makeCanvasBoardState(),
      futureTopLevelField: 'unexpected',
    })
    expect(parsed.success).toBe(false)

    const nestedParsed = v.safeParse(canvasBoardStateSchema, {
      ...makeCanvasBoardState(),
      fusionNodes: [
        {
          id: 8,
          type: 'fusion',
          x: 0,
          y: 0,
          width: 240,
          prompt: 'fusion prompt',
          outputCount: 1,
          aspectRatio: 'auto',
          blendPoint: { x: 0.5, y: 0.5 },
          inputImages: [],
          outputStatus: 'idle',
          outputImages: [],
          futureNestedField: 'unexpected',
        },
      ],
    })
    expect(nestedParsed.success).toBe(false)

    const imageNestedParsed = v.safeParse(canvasBoardStateSchema, {
      ...makeCanvasBoardState(),
      imageNodes: [
        {
          id: 2,
          type: 'image-node',
          x: 0,
          y: 0,
          width: 320,
          sourceImageUrl: 'https://example.com/b.png',
          sourceImageRatio: 1,
          outputStatus: 'ready',
          outputImages: [{ url: 'https://example.com/o1.png', ratio: 1 }],
          futureImageField: 'unexpected',
        },
      ],
    })
    expect(imageNestedParsed.success).toBe(false)
  })
})
