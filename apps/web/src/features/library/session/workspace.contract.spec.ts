import { mediaDisplayFixture } from '../../../test/mediaDisplay'
import { describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import {
  assertLibraryAssetId,
  assertLibraryFolderId,
  assertWorkspaceId,
  libraryAssetKeyFor,
  mapWorkspaceLibraryProvider,
  toWorkspaceLibraryAsset,
  toWorkspaceLibraryAssetInput,
  toWorkspaceLibraryFolder,
  toWorkspaceLibraryMediaAssetInput,
  toWorkspaceLibraryResolveResult,
} from '../core/validators'
import { WORKSPACE_LIBRARY_RESOLVE_MAX_ITEMS, workspaceLibraryApi } from './resource'
import {
  lookupWorkspaceSavedAssetKeys,
  materializeLibraryAssetForCanvas,
  removeWorkspaceSearchReference,
} from './libraryUseCases'
import {
  getDeleteWorkspaceLibraryAssetUrl,
  getDeleteWorkspaceLibraryFolderUrl,
  getListWorkspaceLibraryAssetsUrl,
  getListWorkspaceLibraryFoldersUrl,
  getMoveWorkspaceLibraryAssetUrl,
  getRenameWorkspaceLibraryFolderUrl,
  getResolveWorkspaceLibraryAssetsUrl,
  getSaveWorkspaceLibraryAssetUrl,
} from '../../../api/generated-query/public/workspace-library/workspace-library'
import { getWorkspaceMediaAssetMetadata } from '@/api/media/mediaAssetDelivery'

vi.mock('@/api/media/mediaAssetDelivery', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/media/mediaAssetDelivery')>()
  return {
    ...actual,
    getWorkspaceMediaAssetMetadata: vi.fn(async () => ({
      asset_id: 'asset_media0001',
      status: 'ready',
      mime_type: 'image/png',
      width: 400,
      height: 200,
      size_bytes: 1234,
      original: {
        status: 'ready',
        url: '/api/v1/workspaces/ws_abc123/media-assets/asset_media0001/content',
      },
      derivatives: {},
    })),
  }
})

const mediaMetadataMock = vi.mocked(getWorkspaceMediaAssetMetadata)

const WORKSPACE_ID = 'ws_abc123'
const FOLDER_ID = 'lfolder_folder0001'
const ASSET_ID = 'lasset_asset0001'
const PREVIEW_URL = 'https://images.unsplash.com/photo-test'

function folderFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: FOLDER_ID,
    workspace_id: WORKSPACE_ID,
    name: 'inspo',
    created_at: '2026-09-23T00:00:00Z',
    updated_at: '2026-09-23T00:00:00Z',
    ...overrides,
  }
}

function assetFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: ASSET_ID,
    workspace_id: WORKSPACE_ID,
    folder_id: FOLDER_ID,
    source: 'external',
    provider: 'unsplash',
    source_id: 'u1',
    source_url: PREVIEW_URL,
    preview_url: PREVIEW_URL,
    original_url: PREVIEW_URL,
    title: 'dunes',
    author: 'a',
    width: 100,
    height: 80,
    created_at: '2026-09-23T00:00:00Z',
    display: mediaDisplayFixture(),
    updated_at: '2026-09-23T00:00:00Z',
    ...overrides,
  }
}

function mediaAssetFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 'lasset_media0001',
    workspace_id: WORKSPACE_ID,
    folder_id: FOLDER_ID,
    source: 'media',
    media_asset_id: 'asset_media0001',
    title: 'Generated hero banner',
    width: 1600,
    height: 900,
    created_at: '2026-10-11T09:30:00Z',
    display: mediaDisplayFixture(),
    updated_at: '2026-10-11T09:30:00Z',
    ...overrides,
  }
}

describe('workspace library paths', () => {
  it('builds canonical routes matching the generated hooks (publicMutator adds /v1)', () => {
    expect(getListWorkspaceLibraryFoldersUrl(WORKSPACE_ID)).toBe(
      '/workspaces/ws_abc123/library/folders',
    )
    expect(getRenameWorkspaceLibraryFolderUrl(WORKSPACE_ID, FOLDER_ID)).toBe(
      '/workspaces/ws_abc123/library/folders/lfolder_folder0001',
    )
    expect(getDeleteWorkspaceLibraryFolderUrl(WORKSPACE_ID, FOLDER_ID)).toBe(
      '/workspaces/ws_abc123/library/folders/lfolder_folder0001',
    )
    expect(getListWorkspaceLibraryAssetsUrl(WORKSPACE_ID)).toBe(
      '/workspaces/ws_abc123/library/assets',
    )
    expect(getSaveWorkspaceLibraryAssetUrl(WORKSPACE_ID)).toBe(
      '/workspaces/ws_abc123/library/assets',
    )
    expect(getMoveWorkspaceLibraryAssetUrl(WORKSPACE_ID, ASSET_ID)).toBe(
      '/workspaces/ws_abc123/library/assets/lasset_asset0001',
    )
    expect(getDeleteWorkspaceLibraryAssetUrl(WORKSPACE_ID, ASSET_ID)).toBe(
      '/workspaces/ws_abc123/library/assets/lasset_asset0001',
    )
    expect(getResolveWorkspaceLibraryAssetsUrl(WORKSPACE_ID)).toBe(
      '/workspaces/ws_abc123/library/asset-resolutions',
    )
  })

  it('rejects malformed ids before any network call (integer ids included)', async () => {
    expect(() => assertWorkspaceId('nope')).toThrow()
    expect(() => assertLibraryFolderId('42')).toThrow()
    expect(() => assertLibraryFolderId('lasset_asset0001')).toThrow()
    expect(() => assertLibraryAssetId('42')).toThrow()
    expect(() => assertLibraryAssetId(ASSET_ID)).not.toThrow()
    await expect(
      workspaceLibraryApi.moveAsset(WORKSPACE_ID, 'asset with space', null),
    ).rejects.toThrow()
  })
})

describe('workspace library mapping', () => {
  it('maps folders and assets snake_case to camelCase', () => {
    expect(toWorkspaceLibraryFolder(folderFixture())).toMatchObject({
      id: FOLDER_ID,
      workspaceId: WORKSPACE_ID,
      name: 'inspo',
    })
    expect(toWorkspaceLibraryAsset(assetFixture())).toMatchObject({
      source: 'external',
      id: ASSET_ID,
      folderId: FOLDER_ID,
      provider: 'unsplash',
      sourceId: 'u1',
      previewUrl: PREVIEW_URL,
      width: 100,
    })
    expect(toWorkspaceLibraryAsset(assetFixture({ folder_id: '' })).folderId).toBeUndefined()
    expect(() => mapWorkspaceLibraryProvider('tumblr')).toThrow()
  })

  it('maps resolve outcomes and rejects unknown statuses', () => {
    const resolved = toWorkspaceLibraryResolveResult({
      provider: 'unsplash',
      source_id: 'u1',
      status: 'resolved',
      asset: assetFixture(),
    })
    expect(resolved.asset?.id).toBe(ASSET_ID)
    expect(
      toWorkspaceLibraryResolveResult({
        provider: 'unsplash',
        source_id: 'x',
        status: 'unresolved',
      }).asset,
    ).toBeUndefined()
    expect(() =>
      toWorkspaceLibraryResolveResult({ provider: 'unsplash', status: 'bogus' }),
    ).toThrow()
  })
})

describe('unified library source mapping', () => {
  it('reads external rows with provider identity and real URLs', () => {
    const asset = toWorkspaceLibraryAsset(assetFixture())
    expect(asset.source).toBe('external')
    if (asset.source !== 'external') throw new Error('unreachable')
    expect(asset.provider).toBe('unsplash')
    expect(asset.sourceId).toBe('u1')
    expect(libraryAssetKeyFor(asset)).toBe('unsplash:u1')
  })

  it('reads media rows with the media identity, paired dimensions, and no URL keys', () => {
    const known = toWorkspaceLibraryAsset(mediaAssetFixture())
    expect(known.source).toBe('media')
    if (known.source !== 'media') throw new Error('unreachable')
    expect(known.mediaAssetId).toBe('asset_media0001')
    expect([known.width, known.height]).toEqual([1600, 900])
    expect(known).not.toHaveProperty('provider')
    expect(known).not.toHaveProperty('source_id')
    expect(libraryAssetKeyFor(known)).toBe('media:asset_media0001')
    const unknown = toWorkspaceLibraryAsset(mediaAssetFixture({ width: null, height: null }))
    if (unknown.source !== 'media') throw new Error('unreachable')
    expect([unknown.width, unknown.height]).toEqual([null, null])
  })

  it('rejects unpaired dimensions and media rows carrying URL keys', () => {
    expect(() => toWorkspaceLibraryAsset(mediaAssetFixture({ width: 100, height: null }))).toThrow()
    expect(() =>
      toWorkspaceLibraryAsset(mediaAssetFixture({ provider: 'unsplash', source_id: 'x' })),
    ).toThrow()
    expect(() => toWorkspaceLibraryAsset(mediaAssetFixture({ media_asset_id: 'nope' }))).toThrow()
    expect(() => toWorkspaceLibraryAsset(assetFixture({ source: 'bogus' }))).toThrow()
  })

  it('rejects zero media dimensions and negative external dimensions (no lenient read)', () => {
    expect(() => toWorkspaceLibraryAsset(mediaAssetFixture({ width: 0, height: 0 }))).toThrow()
    expect(() => toWorkspaceLibraryAsset(mediaAssetFixture({ width: 0, height: 100 }))).toThrow()
    expect(() => toWorkspaceLibraryAsset(assetFixture({ width: -1, height: 80 }))).toThrow()
    expect(() => toWorkspaceLibraryAsset(assetFixture({ width: 100, height: -1 }))).toThrow()
  })

  it('rejects assets missing the source discriminator (no fallback reader)', () => {
    const withoutSource = assetFixture()
    delete withoutSource.source
    expect(() => toWorkspaceLibraryAsset(withoutSource)).toThrow()
  })

  it('pins the Phase A read fixtures without drift', async () => {
    const fixtures = (
      await import('../../../../../../contracts/library/library-source.fixtures.json')
    ).default as {
      external: Record<string, unknown>
      media_known: Record<string, unknown>
      media_unknown: Record<string, unknown>
      mixed_list: { data: Record<string, unknown>[] }
      task_lineage: Record<string, unknown>
    }
    const external = toWorkspaceLibraryAsset(fixtures.external)
    expect(external.source).toBe('external')
    const known = toWorkspaceLibraryAsset(fixtures.media_known)
    expect(known.source).toBe('media')
    const unknown = toWorkspaceLibraryAsset(fixtures.media_unknown)
    if (unknown.source !== 'media') throw new Error('unreachable')
    expect([unknown.width, unknown.height]).toEqual([null, null])
    // Task lineage rides along as provenance only: never identity, never wire.
    const lineage = toWorkspaceLibraryAsset(fixtures.task_lineage)
    if (lineage.source !== 'media') throw new Error('unreachable')
    expect(libraryAssetKeyFor(lineage)).toBe(
      `media:${fixtures.task_lineage.media_asset_id as string}`,
    )
    const mixed = fixtures.mixed_list.data.map(toWorkspaceLibraryAsset)
    expect(mixed.map((item) => item.source)).toEqual(['external', 'media', 'media'])
  })
})

describe('unified library source mixed-list transport', () => {
  it('reads a mixed external + managed (known + unknown) page without failing the whole list', async () => {
    const fixtures = (
      await import('../../../../../../contracts/library/library-source.fixtures.json')
    ).default as {
      mixed_list: {
        data: Record<string, unknown>[]
        meta: { next_cursor: string; has_more: boolean }
      }
    }
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/library/assets', () => {
        return HttpResponse.json({
          data: fixtures.mixed_list.data,
          meta: fixtures.mixed_list.meta,
        })
      }),
    )
    const page = await workspaceLibraryApi.listAssets(WORKSPACE_ID)
    expect(page.items).toHaveLength(3)
    expect(page.items.map((item) => item.source)).toEqual(['external', 'media', 'media'])
    const [external, known, unknown] = page.items
    if (external?.source !== 'external') throw new Error('unreachable')
    expect(external.provider).toBe('unsplash')
    expect([external.width, external.height]).toEqual([2400, 1600])
    expect(external).not.toHaveProperty('media_asset_id')
    if (known?.source !== 'media' || unknown?.source !== 'media') {
      throw new Error('unreachable')
    }
    expect(known.mediaAssetId).toBe(fixtures.mixed_list.data[1]?.media_asset_id)
    expect([known.width, known.height]).toEqual([1600, 900])
    expect([unknown.width, unknown.height]).toEqual([null, null])
    for (const item of [known, unknown]) {
      expect(item).not.toHaveProperty('provider')
      expect(item).not.toHaveProperty('source_id')
    }
    // Cards render the inline display renditions, never external URLs.
    for (const item of page.items) expect(item.display).toBeDefined()
    expect(page.hasMore).toBe(false)
  })
})

describe('workspace library pagination and replay headers', () => {
  it('reads cursor meta and forwards replay headers', async () => {
    let seenIdempotencyKey: string | null = null
    let seenRequestId: string | null = null
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/library/folders', ({ request }) => {
        seenIdempotencyKey = request.headers.get('Idempotency-Key')
        seenRequestId = request.headers.get('Request-Id')
        return HttpResponse.json({
          data: [folderFixture()],
          meta: { next_cursor: 'cursor-2', has_more: true },
        })
      }),
    )
    const page = await workspaceLibraryApi.listFolders(WORKSPACE_ID, {
      pageSize: 1,
      idempotencyKey: 'library:folder-create:1',
      requestId: 'req-1',
    })
    expect(page.items).toHaveLength(1)
    expect(page.nextCursor).toBe('cursor-2')
    expect(page.hasMore).toBe(true)
    expect(seenIdempotencyKey).toBe('library:folder-create:1')
    expect(seenRequestId).toBe('req-1')
  })

  it('treats a missing meta as an exhausted page', async () => {
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/library/assets', () => {
        return HttpResponse.json({ data: [assetFixture({ folder_id: '' })] })
      }),
    )
    const page = await workspaceLibraryApi.listAssets(WORKSPACE_ID)
    expect(page.nextCursor).toBe('')
    expect(page.hasMore).toBe(false)
  })
})

describe('workspace library error branches', () => {
  it('propagates machine-readable codes for 404 and 409', async () => {
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/library/assets', () => {
        return HttpResponse.json(
          {
            error: {
              code: 'library_asset_not_found',
              message: '素材不存在',
              request_id: 'req-404',
            },
          },
          { status: 404 },
        )
      }),
    )
    await expect(workspaceLibraryApi.listAssets(WORKSPACE_ID)).rejects.toMatchObject({
      code: 'library_asset_not_found',
    })
  })

  it('bounds resolve batches client-side without touching the network', async () => {
    const oversized = Array.from(
      { length: WORKSPACE_LIBRARY_RESOLVE_MAX_ITEMS + 1 },
      (_, index) => ({ provider: 'unsplash', sourceId: `b${index}` }),
    )
    await expect(workspaceLibraryApi.resolveAssets(WORKSPACE_ID, oversized)).rejects.toThrow()
  })
})

describe('workspace library intent helpers', () => {
  it('maps a normalized Search reference without exposing transport field assembly to callers', () => {
    expect(
      toWorkspaceLibraryAssetInput({
        provider: 'unsplash',
        id: 'photo-1',
        source_url: 'https://unsplash.com/photos/photo-1',
        preview_url: 'https://images.unsplash.com/photo-1?w=800',
        original_url: 'https://images.unsplash.com/photo-1',
        title: 'Warm light',
        author: 'A. Creator',
        width: 1200,
        height: 800,
      }),
    ).toEqual({
      source: 'external',
      provider: 'unsplash',
      sourceId: 'photo-1',
      sourceUrl: 'https://unsplash.com/photos/photo-1',
      previewUrl: 'https://images.unsplash.com/photo-1?w=800',
      originalUrl: 'https://images.unsplash.com/photo-1',
      title: 'Warm light',
      author: 'A. Creator',
      width: 1200,
      height: 800,
    })
  })

  it('builds media inputs with the media identity and no URL keys', () => {
    expect(
      toWorkspaceLibraryMediaAssetInput('asset_media0001', { title: 'Banner' }, FOLDER_ID),
    ).toEqual({
      source: 'media',
      mediaAssetId: 'asset_media0001',
      title: 'Banner',
      folderId: FOLDER_ID,
    })
    expect(toWorkspaceLibraryMediaAssetInput('asset_media0001', {})).toMatchObject({
      source: 'media',
    })
    expect(() => toWorkspaceLibraryMediaAssetInput('nope', {})).toThrow()
  })

  it('targets one folder for saves and rejects malformed folder ids', () => {
    expect(
      toWorkspaceLibraryAssetInput(
        {
          provider: 'pexels',
          id: 'photo-2',
          source_url: 'https://pexels.com/photo-2',
          preview_url: 'https://images.pexels.com/photo-2',
          original_url: 'https://images.pexels.com/photo-2',
          title: '',
          author: '',
          width: 100,
          height: 100,
        },
        FOLDER_ID,
      ).folderId,
    ).toBe(FOLDER_ID)
    expect(() =>
      toWorkspaceLibraryAssetInput(
        {
          provider: 'pexels',
          id: 'photo-2',
          source_url: 'https://pexels.com/photo-2',
          preview_url: 'https://images.pexels.com/photo-2',
          original_url: 'https://images.pexels.com/photo-2',
          title: '',
          author: '',
          width: 100,
          height: 100,
        },
        '42',
      ),
    ).toThrow()
  })

  it('uses one saved identity key format per source for lookup and UI state', () => {
    expect(libraryAssetKeyFor({ source: 'external', provider: 'pexels', sourceId: '123' })).toBe(
      'pexels:123',
    )
    expect(libraryAssetKeyFor({ source: 'media', mediaAssetId: 'asset_media0001' })).toBe(
      'media:asset_media0001',
    )
  })
})

describe('workspace library use cases', () => {
  it('removes a reference by resolving it first, and skips unknown ones', async () => {
    const deleted: string[] = []
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/asset-resolutions', async () => {
        return HttpResponse.json({
          data: {
            items: [
              {
                provider: 'unsplash',
                source_id: 'u1',
                status: 'resolved',
                asset: assetFixture(),
              },
            ],
          },
        })
      }),
      http.delete('/api/v1/workspaces/:workspaceId/library/assets/:assetId', ({ params }) => {
        deleted.push(String(params.assetId))
        return HttpResponse.json({ data: { deleted: true } })
      }),
    )
    const removed = await removeWorkspaceSearchReference(WORKSPACE_ID, 'unsplash', 'u1')
    expect(deleted).toEqual([ASSET_ID])
    // The removed facts come from the resolved asset, not the caller input.
    expect(removed?.asset.id).toBe(ASSET_ID)
    expect(removed?.assetKey).toBe('unsplash:u1')
    expect(removed?.affectedFolderKeys).toEqual(['all', FOLDER_ID])
  })

  it('returns null when removing a reference that was never saved', async () => {
    const deleted: string[] = []
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/asset-resolutions', async () => {
        return HttpResponse.json({
          data: { items: [{ provider: 'unsplash', source_id: 'ghost', status: 'unresolved' }] },
        })
      }),
      http.delete('/api/v1/workspaces/:workspaceId/library/assets/:assetId', ({ params }) => {
        deleted.push(String(params.assetId))
        return HttpResponse.json({ data: { deleted: true } })
      }),
    )
    await expect(
      removeWorkspaceSearchReference(WORKSPACE_ID, 'unsplash', 'ghost'),
    ).resolves.toBeNull()
    expect(deleted).toEqual([])
  })

  it('looks up saved keys for the feed without fetching anything', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/asset-resolutions', async () => {
        return HttpResponse.json({
          data: {
            items: [
              {
                provider: 'unsplash',
                source_id: 'u1',
                status: 'resolved',
                asset: assetFixture(),
              },
              { provider: 'pexels', source_id: 'missing', status: 'unresolved' },
            ],
          },
        })
      }),
    )
    const keys = await lookupWorkspaceSavedAssetKeys(WORKSPACE_ID, [
      { provider: 'unsplash', sourceId: 'u1' },
      { provider: 'pexels', sourceId: 'missing' },
    ])
    expect(keys).toEqual(new Set(['unsplash:u1']))
  })

  it('resolves managed media in one poll-free attempt through the media read path', async () => {
    mediaMetadataMock.mockClear()
    const media = await materializeLibraryAssetForCanvas(WORKSPACE_ID, ASSET_ID)
    expect(mediaMetadataMock).toHaveBeenCalledTimes(1)
    expect(mediaMetadataMock.mock.calls[0]?.slice(0, 2)).toEqual([WORKSPACE_ID, ASSET_ID])
    expect(media).toMatchObject({
      mediaAssetId: 'asset_media0001',
      url: '/api/v1/workspaces/ws_abc123/media-assets/asset_media0001/content',
      width: 400,
      height: 200,
    })
  })

  it('rejects when the managed asset is not readable yet so the Canvas retry loop owns the retry', async () => {
    mediaMetadataMock.mockClear()
    mediaMetadataMock.mockRejectedValueOnce(
      Object.assign(new Error('media_not_found'), { code: 'media_not_found' }),
    )
    await expect(materializeLibraryAssetForCanvas(WORKSPACE_ID, ASSET_ID)).rejects.toMatchObject({
      code: 'media_not_found',
    })
    expect(mediaMetadataMock).toHaveBeenCalledTimes(1)
  })

  it('fails closed on malformed asset ids before any media read', async () => {
    mediaMetadataMock.mockClear()
    await expect(materializeLibraryAssetForCanvas(WORKSPACE_ID, 'nope')).rejects.toThrow()
    expect(mediaMetadataMock).not.toHaveBeenCalled()
  })
})

describe('workspace library canonical ids (lfolder_/lasset_)', () => {
  const FOLDER_ID = 'lfolder_folder0001'
  const ASSET_ID = 'lasset_asset0001'

  it('accepts canonical ids in routes, asserts, and mapping', () => {
    expect(getRenameWorkspaceLibraryFolderUrl(WORKSPACE_ID, FOLDER_ID)).toBe(
      '/workspaces/ws_abc123/library/folders/lfolder_folder0001',
    )
    expect(getMoveWorkspaceLibraryAssetUrl(WORKSPACE_ID, ASSET_ID)).toBe(
      '/workspaces/ws_abc123/library/assets/lasset_asset0001',
    )
    expect(() => assertLibraryFolderId(FOLDER_ID)).not.toThrow()
    expect(() => assertLibraryAssetId(ASSET_ID)).not.toThrow()
    expect(toWorkspaceLibraryFolder(folderFixture({ id: FOLDER_ID })).id).toBe(FOLDER_ID)
    expect(toWorkspaceLibraryAsset(assetFixture({ id: ASSET_ID, folder_id: FOLDER_ID })).id).toBe(
      ASSET_ID,
    )
    // Cross-kind and legacy-integer shapes still fail closed before any network call.
    expect(() => assertLibraryFolderId(ASSET_ID)).toThrow()
    expect(() => assertLibraryAssetId(FOLDER_ID)).toThrow()
    expect(() => assertLibraryAssetId('42')).toThrow()
  })
})
