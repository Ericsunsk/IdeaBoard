import { describe, expect, it } from 'vitest'
import type { ImageNodeItem } from '../../../board'
import {
  createPendingLibraryImportFact,
  failLibraryImport,
  materializeLibraryImport,
  retryLibraryImport,
} from './libraryImportState'

const LIBRARY_ASSET_ID = 'lasset_asset0007'

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
    libraryImport: createPendingLibraryImportFact(LIBRARY_ASSET_ID),
  }
}

describe('CanvasLibraryImportState', () => {
  it('owns pending, failed, retry, and materialized Canvas facts', () => {
    const pending = pendingImage()
    const failed = failLibraryImport(pending, LIBRARY_ASSET_ID, 'Source unavailable')!
    const retried = retryLibraryImport(failed)!
    const materialized = materializeLibraryImport(
      retried,
      LIBRARY_ASSET_ID,
      {
        mediaAssetId: 'asset_media0042',
        url: 'https://app.example/media/42',
        width: 400,
        height: 200,
      },
      1,
    )!

    expect(failed.libraryImport).toEqual({
      state: 'failed',
      libraryAssetId: LIBRARY_ASSET_ID,
      failureMessage: 'Source unavailable',
    })
    expect(retried.libraryImport).toEqual(createPendingLibraryImportFact(LIBRARY_ASSET_ID))
    expect(materialized).toMatchObject({
      sourceMediaRef: { assetId: 'asset_media0042', derivativeTier: 'preview' },
      sourceImageUrl: 'https://app.example/media/42',
      sourceImageRatio: 0.5,
      outputStatus: 'ready',
    })
    expect(materialized.sourceMediaAssetId).toBeUndefined()
    expect(materialized.libraryImport).toBeUndefined()
  })

  it('rejects transitions from a mismatched durable fact', () => {
    const pending = pendingImage()

    expect(failLibraryImport(pending, 'lasset_asset0008', 'Source unavailable')).toBeNull()
    expect(
      materializeLibraryImport(
        pending,
        'lasset_asset0008',
        {
          mediaAssetId: 'asset_media0042',
          url: 'https://app.example/media/42',
          width: 400,
          height: 200,
        },
        1,
      ),
    ).toBeNull()
  })
})
