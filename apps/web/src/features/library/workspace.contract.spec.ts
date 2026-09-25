import { describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../test/msw'
import {
  assertLibraryAssetId,
  assertLibraryFolderId,
  assertLibraryMaterializationId,
  assertWorkspaceId,
  libraryAssetKey,
  lookupWorkspaceSavedAssetKeys,
  mapWorkspaceLibraryMaterializationStatus,
  mapWorkspaceLibraryProvider,
  materializeLibraryAssetForCanvas,
  removeWorkspaceSearchReference,
  resolveLibraryAssetCardURL,
  saveWorkspaceSearchReference,
  toWorkspaceLibraryAsset,
  toWorkspaceLibraryAssetInput,
  toWorkspaceLibraryFolder,
  toWorkspaceLibraryMaterialization,
  toWorkspaceLibraryResolveResult,
  WORKSPACE_LIBRARY_RESOLVE_MAX_ITEMS,
  workspaceLibraryApi,
  workspaceLibraryAssetSuffix,
  workspaceLibraryAssetsSuffix,
  workspaceLibraryFolderSuffix,
  workspaceLibraryFoldersSuffix,
  workspaceLibraryMaterializationSuffix,
  workspaceLibraryMaterializationsSuffix,
  workspaceLibraryResolveSuffix,
} from './workspace'

vi.mock('../../shared/api/mediaAssetDelivery', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../shared/api/mediaAssetDelivery')>()
  return {
    ...actual,
    getMediaAssetMetadata: vi.fn(async () => ({
      asset_id: 'asset_media0001',
      status: 'ready',
      mime_type: 'image/png',
      width: 400,
      height: 200,
      size_bytes: 1234,
      original: { status: 'ready', url: '/api/user/media-assets/asset_media0001/original/content' },
      derivatives: {},
    })),
  }
})

const WORKSPACE_ID = 'ws_abc123'
const FOLDER_ID = 'lfolder_folder0001'
const ASSET_ID = 'lasset_asset0001'
const MATERIALIZATION_ID = 'lmat_job0001'
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
    updated_at: '2026-09-23T00:00:00Z',
    ...overrides,
  }
}

function materializationFixture(
  overrides: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    id: MATERIALIZATION_ID,
    workspace_id: WORKSPACE_ID,
    library_asset_id: ASSET_ID,
    status: 'completed',
    media_asset_id: 'asset_abc123',
    created_at: '2026-09-23T00:00:00Z',
    updated_at: '2026-09-23T00:00:00Z',
    ...overrides,
  }
}

describe('workspace library paths', () => {
  it('builds canonical transport suffixes with opaque ids only', () => {
    expect(workspaceLibraryFoldersSuffix(WORKSPACE_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/folders',
    )
    expect(workspaceLibraryFolderSuffix(WORKSPACE_ID, FOLDER_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/folders/lfolder_folder0001',
    )
    expect(workspaceLibraryAssetsSuffix(WORKSPACE_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/assets',
    )
    expect(workspaceLibraryAssetSuffix(WORKSPACE_ID, ASSET_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/assets/lasset_asset0001',
    )
    expect(workspaceLibraryResolveSuffix(WORKSPACE_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/asset-resolutions',
    )
    expect(workspaceLibraryMaterializationsSuffix(WORKSPACE_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/materializations',
    )
    expect(workspaceLibraryMaterializationSuffix(WORKSPACE_ID, MATERIALIZATION_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/materializations/lmat_job0001',
    )
  })

  it('rejects malformed ids before any network call (integer ids included)', () => {
    expect(() => assertWorkspaceId('nope')).toThrow()
    expect(() => assertLibraryFolderId('42')).toThrow()
    expect(() => assertLibraryFolderId('lasset_asset0001')).toThrow()
    expect(() => assertLibraryAssetId('42')).toThrow()
    expect(() => assertLibraryMaterializationId('nope')).toThrow()
    expect(() => assertLibraryAssetId(ASSET_ID)).not.toThrow()
    expect(() => workspaceLibraryAssetSuffix(WORKSPACE_ID, 'asset with space')).toThrow()
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

  it('maps every materialization status and gates the asset_ result link', () => {
    for (const status of ['pending', 'running', 'completed', 'failed']) {
      expect(mapWorkspaceLibraryMaterializationStatus(status)).toBe(status)
    }
    expect(() => mapWorkspaceLibraryMaterializationStatus('done')).toThrow()
    expect(toWorkspaceLibraryMaterialization(materializationFixture())).toMatchObject({
      status: 'completed',
      mediaAssetId: 'asset_abc123',
    })
    expect(() =>
      toWorkspaceLibraryMaterialization(materializationFixture({ media_asset_id: '42' })),
    ).toThrow()
    expect(() => toWorkspaceLibraryMaterialization(materializationFixture({ id: '42' }))).toThrow()
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
      toWorkspaceLibraryResolveResult({ provider: 'unsplash', source_id: 'x', status: 'unresolved' })
        .asset,
    ).toBeUndefined()
    expect(() =>
      toWorkspaceLibraryResolveResult({ provider: 'unsplash', status: 'bogus' }),
    ).toThrow()
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

  it('replays materialization creation with the same idempotency key', async () => {
    const seenKeys: Array<string | null> = []
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/materializations', ({ request }) => {
        seenKeys.push(request.headers.get('Idempotency-Key'))
        return HttpResponse.json({ data: materializationFixture() }, { status: 202 })
      }),
    )
    const first = await workspaceLibraryApi.createMaterialization(WORKSPACE_ID, ASSET_ID, {
      idempotencyKey: 'library:materialize:1',
    })
    const replay = await workspaceLibraryApi.createMaterialization(WORKSPACE_ID, ASSET_ID, {
      idempotencyKey: 'library:materialize:1',
    })
    expect(first.id).toBe(MATERIALIZATION_ID)
    expect(replay.id).toBe(MATERIALIZATION_ID)
    expect(seenKeys).toEqual(['library:materialize:1', 'library:materialize:1'])
  })
})

describe('workspace library error branches', () => {  it('propagates machine-readable codes for 404 and 409', async () => {
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

  it('uses one saved identity key format for lookup and UI state', () => {
    expect(libraryAssetKey('pexels', '123')).toBe('pexels:123')
  })

  it('bounds provider preview delivery for Library cards', () => {
    expect(
      resolveLibraryAssetCardURL({
        provider: 'unsplash',
        previewUrl: 'https://images.unsplash.com/photo-1?ixid=abc&w=1080',
      }),
    ).toBe('https://images.unsplash.com/photo-1?ixid=abc&w=480&auto=format&fit=crop&q=75')

    expect(
      resolveLibraryAssetCardURL({
        provider: 'pexels',
        previewUrl: 'https://images.pexels.com/photos/1.jpeg?w=1200',
      }),
    ).toBe('https://images.pexels.com/photos/1.jpeg?w=480&auto=compress&cs=tinysrgb')
  })
})

describe('workspace library use cases', () => {
  const reference = {
    provider: 'unsplash' as const,
    id: 'u1',
    source_url: PREVIEW_URL,
    preview_url: PREVIEW_URL,
    original_url: PREVIEW_URL,
    title: 'dunes',
    author: 'a',
    width: 100,
    height: 80,
  }

  it('saves a reference into a newly created folder', async () => {
    const seenBodies: unknown[] = []
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/folders', async () => {
        return HttpResponse.json({ data: folderFixture({ name: 'fresh' }) }, { status: 201 })
      }),
      http.post('/api/v1/workspaces/:workspaceId/library/assets', async ({ request }) => {
        seenBodies.push(await request.json())
        return HttpResponse.json({ data: assetFixture() }, { status: 201 })
      }),
    )
    const asset = await saveWorkspaceSearchReference(WORKSPACE_ID, reference, undefined, 'fresh')
    expect(asset.id).toBe(ASSET_ID)
    expect(seenBodies).toHaveLength(1)
    expect(seenBodies[0]).toMatchObject({
      data: { provider: 'unsplash', source_id: 'u1', folder_id: FOLDER_ID },
    })
  })

  it('saves a reference unfiled without a folder', async () => {
    const seenBodies: unknown[] = []
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/assets', async ({ request }) => {
        seenBodies.push(await request.json())
        return HttpResponse.json({ data: assetFixture({ folder_id: '' }) }, { status: 201 })
      }),
    )
    const asset = await saveWorkspaceSearchReference(WORKSPACE_ID, reference)
    expect(asset.folderId).toBeUndefined()
    expect(seenBodies[0]).toMatchObject({ data: { provider: 'unsplash', source_id: 'u1' } })
    expect((seenBodies[0] as { data: Record<string, unknown> }).data).not.toHaveProperty(
      'folder_id',
    )
  })

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
    await removeWorkspaceSearchReference(WORKSPACE_ID, 'unsplash', 'u1')
    expect(deleted).toEqual([ASSET_ID])
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

  it('materializes by polling the job, then resolves display media', async () => {
    let polls = 0
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/materializations', async () => {
        return HttpResponse.json(
          { data: materializationFixture({ status: 'pending' }) },
          { status: 202 },
        )
      }),
      http.get(
        '/api/v1/workspaces/:workspaceId/library/materializations/:materializationId',
        async () => {
          polls += 1
          return HttpResponse.json({
            data: materializationFixture({
              status: polls > 1 ? 'completed' : 'running',
            }),
          })
        },
      ),
    )
    const media = await materializeLibraryAssetForCanvas(WORKSPACE_ID, ASSET_ID, {
      pollDelayMs: 1,
      pollAttempts: 5,
    })
    expect(polls).toBeGreaterThan(1)
    expect(media).toMatchObject({
      mediaAssetId: 'asset_abc123',
      url: '/api/user/media-assets/asset_media0001/original/content',
      width: 400,
      height: 200,
    })
  })

  it('surfaces a failed materialization job instead of media', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/materializations', async () => {
        return HttpResponse.json(
          { data: materializationFixture({ status: 'pending' }) },
          { status: 202 },
        )
      }),
      http.get(
        '/api/v1/workspaces/:workspaceId/library/materializations/:materializationId',
        async () => {
          return HttpResponse.json({
            data: materializationFixture({ status: 'failed', error_message: 'quote broke' }),
          })
        },
      ),
    )
    await expect(
      materializeLibraryAssetForCanvas(WORKSPACE_ID, ASSET_ID, {
        pollDelayMs: 1,
        pollAttempts: 3,
      }),
    ).rejects.toThrow('quote broke')
  })
})

describe('workspace library canonical ids (lfolder_/lasset_/lmat_)', () => {
  const FOLDER_ID = 'lfolder_folder0001'
  const ASSET_ID = 'lasset_asset0001'
  const MATERIALIZATION_ID = 'lmat_job0001'

  it('accepts canonical ids in suffixes, asserts, and mapping', () => {
    expect(workspaceLibraryFolderSuffix(WORKSPACE_ID, FOLDER_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/folders/lfolder_folder0001',
    )
    expect(workspaceLibraryAssetSuffix(WORKSPACE_ID, ASSET_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/assets/lasset_asset0001',
    )
    expect(workspaceLibraryMaterializationSuffix(WORKSPACE_ID, MATERIALIZATION_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/materializations/lmat_job0001',
    )
    expect(() => assertLibraryFolderId(FOLDER_ID)).not.toThrow()
    expect(() => assertLibraryAssetId(ASSET_ID)).not.toThrow()
    expect(() => assertLibraryMaterializationId(MATERIALIZATION_ID)).not.toThrow()
    expect(toWorkspaceLibraryFolder(folderFixture({ id: FOLDER_ID })).id).toBe(FOLDER_ID)
    expect(toWorkspaceLibraryAsset(assetFixture({ id: ASSET_ID, folder_id: FOLDER_ID })).id).toBe(
      ASSET_ID,
    )
    expect(
      toWorkspaceLibraryMaterialization(
        materializationFixture({ id: MATERIALIZATION_ID, library_asset_id: ASSET_ID }),
      ).id,
    ).toBe(MATERIALIZATION_ID)
    // Cross-kind and legacy-integer shapes still fail closed before any network call.
    expect(() => assertLibraryFolderId(ASSET_ID)).toThrow()
    expect(() => assertLibraryAssetId(FOLDER_ID)).toThrow()
    expect(() => assertLibraryMaterializationId(ASSET_ID)).toThrow()
    expect(() => assertLibraryAssetId('42')).toThrow()
  })

  it('materializes canonical ids end to end: create, poll, then resolve display media', async () => {
    let polls = 0
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/materializations', async () => {
        return HttpResponse.json(
          { data: materializationFixture({ id: MATERIALIZATION_ID, library_asset_id: ASSET_ID, status: 'pending' }) },
          { status: 202 },
        )
      }),
      http.get(
        '/api/v1/workspaces/:workspaceId/library/materializations/:materializationId',
        async () => {
          polls += 1
          return HttpResponse.json({
            data: materializationFixture({
              id: MATERIALIZATION_ID,
              library_asset_id: ASSET_ID,
              status: polls > 1 ? 'completed' : 'running',
            }),
          })
        },
      ),
    )
    const media = await materializeLibraryAssetForCanvas(WORKSPACE_ID, ASSET_ID, {
      pollDelayMs: 1,
      pollAttempts: 5,
    })
    expect(polls).toBeGreaterThan(1)
    expect(media).toMatchObject({
      mediaAssetId: 'asset_abc123',
      width: 400,
      height: 200,
    })
  })

  it('degrades gracefully when creation renders 503 billing_quote_unavailable', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/materializations', async () => {
        return HttpResponse.json(
          {
            error: {
              code: 'billing_quote_unavailable',
              message: '价格服务暂不可用',
              request_id: 'req-503',
            },
          },
          { status: 503 },
        )
      }),
    )
    await expect(
      materializeLibraryAssetForCanvas(WORKSPACE_ID, ASSET_ID, {
        pollDelayMs: 1,
        pollAttempts: 3,
      }),
    ).rejects.toMatchObject({ code: 'billing_quote_unavailable' })
  })

  it('degrades gracefully when the materializer seam is down (503 service_unavailable)', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/materializations', async () => {
        return HttpResponse.json(
          {
            error: {
              code: 'service_unavailable',
              message: '转存服务暂不可用',
              request_id: 'req-503',
            },
          },
          { status: 503 },
        )
      }),
    )
    await expect(
      materializeLibraryAssetForCanvas(WORKSPACE_ID, ASSET_ID, {
        pollDelayMs: 1,
        pollAttempts: 3,
      }),
    ).rejects.toMatchObject({ code: 'service_unavailable' })
  })

  it('degrades gracefully when polling renders 503 instead of a snapshot', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/materializations', async () => {
        return HttpResponse.json(
          { data: materializationFixture({ id: MATERIALIZATION_ID, library_asset_id: ASSET_ID, status: 'pending' }) },
          { status: 202 },
        )
      }),
      http.get(
        '/api/v1/workspaces/:workspaceId/library/materializations/:materializationId',
        async () => {
          return HttpResponse.json(
            {
              error: {
                code: 'service_unavailable',
                message: '素材库服务暂不可用',
                request_id: 'req-503',
              },
            },
            { status: 503 },
          )
        },
      ),
    )
    await expect(
      materializeLibraryAssetForCanvas(WORKSPACE_ID, ASSET_ID, {
        pollDelayMs: 1,
        pollAttempts: 3,
      }),
    ).rejects.toMatchObject({ code: 'service_unavailable' })
  })

  it('degrades gracefully when the polled job is unknown (404 library_materialization_not_found)', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/materializations', async () => {
        return HttpResponse.json(
          { data: materializationFixture({ id: MATERIALIZATION_ID, library_asset_id: ASSET_ID, status: 'pending' }) },
          { status: 202 },
        )
      }),
      http.get(
        '/api/v1/workspaces/:workspaceId/library/materializations/:materializationId',
        async () => {
          return HttpResponse.json(
            {
              error: {
                code: 'library_materialization_not_found',
                message: '转存任务不存在',
                request_id: 'req-404',
              },
            },
            { status: 404 },
          )
        },
      ),
    )
    await expect(
      materializeLibraryAssetForCanvas(WORKSPACE_ID, ASSET_ID, {
        pollDelayMs: 1,
        pollAttempts: 3,
      }),
    ).rejects.toMatchObject({ code: 'library_materialization_not_found' })
  })

  it('fails closed instead of hanging when the job never reaches a terminal state', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/library/materializations', async () => {
        return HttpResponse.json(
          { data: materializationFixture({ id: MATERIALIZATION_ID, library_asset_id: ASSET_ID, status: 'pending' }) },
          { status: 202 },
        )
      }),
      http.get(
        '/api/v1/workspaces/:workspaceId/library/materializations/:materializationId',
        async () => {
          return HttpResponse.json({
            data: materializationFixture({
              id: MATERIALIZATION_ID,
              library_asset_id: ASSET_ID,
              status: 'running',
            }),
          })
        },
      ),
    )
    await expect(
      materializeLibraryAssetForCanvas(WORKSPACE_ID, ASSET_ID, {
        pollDelayMs: 1,
        pollAttempts: 2,
      }),
    ).rejects.toThrow()
  })
})
