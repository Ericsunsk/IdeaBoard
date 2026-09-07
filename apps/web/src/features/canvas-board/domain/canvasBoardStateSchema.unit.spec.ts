import { describe, expect, it } from 'vitest'
import * as v from 'valibot'
import { canvasBoardStateSchema } from './canvasBoardStateSchema'
import { createTypedCanvasNodeSnapshot } from './canvasNodeRegistry'
import { CANVAS_BOARD_STATE_SCHEMA_VERSION, type CanvasGroup } from './canvasNodes'
import { normalizeLatestCanvasBoardState } from './latestBoardState'
import { makeCanvasBoardState, makePendingImageNodeSnapshot } from '../testing'

function makeCanvasGroup(overrides: Partial<CanvasGroup> = {}): CanvasGroup {
  return {
    id: 'group-1',
    nodeIds: ['image-1', 'prompt-1'],
    x: 20,
    y: 20,
    width: 700,
    height: 360,
    ...overrides,
  }
}

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

  it('accepts canonical node snapshots with output resolution at the board-save boundary', () => {
    const imageNode = {
      ...createTypedCanvasNodeSnapshot('image', { x: 12, y: 24 }, 1),
      title: 'dropped-image.png',
      sourceImageUrl: 'https://example.com/dropped-image.png',
      sourceMediaAssetId: 101,
      sourceImageRatio: 1.5,
      outputStatus: 'ready' as const,
      outputImages: [],
    }
    const state = makeCanvasBoardState({
      imageNodes: [imageNode],
      promptNodes: [createTypedCanvasNodeSnapshot('prompt', { x: 320, y: 24 }, 2)],
      fusionNodes: [createTypedCanvasNodeSnapshot('fusion', { x: 640, y: 24 }, 3)],
    })

    const result = normalizeLatestCanvasBoardState(state, {
      mode: 'result',
      boundary: 'board-save',
    })

    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.value.imageNodes[0]?.outputResolution).toBe('1k')
      expect(result.value.promptNodes[0]?.outputResolution).toBe('1k')
      expect(result.value.fusionNodes[0]?.outputResolution).toBe('1k')
    }
  })

  it('accepts pending task metadata on image nodes', () => {
    const state = makeCanvasBoardState({
      imageNodes: [
        makePendingImageNodeSnapshot({ pendingTaskId: 'task_pending_2', pendingResultIndex: 2 }),
      ],
    })
    const parsed = v.safeParse(canvasBoardStateSchema, state)
    expect(parsed.success).toBe(true)
    if (parsed.success) {
      expect(parsed.output.imageNodes[0]?.pendingTaskId).toBe('task_pending_2')
      expect(parsed.output.imageNodes[0]?.pendingResultIndex).toBe(2)
    }
  })

  it('accepts only durable Library import facts on image nodes', () => {
    const state = makeCanvasBoardState({
      imageNodes: [
        {
          ...makePendingImageNodeSnapshot(),
          libraryImport: { state: 'failed', libraryAssetId: 7, failureMessage: '素材导入失败' },
        },
      ],
    })

    const parsed = v.safeParse(canvasBoardStateSchema, state)
    expect(parsed.success).toBe(true)
    if (parsed.success) {
      expect(parsed.output.imageNodes[0]?.libraryImport).toEqual({
        state: 'failed',
        libraryAssetId: 7,
        failureMessage: '素材导入失败',
      })
    }
  })

  it('rejects transient provider URLs inside Library import facts', () => {
    const validState = makeCanvasBoardState({ imageNodes: [makePendingImageNodeSnapshot()] })
    const state = {
      ...validState,
      imageNodes: validState.imageNodes.map((node) => ({
        ...node,
        libraryImport: {
          state: 'pending',
          libraryAssetId: 7,
          previewUrl: 'https://provider.example/preview.png',
        },
      })),
    }

    expect(v.safeParse(canvasBoardStateSchema, state).success).toBe(false)
  })

  it('preserves media asset ids on node output images', () => {
    const state = makeCanvasBoardState({
      imageNodes: [
        {
          id: 1,
          type: 'image-node',
          x: 0,
          y: 0,
          width: 280,
          sourceImageUrl: 'https://example.com/source.png',
          sourceImageRatio: 1,
          outputStatus: 'ready',
          outputImages: [
            { url: 'https://example.com/image-output.png', ratio: 1, mediaAssetId: 201 },
          ],
        },
      ],
      fusionNodes: [
        {
          id: 2,
          type: 'fusion',
          x: 320,
          y: 0,
          width: 280,
          prompt: '',
          outputCount: 1,
          aspectRatio: 'auto',
          blendPoint: { x: 0.5, y: 0.5 },
          inputImages: [],
          outputStatus: 'ready',
          outputImages: [
            { url: 'https://example.com/fusion-output.png', ratio: 1, mediaAssetId: 202 },
          ],
        },
      ],
    })

    const parsed = v.safeParse(canvasBoardStateSchema, state)

    expect(parsed.success).toBe(true)
    if (parsed.success) {
      expect(parsed.output.imageNodes[0]?.outputImages[0]?.mediaAssetId).toBe(201)
      expect(parsed.output.fusionNodes[0]?.outputImages[0]?.mediaAssetId).toBe(202)
    }
  })

  it('accepts persistent canvas groups', () => {
    const state = makeCanvasBoardState({
      groups: [
        makeCanvasGroup(),
        makeCanvasGroup({ id: 'group-2', nodeIds: ['image-1'] }),
        makeCanvasGroup({ id: 'group-3', nodeIds: [] }),
      ],
    })

    const parsed = v.safeParse(canvasBoardStateSchema, state)

    expect(parsed.success).toBe(true)
    if (parsed.success) {
      expect(parsed.output.groups).toEqual([
        makeCanvasGroup(),
        makeCanvasGroup({ id: 'group-2', nodeIds: ['image-1'] }),
        makeCanvasGroup({ id: 'group-3', nodeIds: [] }),
      ])
    }
  })

  it('accepts persistent canvas groups that reference decimal draft node ids', () => {
    const state = makeCanvasBoardState({
      promptNodes: [
        {
          id: 1000.25,
          type: 'prompt',
          x: 0,
          y: 0,
          width: 200,
          text: 'draft prompt',
          outputCount: 1,
          aspectRatio: 'auto',
          inputImages: [],
        },
        {
          id: 1001.5,
          type: 'prompt',
          x: 320,
          y: 0,
          width: 200,
          text: 'draft prompt 2',
          outputCount: 1,
          aspectRatio: 'auto',
          inputImages: [],
        },
      ],
      groups: [makeCanvasGroup({ nodeIds: ['prompt-1000.25', 'prompt-1001.5'] })],
    })

    const parsed = v.safeParse(canvasBoardStateSchema, state)

    expect(parsed.success).toBe(true)
    if (parsed.success) {
      expect(parsed.output.groups).toEqual([
        makeCanvasGroup({ nodeIds: ['prompt-1000.25', 'prompt-1001.5'] }),
      ])
    }
  })

  it('rejects invalid canvas groups', () => {
    const parsed = v.safeParse(
      canvasBoardStateSchema,
      makeCanvasBoardState({
        groups: [makeCanvasGroup()],
      }),
    )
    expect(parsed.success).toBe(true)

    const invalidIdParsed = v.safeParse(canvasBoardStateSchema, {
      ...makeCanvasBoardState(),
      groups: [{ ...makeCanvasGroup(), id: 'group-a' }],
    })
    expect(invalidIdParsed.success).toBe(false)

    const invalidNodeIdParsed = v.safeParse(canvasBoardStateSchema, {
      ...makeCanvasBoardState(),
      groups: [{ ...makeCanvasGroup(), nodeIds: ['node-1'] }],
    })
    expect(invalidNodeIdParsed.success).toBe(false)

    const missingGeometryParsed = v.safeParse(canvasBoardStateSchema, {
      ...makeCanvasBoardState(),
      groups: [{ id: 'group-1', nodeIds: ['image-1', 'prompt-1'] }],
    })
    expect(missingGeometryParsed.success).toBe(false)

    const invalidGeometryParsed = v.safeParse(canvasBoardStateSchema, {
      ...makeCanvasBoardState(),
      groups: [makeCanvasGroup({ width: 0 })],
    })
    expect(invalidGeometryParsed.success).toBe(false)

    const nonFiniteGeometryParsed = v.safeParse(canvasBoardStateSchema, {
      ...makeCanvasBoardState(),
      groups: [makeCanvasGroup({ x: Number.NaN })],
    })
    expect(nonFiniteGeometryParsed.success).toBe(false)
  })

  it('rejects non-migratable old schema versions during latest-state normalization', () => {
    const parsed = normalizeLatestCanvasBoardState({
      ...makeCanvasBoardState(),
      schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION - 2,
    })

    expect(parsed).toBeNull()
  })

  it('migrates v3 adjustmentRecipe to retouchRecipe during latest-state normalization', () => {
    const recipe = {
      version: 1,
      sourceMediaAssetId: 42,
      plan: [{ kind: 'threshold', threshold: 0.5, invert: false }],
    }
    const parsed = normalizeLatestCanvasBoardState({
      ...makeCanvasBoardState(),
      schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION - 1,
      imageNodes: [
        {
          id: 1,
          type: 'image-node',
          x: 0,
          y: 0,
          width: 280,
          sourceImageUrl: 'https://example.com/source.png',
          sourceImageRatio: 1,
          outputStatus: 'ready',
          outputImages: [{ url: 'https://example.com/output.png', ratio: 1 }],
          // Intentionally legacy: v3 payloads carry the pre-rename field.
          adjustmentRecipe: recipe,
        },
      ],
    })

    expect(parsed).not.toBeNull()
    const imageNode = parsed?.imageNodes[0]
    expect(parsed?.schemaVersion).toBe(CANVAS_BOARD_STATE_SCHEMA_VERSION)
    expect(imageNode).toMatchObject({ retouchRecipe: recipe })
    expect(imageNode).not.toHaveProperty('adjustmentRecipe')
  })

  it('migrates v3 payloads without a recipe during latest-state normalization', () => {
    const parsed = normalizeLatestCanvasBoardState({
      ...makeCanvasBoardState(),
      schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION - 1,
    })

    expect(parsed).not.toBeNull()
    expect(parsed?.schemaVersion).toBe(CANVAS_BOARD_STATE_SCHEMA_VERSION)
  })

  it('rejects stale adjustmentRecipe keys on current-version payloads', () => {
    const parsed = normalizeLatestCanvasBoardState({
      ...makeCanvasBoardState(),
      imageNodes: [
        {
          id: 1,
          type: 'image-node',
          x: 0,
          y: 0,
          width: 280,
          sourceImageUrl: 'https://example.com/source.png',
          sourceImageRatio: 1,
          outputStatus: 'ready',
          outputImages: [{ url: 'https://example.com/output.png', ratio: 1 }],
          // Intentionally stale: current-version payloads must not carry it.
          adjustmentRecipe: {
            version: 1,
            sourceMediaAssetId: 42,
            plan: [],
          },
        },
      ],
    })

    expect(parsed).toBeNull()
  })

  it('rejects missing schema versions during latest-state normalization', () => {
    const { schemaVersion: _schemaVersion, ...stateWithoutVersion } = makeCanvasBoardState()

    const parsed = normalizeLatestCanvasBoardState(stateWithoutVersion)

    expect(parsed).toBeNull()
  })

  it('rejects old node shape during latest-state normalization', () => {
    const parsed = normalizeLatestCanvasBoardState({
      ...makeCanvasBoardState(),
      imageNodes: [
        {
          id: 1,
          type: 'image-node',
          x: 0,
          y: 0,
          width: 280,
          sourceImageUrl: 'https://example.com/source.png',
          sourceImageRatio: 1,
          outputImages: [{ url: 'https://example.com/output.png', ratio: 1 }],
        },
      ],
    })

    expect(parsed).toBeNull()
  })

  it('rejects old group shape during latest-state normalization', () => {
    const parsed = normalizeLatestCanvasBoardState({
      ...makeCanvasBoardState(),
      groups: [{ id: 'group-1', nodeIds: ['image-1', 'prompt-1'] }],
    })

    expect(parsed).toBeNull()
  })

  it('rejects invalid state', () => {
    const parsed = v.safeParse(canvasBoardStateSchema, {
      ...makeCanvasBoardState(),
      connections: [
        {
          id: 1,
          fromNodeType: 'image',
          fromNodeId: 1,
          fromPort: 'image',
          toNodeType: 'unknown',
        },
      ],
    })
    expect(parsed.success).toBe(false)
  })

  it('applies fusion defaults', () => {
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
    })
    const [fusionNode] = state.fusionNodes
    const {
      aspectRatio: _aspectRatio,
      blendPoint: _blendPoint,
      ...fusionNodeWithoutDefaults
    } = fusionNode!
    const parsed = v.safeParse(canvasBoardStateSchema, {
      ...state,
      fusionNodes: [fusionNodeWithoutDefaults],
    })
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
