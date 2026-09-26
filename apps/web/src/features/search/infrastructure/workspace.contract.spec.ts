import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import {
  assertWorkspaceLibraryAssetId,
  mapWorkspaceSearchProvider,
  toWorkspaceSearchResult,
  toWorkspaceVisionQuery,
  workspaceSearchApi,
} from './workspace'
import {
  getCreateWorkspaceVisionQueryUrl,
  getSearchWorkspaceUrl,
} from '../../../shared/api/generated-query/public/workspace-search/workspace-search'

const WORKSPACE_ID = 'ws_abc123'
const ASSET_ID = 'lasset_asset0001'
const PREVIEW_URL = 'https://images.unsplash.com/photo-test'

function resultFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    provider: 'unsplash',
    source_id: 'u1',
    title: 'dunes',
    author: 'a',
    source_url: PREVIEW_URL,
    preview_url: PREVIEW_URL,
    original_url: PREVIEW_URL,
    width: 100,
    height: 80,
    ...overrides,
  }
}

describe('workspace search paths', () => {
  it('builds canonical routes matching the generated hooks (publicMutator adds /v1)', () => {
    expect(getSearchWorkspaceUrl(WORKSPACE_ID, { q: 'cafe' })).toBe(
      '/workspaces/ws_abc123/search?q=cafe',
    )
    expect(getCreateWorkspaceVisionQueryUrl(WORKSPACE_ID)).toBe(
      '/workspaces/ws_abc123/search/vision-queries',
    )
  })

  it('rejects malformed ids before any network call', async () => {
    expect(() => assertWorkspaceLibraryAssetId('42')).toThrow()
    expect(() => assertWorkspaceLibraryAssetId('lfolder_x')).toThrow()
    expect(() => assertWorkspaceLibraryAssetId('lfolder_x')).toThrow()
    expect(() => assertWorkspaceLibraryAssetId(ASSET_ID)).not.toThrow()
    expect(() => assertWorkspaceLibraryAssetId('lasset_asset0001')).not.toThrow()
    await expect(workspaceSearchApi.search({ workspaceId: 'nope', query: 'cafe' })).rejects.toThrow()
  })
})

describe('workspace search mapping', () => {
  it('maps results snake_case to camelCase and gates providers', () => {
    expect(toWorkspaceSearchResult(resultFixture())).toMatchObject({
      provider: 'unsplash',
      sourceId: 'u1',
      previewUrl: PREVIEW_URL,
    })
    expect(() => mapWorkspaceSearchProvider('tumblr')).toThrow()
    expect(() => toWorkspaceSearchResult(resultFixture({ provider: 'tumblr' }))).toThrow()
  })

  it('maps vision queries and gates the visq_ operation id', () => {
    expect(
      toWorkspaceVisionQuery({
        operation_id: 'visq_op0001',
        status: 'succeeded',
        query: 'warm cafe',
        created_at: '2026-09-23T00:00:00Z',
      }),
    ).toMatchObject({ operationId: 'visq_op0001', query: 'warm cafe' })
    expect(
      toWorkspaceVisionQuery({
        operation_id: 'visq_op0001',
        status: 'succeeded',
        query: 'warm cafe',
        replayed: true,
        created_at: '2026-09-23T00:00:00Z',
      }).replayed,
    ).toBe(true)
    expect(() =>
      toWorkspaceVisionQuery({
        operation_id: 'op-1',
        status: 'succeeded',
        query: 'x',
        created_at: '2026-09-23T00:00:00Z',
      }),
    ).toThrow()
  })
})

describe('workspace search pagination and filters', () => {
  it('normalizes the query and forwards provider filters plus cursor meta', async () => {
    let seenUrl: string | null = null
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/search', ({ request }) => {
        seenUrl = request.url
        return HttpResponse.json({
          data: [resultFixture()],
          meta: { next_cursor: 'cursor-2', has_more: true },
        })
      }),
    )
    const page = await workspaceSearchApi.search({
      workspaceId: WORKSPACE_ID,
      query: '  warm\t cafe\n',
      providers: ['pexels'],
      pageSize: 10,
    })
    expect(page.items).toHaveLength(1)
    expect(page.nextCursor).toBe('cursor-2')
    expect(page.hasMore).toBe(true)
    expect(seenUrl).toContain('q=warm+cafe')
    expect(seenUrl).toContain('provider=pexels')
    expect(seenUrl).toContain('page_size=10')
  })

  it('propagates the 422 validation code for blank queries', async () => {
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/search', () => {
        return HttpResponse.json(
          {
            error: {
              code: 'validation_failed',
              message: '搜索关键词非法',
              details: { fields: { q: 'invalid' } },
              request_id: 'req-422',
            },
          },
          { status: 422 },
        )
      }),
    )
    await expect(
      workspaceSearchApi.search({ workspaceId: WORKSPACE_ID, query: '   ' }),
    ).rejects.toMatchObject({ code: 'validation_failed' })
  })
})

describe('workspace vision query replay headers', () => {
  it('sends the idempotency key and maps the settled query', async () => {
    let seenKey: string | null = null
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/search/vision-queries', ({ request }) => {
        seenKey = request.headers.get('Idempotency-Key')
        return HttpResponse.json(
          {
            data: {
              operation_id: 'visq_op0001',
              status: 'succeeded',
              query: 'warm cafe',
              created_at: '2026-09-23T00:00:00Z',
            },
          },
          { status: 201 },
        )
      }),
    )
    const query = await workspaceSearchApi.createVisionQuery(
      { workspaceId: WORKSPACE_ID, userText: 'warm cafe', libraryAssetIds: [ASSET_ID] },
      { idempotencyKey: 'search:vision:1' },
    )
    expect(query.operationId).toBe('visq_op0001')
    expect(seenKey).toBe('search:vision:1')
  })

  it('propagates billing and provider failure codes', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/search/vision-queries', () => {
        return HttpResponse.json(
          {
            error: {
              code: 'billing_wallet_restricted',
              message: '受限',
              request_id: 'req-403',
            },
          },
          { status: 403 },
        )
      }),
    )
    await expect(
      workspaceSearchApi.createVisionQuery(
        { workspaceId: WORKSPACE_ID, userText: 'cafe' },
        { idempotencyKey: 'search:vision:2' },
      ),
    ).rejects.toMatchObject({ code: 'billing_wallet_restricted' })
  })

  it('sends canonical lasset_ refs in the vision query body', async () => {
    let seenBody: unknown = null
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/search/vision-queries', async ({ request }) => {
        seenBody = await request.json()
        return HttpResponse.json(
          {
            data: {
              operation_id: 'visq_op0002',
              status: 'succeeded',
              query: 'warm cafe',
              created_at: '2026-09-23T00:00:00Z',
            },
          },
          { status: 201 },
        )
      }),
    )
    const query = await workspaceSearchApi.createVisionQuery(
      { workspaceId: WORKSPACE_ID, userText: 'warm cafe', libraryAssetIds: ['lasset_asset0001'] },
      { idempotencyKey: 'search:vision:3' },
    )
    expect(query.operationId).toBe('visq_op0002')
    expect(seenBody).toMatchObject({ data: { library_asset_ids: ['lasset_asset0001'] } })
  })

  it('sends staged upload refs and allowlisted image urls in the vision query body', async () => {
    let seenBody: unknown = null
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/search/vision-queries', async ({ request }) => {
        seenBody = await request.json()
        return HttpResponse.json(
          {
            data: {
              operation_id: 'visq_op0003',
              status: 'succeeded',
              query: 'warm cafe',
              created_at: '2026-09-23T00:00:00Z',
            },
          },
          { status: 201 },
        )
      }),
    )
    const query = await workspaceSearchApi.createVisionQuery(
      {
        workspaceId: WORKSPACE_ID,
        uploadIds: ['upload_up0001'],
        imageUrls: ['https://images.unsplash.com/photo-test'],
      },
      { idempotencyKey: 'search:vision:4' },
    )
    expect(query.operationId).toBe('visq_op0003')
    expect(seenBody).toMatchObject({
      data: {
        upload_ids: ['upload_up0001'],
        image_urls: ['https://images.unsplash.com/photo-test'],
      },
    })
  })

  it('fails closed on multi-provider filters (generated URL gap, see module TODO)', async () => {
    await expect(
      workspaceSearchApi.search({
        workspaceId: WORKSPACE_ID,
        query: 'cafe',
        providers: ['unsplash', 'pexels'],
      }),
    ).rejects.toThrow()
  })
})
