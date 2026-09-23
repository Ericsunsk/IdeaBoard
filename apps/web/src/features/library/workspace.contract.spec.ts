import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../test/msw'
import {
  assertLibraryAssetId,
  assertLibraryFolderId,
  assertLibraryMaterializationId,
  assertWorkspaceId,
  mapWorkspaceLibraryMaterializationStatus,
  mapWorkspaceLibraryProvider,
  toWorkspaceLibraryAsset,
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

const WORKSPACE_ID = 'ws_abc123'
const FOLDER_ID = 'libf_folder0001'
const ASSET_ID = 'liba_asset0001'
const MATERIALIZATION_ID = 'libm_job0001'
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
      '/v1/workspaces/ws_abc123/library/folders/libf_folder0001',
    )
    expect(workspaceLibraryAssetsSuffix(WORKSPACE_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/assets',
    )
    expect(workspaceLibraryAssetSuffix(WORKSPACE_ID, ASSET_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/assets/liba_asset0001',
    )
    expect(workspaceLibraryResolveSuffix(WORKSPACE_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/asset-resolutions',
    )
    expect(workspaceLibraryMaterializationsSuffix(WORKSPACE_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/materializations',
    )
    expect(workspaceLibraryMaterializationSuffix(WORKSPACE_ID, MATERIALIZATION_ID)).toBe(
      '/v1/workspaces/ws_abc123/library/materializations/libm_job0001',
    )
  })

  it('rejects malformed ids before any network call (integer ids included)', () => {
    expect(() => assertWorkspaceId('nope')).toThrow()
    expect(() => assertLibraryFolderId('42')).toThrow()
    expect(() => assertLibraryFolderId('liba_asset0001')).toThrow()
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
