import { describe, expect, it } from 'vitest'
import { makeCanvasBoardState } from '../testing'
import {
  CANVAS_BOARD_STATE_SCHEMA_VERSION,
  DEFAULT_FUSION_BLEND_POINT,
  DEFAULT_FUSION_NODE_OUTPUT_COUNT,
  DEFAULT_PROMPT_ASPECT_RATIO,
  DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
} from '../domain'
import {
  deserializeRemoteBoardStateEnvelope,
  normalizeCanvasBoardState,
  serializeCanvasBoardState,
} from './boardStateSerialization'

describe('boardStateSerialization', () => {
  it('serializes a local preview fallback without preview data', () => {
    const state = makeCanvasBoardState({
      updatedAt: 200,
      mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: 'source' }],
      previewSnapshotDataUrl: 'data:image/webp;base64,preview',
      previewSnapshotUpdatedAt: 201,
      previewSnapshotVersion: 3,
    })

    const fallback = serializeCanvasBoardState(state, 'local-preview-fallback')

    expect(fallback.updatedAt).toBe(200)
    expect(fallback.mediaRefs).toEqual([
      { mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: 'source' },
    ])
    expect(fallback).not.toHaveProperty('previewSnapshotDataUrl')
    expect(fallback).not.toHaveProperty('previewSnapshotUpdatedAt')
    expect(fallback).not.toHaveProperty('previewSnapshotVersion')
    expect(fallback).not.toHaveProperty('viewport')
  })

  it('serializes a board session snapshot without local or preview metadata', () => {
    const state = makeCanvasBoardState({
      updatedAt: 200,
      mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1' }],
      previewSnapshotDataUrl: 'data:image/webp;base64,preview',
      previewSnapshotUpdatedAt: 201,
      previewSnapshotVersion: 3,
    })

    const snapshot = serializeCanvasBoardState(state, 'session-snapshot')

    expect(snapshot.mediaRefs).toEqual([
      { mediaAssetId: 12, refType: 'board_node', refId: 'image-1' },
    ])
    expect(snapshot).not.toHaveProperty('updatedAt')
    expect(snapshot).not.toHaveProperty('previewSnapshotDataUrl')
    expect(snapshot).not.toHaveProperty('viewport')
  })

  it('requires remote envelope updatedAt and rejects viewport', () => {
    const remoteWithoutUpdatedAt = makeCanvasBoardState({ updatedAt: 100 })
    delete (remoteWithoutUpdatedAt as Partial<typeof remoteWithoutUpdatedAt>).updatedAt

    expect(deserializeRemoteBoardStateEnvelope(remoteWithoutUpdatedAt)).toBeNull()

    expect(
      deserializeRemoteBoardStateEnvelope({
        ...makeCanvasBoardState(),
        viewport: { x: 1, y: 2, zoom: 1 },
      }),
    ).toBeNull()
  })

  it('canonicalizes normalizable older persisted state to the latest schema', () => {
    const olderState = {
      schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION,
      imageNodes: [
        {
          id: 7,
          type: 'image-node',
          x: 10,
          y: 20,
          width: 320,
          sourceImageUrl: 'https://example.com/source.png',
          sourceImageRatio: 1,
          outputImages: [{ url: 'https://example.com/out.png', ratio: 1 }],
        },
      ],
      promptNodes: [
        {
          id: 8,
          type: 'prompt',
          x: 30,
          y: 40,
          width: 280,
          text: 'older prompt',
        },
      ],
      fusionNodes: [
        {
          id: 9,
          type: 'fusion',
          x: 50,
          y: 60,
          width: 300,
          outputStatus: 'idle',
          outputImages: [],
        },
      ],
      connections: [],
      updatedAt: 123,
    }

    const normalized = normalizeCanvasBoardState(olderState)

    expect(normalized).toEqual(
      expect.objectContaining({
        schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION,
        updatedAt: 123,
      }),
    )
    expect(normalized?.imageNodes[0]).toEqual(
      expect.objectContaining({
        outputStatus: 'ready',
        outputImages: [{ url: 'https://example.com/out.png', ratio: 1 }],
      }),
    )
    expect(normalized?.promptNodes[0]).toEqual(
      expect.objectContaining({
        outputCount: DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
        aspectRatio: DEFAULT_PROMPT_ASPECT_RATIO,
        inputImages: [],
      }),
    )
    expect(normalized?.fusionNodes[0]).toEqual(
      expect.objectContaining({
        prompt: '',
        outputCount: DEFAULT_FUSION_NODE_OUTPUT_COUNT,
        aspectRatio: DEFAULT_PROMPT_ASPECT_RATIO,
        blendPoint: DEFAULT_FUSION_BLEND_POINT,
        inputImages: [],
      }),
    )
  })

  it('returns recoverable validation context for invalid board state', () => {
    const result = normalizeCanvasBoardState(
      {
        ...makeCanvasBoardState(),
        imageNodes: [
          {
            id: 1,
            type: 'image-node',
            x: 0,
            y: 0,
            width: 280,
            sourceImageUrl: 'https://example.com/source.png',
            sourceImageRatio: 0,
            outputStatus: 'ready',
            outputImages: [],
          },
        ],
      },
      { mode: 'result', boundary: 'unit-test' },
    )

    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error).toEqual(
      expect.objectContaining({
        code: 'INVALID_BOARD_STATE',
        boundary: 'unit-test',
        recoverable: true,
      }),
    )
    expect(result.error.issues[0]).toEqual(
      expect.objectContaining({
        path: 'imageNodes.0.sourceImageRatio',
      }),
    )
  })
})
