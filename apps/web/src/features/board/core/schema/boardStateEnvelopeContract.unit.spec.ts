import { describe, expect, it } from 'vitest'
import { BOARD_STATE_SCHEMA_VERSION } from '../document/nodes'
import { parseBoardStateEnvelopeContract } from './boardStateEnvelopeContract'
import { makeBoardState } from '../../testing'

describe('boardStateEnvelopeContract', () => {
  it('accepts the canonical remote envelope fields', () => {
    const envelope = makeBoardState({
      updatedAt: 123,
      mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: 'source' }],
    })

    const parsed = parseBoardStateEnvelopeContract(envelope)

    expect(parsed).toEqual(
      expect.objectContaining({
        schemaVersion: BOARD_STATE_SCHEMA_VERSION,
        imageNodes: expect.any(Array),
        promptNodes: expect.any(Array),
        fusionNodes: expect.any(Array),
        connections: expect.any(Array),
        mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: 'source' }],
        updatedAt: 123,
      }),
    )
    expect(parsed).not.toHaveProperty('previewSnapshotDataUrl')
  })

  it('rejects unknown top-level fields, including viewport', () => {
    expect(
      parseBoardStateEnvelopeContract({
        ...makeBoardState(),
        futureTopLevelField: true,
      }),
    ).toBeNull()

    expect(
      parseBoardStateEnvelopeContract({
        ...makeBoardState(),
        viewport: { x: 1, y: 2, zoom: 1 },
      }),
    ).toBeNull()
  })

  it('rejects the retired preview cache instead of carrying it', () => {
    const parsed = parseBoardStateEnvelopeContract({
      ...makeBoardState(),
      previewSnapshotDataUrl: 'data:image/webp;base64,preview',
      previewSnapshotUpdatedAt: 124,
      previewSnapshotVersion: 2,
    })

    expect(parsed).toBeNull()
  })

  it('rejects invalid schema version, graph arrays, and media refs', () => {
    expect(
      parseBoardStateEnvelopeContract({
        ...makeBoardState(),
        schemaVersion: BOARD_STATE_SCHEMA_VERSION + 1,
      }),
    ).toBeNull()

    expect(
      parseBoardStateEnvelopeContract({
        ...makeBoardState(),
        connections: null,
      }),
    ).toBeNull()

    expect(
      parseBoardStateEnvelopeContract({
        ...makeBoardState(),
        mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1', unknown: true }],
      }),
    ).toBeNull()

    expect(
      parseBoardStateEnvelopeContract({
        ...makeBoardState(),
        mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: '' }],
      }),
    ).toBeNull()
  })
})
