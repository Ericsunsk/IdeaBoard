import { describe, expect, it } from 'vitest'
import type { ImageNodeItem } from '../../../board/domain'
import {
  createPendingLibraryImportFact,
  failLibraryImport,
  materializeLibraryImport,
  retryLibraryImport,
} from './canvasLibraryImportState'

function pendingImage(): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 320,
    sourceImageUrl: '',
    sourceImageRatio: 0.5,
    outputStatus: 'idle',
    outputImages: [],
    libraryImport: createPendingLibraryImportFact(7),
  }
}

describe('CanvasLibraryImportState', () => {
  it('owns pending, failed, retry, and materialized Canvas facts', () => {
    const pending = pendingImage()
    const failed = failLibraryImport(pending, 7, 'Source unavailable')!
    const retried = retryLibraryImport(failed)!
    const materialized = materializeLibraryImport(
      retried,
      7,
      { mediaAssetId: 42, url: 'https://app.example/media/42', width: 400, height: 200 },
      1,
    )!

    expect(failed.libraryImport).toEqual({
      state: 'failed',
      libraryAssetId: 7,
      failureMessage: 'Source unavailable',
    })
    expect(retried.libraryImport).toEqual(createPendingLibraryImportFact(7))
    expect(materialized).toMatchObject({
      sourceMediaAssetId: 42,
      sourceImageUrl: 'https://app.example/media/42',
      sourceImageRatio: 0.5,
      outputStatus: 'ready',
    })
    expect(materialized.libraryImport).toBeUndefined()
  })

  it('rejects transitions from a mismatched durable fact', () => {
    const pending = pendingImage()

    expect(failLibraryImport(pending, 8, 'Source unavailable')).toBeNull()
    expect(
      materializeLibraryImport(
        pending,
        8,
        {
          mediaAssetId: 42,
          url: 'https://app.example/media/42',
          width: 400,
          height: 200,
        },
        1,
      ),
    ).toBeNull()
  })
})
