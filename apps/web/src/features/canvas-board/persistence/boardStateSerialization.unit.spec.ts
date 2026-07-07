import { describe, expect, it } from 'vitest'
import { makeCanvasBoardState } from '../testing'
import {
  deserializeRemoteBoardStateEnvelope,
  serializeCanvasBoardState,
} from './boardStateSerialization'

describe('boardStateSerialization', () => {
  it('serializes a local preview fallback without preview data', () => {
    const state = makeCanvasBoardState({
      updatedAt: 200,
      mediaRefs: [{ url: 'https://example.com/source.png', role: 'source' }],
      previewSnapshotDataUrl: 'data:image/webp;base64,preview',
      previewSnapshotUpdatedAt: 201,
      previewSnapshotVersion: 3,
    })

    const fallback = serializeCanvasBoardState(state, 'local-preview-fallback')

    expect(fallback.updatedAt).toBe(200)
    expect(fallback.mediaRefs).toEqual([{ url: 'https://example.com/source.png', role: 'source' }])
    expect(fallback).not.toHaveProperty('previewSnapshotDataUrl')
    expect(fallback).not.toHaveProperty('previewSnapshotUpdatedAt')
    expect(fallback).not.toHaveProperty('previewSnapshotVersion')
    expect(fallback).not.toHaveProperty('viewport')
  })

  it('serializes a board session snapshot without local or preview metadata', () => {
    const state = makeCanvasBoardState({
      updatedAt: 200,
      mediaRefs: [{ url: 'https://example.com/source.png' }],
      previewSnapshotDataUrl: 'data:image/webp;base64,preview',
      previewSnapshotUpdatedAt: 201,
      previewSnapshotVersion: 3,
    })

    const snapshot = serializeCanvasBoardState(state, 'session-snapshot')

    expect(snapshot.mediaRefs).toEqual([{ url: 'https://example.com/source.png' }])
    expect(snapshot).not.toHaveProperty('updatedAt')
    expect(snapshot).not.toHaveProperty('previewSnapshotDataUrl')
    expect(snapshot).not.toHaveProperty('viewport')
  })

  it('requires remote envelope updatedAt and rejects viewport', () => {
    const remoteWithoutUpdatedAt = makeCanvasBoardState({ updatedAt: 100 })
    delete (remoteWithoutUpdatedAt as Partial<typeof remoteWithoutUpdatedAt>).updatedAt

    expect(deserializeRemoteBoardStateEnvelope(remoteWithoutUpdatedAt)).toBeNull()

    expect(
      deserializeRemoteBoardStateEnvelope(
        {
          ...makeCanvasBoardState(),
          viewport: { x: 1, y: 2, zoom: 1 },
        },
      ),
    ).toBeNull()
  })
})
