import { describe, expect, it } from 'vitest'
import { makeBoardState } from '../../testing'
import { BOARD_STATE_SCHEMA_VERSION } from '../../core/document/nodes'
import {
  deserializeRemoteBoardStateEnvelope,
  normalizeBoardState,
  serializeBoardState,
} from './boardStateSerialization'

describe('boardStateSerialization', () => {
  it('rejects the retired preview cache instead of repairing it', () => {
    const legacy = {
      ...makeBoardState({ updatedAt: 200 }),
      previewSnapshotDataUrl: 'data:image/webp;base64,preview',
      previewSnapshotUpdatedAt: 201,
      previewSnapshotVersion: 3,
    }

    // Strict cutover: unknown fields fail validation in every mode. The
    // retired cache is never read back and never stripped back into shape;
    // undecodable local bytes are rejected here and dropped by storage
    // writes, not resurrected.
    expect(normalizeBoardState(legacy)).toBeNull()
    expect(normalizeBoardState(legacy, { mode: 'result', boundary: 'unit-test' }).isOk).toBe(false)
  })

  it('serializes a board session snapshot without local metadata', () => {
    const state = makeBoardState({
      updatedAt: 200,
      mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1' }],
    })

    const snapshot = serializeBoardState(state, 'session-snapshot')

    expect(snapshot.mediaRefs).toEqual([
      { mediaAssetId: 12, refType: 'board_node', refId: 'image-1' },
    ])
    expect(snapshot).not.toHaveProperty('updatedAt')
    expect(snapshot).not.toHaveProperty('viewport')
  })

  it('requires remote envelope updatedAt and rejects viewport', () => {
    const remoteWithoutUpdatedAt = makeBoardState({ updatedAt: 100 })
    delete (remoteWithoutUpdatedAt as Partial<typeof remoteWithoutUpdatedAt>).updatedAt

    expect(deserializeRemoteBoardStateEnvelope(remoteWithoutUpdatedAt)).toBeNull()

    expect(
      deserializeRemoteBoardStateEnvelope({
        ...makeBoardState(),
        viewport: { x: 1, y: 2, zoom: 1 },
      }),
    ).toBeNull()
  })

  it('rejects old-shape persisted state instead of canonicalizing it', () => {
    const olderState = {
      schemaVersion: BOARD_STATE_SCHEMA_VERSION,
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
      groups: [],
      updatedAt: 123,
    }

    const normalized = normalizeBoardState(olderState)

    expect(normalized).toBeNull()
  })

  it('returns recoverable validation context for invalid board state', () => {
    const result = normalizeBoardState(
      {
        ...makeBoardState(),
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

    expect(result.isOk).toBe(false)
    if (result.isOk) return
    expect(result.error).toEqual(
      expect.objectContaining({
        code: 'INVALID_BOARD_STATE',
        boundary: 'unit-test',
        isRecoverable: true,
      }),
    )
    expect(result.error.issues[0]).toEqual(
      expect.objectContaining({
        path: 'imageNodes.0.sourceImageRatio',
      }),
    )
  })

  it('returns recoverable validation context for non-serializable board state', () => {
    const nonSerializableState = makeBoardState() as ReturnType<typeof makeBoardState> & {
      toJSON?: () => unknown
    }
    Object.defineProperty(nonSerializableState, 'toJSON', {
      value: () => {
        throw new Error('serialization failed')
      },
    })

    const result = normalizeBoardState(nonSerializableState, {
      mode: 'result',
      boundary: 'unit-test',
    })

    expect(result.isOk).toBe(false)
    if (result.isOk) return
    expect(result.error).toEqual(
      expect.objectContaining({
        code: 'INVALID_BOARD_STATE',
        boundary: 'unit-test',
        isRecoverable: true,
      }),
    )
    expect(result.error.issues[0]).toEqual(
      expect.objectContaining({
        path: '$',
        message: 'BoardState must be JSON serializable',
      }),
    )
  })
})
