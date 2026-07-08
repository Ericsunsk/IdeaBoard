import { describe, expect, it } from 'vitest'
import { CANVAS_BOARD_STATE_SCHEMA_VERSION } from './canvasNodes'
import { parseBoardStateEnvelopeContract } from './boardStateEnvelopeContract'
import { makeCanvasBoardState } from '../testing'

describe('boardStateEnvelopeContract', () => {
  it('accepts the canonical remote envelope fields', () => {
    const envelope = makeCanvasBoardState({
      updatedAt: 123,
      mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: 'source' }],
      previewSnapshotDataUrl: 'data:image/webp;base64,preview',
      previewSnapshotUpdatedAt: 124,
      previewSnapshotVersion: 2,
    })

    const parsed = parseBoardStateEnvelopeContract(envelope)

    expect(parsed).toEqual(
      expect.objectContaining({
        schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION,
        imageNodes: expect.any(Array),
        promptNodes: expect.any(Array),
        fusionNodes: expect.any(Array),
        connections: expect.any(Array),
        mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: 'source' }],
        previewSnapshotDataUrl: 'data:image/webp;base64,preview',
        previewSnapshotUpdatedAt: 124,
        previewSnapshotVersion: 2,
        updatedAt: 123,
      }),
    )
  })

  it('rejects unknown top-level fields, including viewport', () => {
    expect(
      parseBoardStateEnvelopeContract({
        ...makeCanvasBoardState(),
        futureTopLevelField: true,
      }),
    ).toBeNull()

    expect(
      parseBoardStateEnvelopeContract({
        ...makeCanvasBoardState(),
        viewport: { x: 1, y: 2, zoom: 1 },
      }),
    ).toBeNull()
  })

  it('rejects invalid schema version, graph arrays, metadata, and media refs', () => {
    expect(
      parseBoardStateEnvelopeContract({
        ...makeCanvasBoardState(),
        schemaVersion: 2,
      }),
    ).toBeNull()

    expect(
      parseBoardStateEnvelopeContract({
        ...makeCanvasBoardState(),
        connections: null,
      }),
    ).toBeNull()

    expect(
      parseBoardStateEnvelopeContract({
        ...makeCanvasBoardState(),
        previewSnapshotVersion: 0,
      }),
    ).toBeNull()

    expect(
      parseBoardStateEnvelopeContract({
        ...makeCanvasBoardState(),
        mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1', unknown: true }],
      }),
    ).toBeNull()

    expect(
      parseBoardStateEnvelopeContract({
        ...makeCanvasBoardState(),
        mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: '' }],
      }),
    ).toBeNull()
  })
})
