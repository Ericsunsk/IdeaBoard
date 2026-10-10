// Issue #803: workspace upload sessions + media assets contract spec.
//
// Covers the session state machine mapping, completion idempotency
// headers, derivative/content URL shape (no storage keys), and the
// 404-vs-403 branches. MSW only by design (contract specs must not mock
// generated-query modules); legacy `/api/user` upload behavior is owned
// by `upload.contract.spec.ts` and untouched here.
import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../test/msw'
import { createApiError } from './client'
import {
  assertUploadId,
  assertWorkspaceAssetId,
  assertWorkspaceId,
  mapWorkspaceUploadSessionStatus,
  toWorkspaceMediaAsset,
  toWorkspaceUploadSession,
  workspaceMediaApi,
  workspaceMediaAssetContentSuffix,
  workspaceMediaAssetDerivativeSuffix,
  workspaceUploadApi,
} from './upload'
import {
  getArchiveWorkspaceMediaAssetUrl,
  getCompleteWorkspaceUploadUrl,
  getCreateWorkspaceUploadUrl,
  getGetWorkspaceUploadUrl,
  getStageWorkspaceUploadBytesUrl,
} from './generated-query/public/workspace-media/workspace-media'
import {
  resolveWorkspaceMediaAssetDerivativeURL,
  resolveWorkspaceMediaAssetOriginalURL,
} from './mediaAssetDelivery'
import { workspaceMediaIdempotencyKey, workspaceUploadIdempotencyKey } from './idempotencyKey'

const WORKSPACE_ID = 'ws_abc123'
const UPLOAD_ID = 'upload_def456'
const ASSET_ID = 'asset_789abc'
const CHECKSUM = 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'

function sessionFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: UPLOAD_ID,
    workspace_id: WORKSPACE_ID,
    status: 'pending',
    filename: 'photo.png',
    mime_type: 'image/png',
    size_bytes: 1024,
    checksum_sha256: CHECKSUM,
    media_kind: 'image',
    upload_target: {
      kind: 'same-origin-slot',
      slot: UPLOAD_ID,
      expires_at: '2026-09-23T01:00:00Z',
    },
    created_at: '2026-09-23T00:00:00Z',
    updated_at: '2026-09-23T00:00:00Z',
    expires_at: '2026-09-23T00:30:00Z',
    ...overrides,
  }
}

function assetFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: ASSET_ID,
    workspace_id: WORKSPACE_ID,
    status: 'pending',
    media_kind: 'image',
    mime_type: 'image/png',
    width: 4,
    height: 3,
    size_bytes: 1024,
    checksum_sha256: CHECKSUM,
    content_url: `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/content`,
    created_at: '2026-09-23T00:00:00Z',
    updated_at: '2026-09-23T00:00:00Z',
    ...overrides,
  }
}

describe('workspace upload paths', () => {
  it('builds canonical routes matching the generated hooks (publicMutator adds /v1)', () => {
    expect(getCreateWorkspaceUploadUrl(WORKSPACE_ID)).toBe('/workspaces/ws_abc123/uploads')
    expect(getGetWorkspaceUploadUrl(WORKSPACE_ID, UPLOAD_ID)).toBe(
      '/workspaces/ws_abc123/uploads/upload_def456',
    )
    expect(getCompleteWorkspaceUploadUrl(WORKSPACE_ID, UPLOAD_ID)).toBe(
      '/workspaces/ws_abc123/uploads/upload_def456/complete',
    )
    expect(getStageWorkspaceUploadBytesUrl(WORKSPACE_ID, UPLOAD_ID)).toBe(
      '/workspaces/ws_abc123/uploads/upload_def456/bytes',
    )
    expect(getArchiveWorkspaceMediaAssetUrl(WORKSPACE_ID, ASSET_ID)).toBe(
      '/workspaces/ws_abc123/media-assets/asset_789abc/archive',
    )
    expect(workspaceMediaAssetContentSuffix(WORKSPACE_ID, ASSET_ID)).toBe(
      '/v1/workspaces/ws_abc123/media-assets/asset_789abc/content',
    )
    expect(workspaceMediaAssetDerivativeSuffix(WORKSPACE_ID, ASSET_ID, 'preview')).toBe(
      '/v1/workspaces/ws_abc123/media-assets/asset_789abc/derivatives/preview',
    )
  })

  it('rejects malformed opaque ids before any network call', async () => {
    expect(() => assertWorkspaceId('nope')).toThrow()
    expect(() => assertWorkspaceId('upload_def456')).toThrow()
    expect(() => assertUploadId(UPLOAD_ID)).not.toThrow()
    expect(() => assertWorkspaceAssetId('42')).toThrow()
    await expect(
      workspaceUploadApi.createSession('  ', {
        filename: 'x.png',
        mimeType: 'image/png',
        sizeBytes: 1,
        checksumSha256: CHECKSUM,
      }),
    ).rejects.toThrow()
    expect(() => workspaceMediaAssetContentSuffix(WORKSPACE_ID, 'asset with space')).toThrow()
  })
})

describe('workspace upload session state machine', () => {
  it('maps every session state and rejects unknown states', () => {
    for (const status of ['pending', 'transferring', 'completed', 'failed', 'expired']) {
      expect(mapWorkspaceUploadSessionStatus(status)).toBe(status)
    }
    expect(() => mapWorkspaceUploadSessionStatus('ready')).toThrow()
    expect(() => mapWorkspaceUploadSessionStatus(null)).toThrow()
  })

  it('parses session payloads and guards the opaque slot', () => {
    const session = toWorkspaceUploadSession(sessionFixture({ status: 'transferring' }))
    expect(session).toMatchObject({
      id: UPLOAD_ID,
      workspaceId: WORKSPACE_ID,
      status: 'transferring',
      uploadTarget: { kind: 'same-origin-slot', slot: UPLOAD_ID },
    })
    expect(session.assetId).toBeUndefined()
    expect(toWorkspaceUploadSession(sessionFixture({ asset_id: ASSET_ID })).assetId).toBe(ASSET_ID)
    expect(() =>
      toWorkspaceUploadSession(
        sessionFixture({ upload_target: { kind: 'presigned', slot: 'https://bucket/secret' } }),
      ),
    ).toThrow()
  })

  it('parses asset payloads and rejects storage-key content URLs', () => {
    const asset = toWorkspaceMediaAsset(assetFixture())
    expect(asset).toMatchObject({ id: ASSET_ID, workspaceId: WORKSPACE_ID, status: 'pending' })
    expect(() =>
      toWorkspaceMediaAsset(assetFixture({ content_url: 'https://cdn/bucket/storage_key=abc' })),
    ).toThrow()
    expect(() => toWorkspaceMediaAsset(assetFixture({ id: '42' }))).toThrow()
  })
})

describe('workspace media delivery URLs', () => {
  it('resolves canonical v1 URLs carrying asset ids only', () => {
    expect(resolveWorkspaceMediaAssetOriginalURL(WORKSPACE_ID, ASSET_ID)).toBe(
      `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/content`,
    )
    expect(resolveWorkspaceMediaAssetDerivativeURL(WORKSPACE_ID, ASSET_ID, 'preview')).toBe(
      `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/derivatives/preview`,
    )
    for (const url of [
      resolveWorkspaceMediaAssetOriginalURL(WORKSPACE_ID, ASSET_ID),
      resolveWorkspaceMediaAssetDerivativeURL(WORKSPACE_ID, ASSET_ID),
    ]) {
      expect(url).toContain(ASSET_ID)
      expect(url).not.toMatch(/storage[_-]?key|bucket|secret/i)
    }
    expect(() => resolveWorkspaceMediaAssetOriginalURL('nope', ASSET_ID)).toThrow()
  })
})

describe('workspace upload idempotency keys', () => {
  it('scopes keys per operation with the backend upload:/media: prefixes', () => {
    expect(workspaceUploadIdempotencyKey('create')).toMatch(/^upload:create:.+/)
    expect(workspaceUploadIdempotencyKey('bytes')).toMatch(/^upload:bytes:.+/)
    expect(workspaceUploadIdempotencyKey('complete')).toMatch(/^upload:complete:.+/)
    expect(workspaceMediaIdempotencyKey('archive')).toMatch(/^media:archive:.+/)
    expect(workspaceUploadIdempotencyKey('create')).not.toBe(
      workspaceUploadIdempotencyKey('create'),
    )
  })
})

describe('workspace upload client', () => {
  it('creates sessions with header-only idempotency keys', async () => {
    const seen: Array<{ idempotencyKey: string | null; requestId: string | null; path: string }> =
      []
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/uploads', ({ request, params }) => {
        seen.push({
          idempotencyKey: request.headers.get('Idempotency-Key'),
          requestId: request.headers.get('Request-Id'),
          path: new URL(request.url).pathname,
        })
        expect(params.workspaceId).toBe('ws_abc123')
        return HttpResponse.json({ data: sessionFixture() })
      }),
    )
    const session = await workspaceUploadApi.createSession(
      WORKSPACE_ID,
      { filename: 'photo.png', mimeType: 'image/png', sizeBytes: 1024, checksumSha256: CHECKSUM },
      { idempotencyKey: 'upload:create:1', requestId: 'req-1' },
    )
    expect(session.id).toBe(UPLOAD_ID)
    expect(session.uploadTarget.slot).toBe(UPLOAD_ID)
    expect(seen).toEqual([
      {
        idempotencyKey: 'upload:create:1',
        requestId: 'req-1',
        path: '/api/v1/workspaces/ws_abc123/uploads',
      },
    ])
  })

  it('stages raw bytes with the declared MIME and header-only idempotency keys', async () => {
    const seen: Array<{ idempotencyKey: string | null; contentType: string | null }> = []
    useMswHandlers(
      http.put(
        '/api/v1/workspaces/:workspaceId/uploads/:uploadId/bytes',
        async ({ request, params }) => {
          expect(params.workspaceId).toBe('ws_abc123')
          expect(params.uploadId).toBe(UPLOAD_ID)
          seen.push({
            idempotencyKey: request.headers.get('Idempotency-Key'),
            contentType: request.headers.get('Content-Type'),
          })
          const bytes = new Uint8Array(await request.arrayBuffer())
          expect(bytes.length).toBe(4)
          return HttpResponse.json({ data: sessionFixture({ status: 'transferring' }) })
        },
      ),
    )
    const session = await workspaceUploadApi.stageBytes(
      WORKSPACE_ID,
      UPLOAD_ID,
      new Blob([new Uint8Array([137, 80, 78, 71])], { type: 'image/png' }),
      { contentType: 'image/png', idempotencyKey: 'upload:bytes:1' },
    )
    expect(session.id).toBe(UPLOAD_ID)
    expect(session.status).toBe('transferring')
    expect(seen).toEqual([{ idempotencyKey: 'upload:bytes:1', contentType: 'image/png' }])
  })

  it('completes sessions and returns the same asset id on replay', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/uploads/:uploadId/complete', () => {
        return HttpResponse.json({
          data: {
            upload: sessionFixture({ status: 'completed', asset_id: ASSET_ID }),
            asset: assetFixture(),
          },
        })
      }),
    )
    const input = { checksumSha256: CHECKSUM, sizeBytes: 1024 }
    const first = await workspaceUploadApi.completeSession(WORKSPACE_ID, UPLOAD_ID, input, {
      idempotencyKey: 'upload:complete:1',
    })
    const replay = await workspaceUploadApi.completeSession(WORKSPACE_ID, UPLOAD_ID, input, {
      idempotencyKey: 'upload:complete:1',
    })
    expect(first.asset.id).toBe(ASSET_ID)
    expect(replay.asset.id).toBe(first.asset.id)
    expect(replay.upload.status).toBe('completed')
  })

  it('keeps hidden (404) and forbidden (403) branches distinct', async () => {
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/uploads/:uploadId', ({ params }) => {
        if (params.workspaceId === 'ws_hidden') {
          return HttpResponse.json(
            { error: { code: 'workspace_not_found', message: 'gone', request_id: 'r1' } },
            { status: 404 },
          )
        }
        return HttpResponse.json(
          { error: { code: 'upload_not_found', message: 'gone', request_id: 'r2' } },
          { status: 404 },
        )
      }),
      http.post('/api/v1/workspaces/:workspaceId/media-assets/:assetId/archive', () => {
        return HttpResponse.json(
          { error: { code: 'workspace_forbidden', message: 'denied', request_id: 'r3' } },
          { status: 403 },
        )
      }),
    )
    await expect(workspaceUploadApi.getSession('ws_hidden', UPLOAD_ID)).rejects.toMatchObject({
      code: 'workspace_not_found',
      status: 404,
    })
    await expect(workspaceUploadApi.getSession(WORKSPACE_ID, UPLOAD_ID)).rejects.toMatchObject({
      code: 'upload_not_found',
      status: 404,
    })
    await expect(
      workspaceMediaApi.archiveAsset(WORKSPACE_ID, ASSET_ID, { idempotencyKey: 'media:archive:1' }),
    ).rejects.toMatchObject({ code: 'workspace_forbidden', status: 403 })
  })
})

describe('contract error mapping', () => {
  it('maps v1 error envelopes to branchable codes without touching legacy codes', () => {
    const contract = createApiError(
      {
        error: {
          code: 'upload_idempotency_conflict',
          message: 'conflict',
          details: { fields: {} },
          request_id: 'req-7',
        },
      },
      409,
    )
    expect(contract.code).toBe('upload_idempotency_conflict')
    expect(contract.status).toBe(409)
    expect(contract.requestId).toBe('req-7')
    expect(contract.hasSafeDisplayMessage).toBe(true)

    const legacy = createApiError({ code: 413, msg: 'too large', message_safe: true }, 413)
    expect(legacy.code).toBe(413)
    expect(legacy.requestId).toBeUndefined()
  })
})
