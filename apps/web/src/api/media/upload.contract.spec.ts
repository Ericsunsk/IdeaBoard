import { describe, expect, it, test, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../test/msw'
import {
  isBoardPublicIdShape,
  UPLOAD_OPERATION_TIMEOUT_MS,
  uploadApi,
  workspaceUploadApi,
} from './upload'

const WORKSPACE_ID = 'ws_board1'
const ASSET_ID = 'asset_upload42'
const CHECKSUM = 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'

function assetFixture(overrides: Record<string, unknown> = {}) {
  return {
    id: ASSET_ID,
    workspace_id: WORKSPACE_ID,
    status: 'ready',
    media_kind: 'image',
    mime_type: 'image/png',
    width: 640,
    height: 480,
    size_bytes: 5,
    checksum_sha256: CHECKSUM,
    content_url: `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/content`,
    created_at: '2026-09-23T00:00:00Z',
    updated_at: '2026-09-23T00:00:00Z',
    ...overrides,
  }
}

function registerSuccessfulUploadHandlers() {
  return useMswHandlers(
    http.post('/api/v1/workspaces/:workspaceId/media-assets/direct', () =>
      HttpResponse.json({ data: assetFixture() }),
    ),
  )
}

describe('uploadApi', () => {
  it('uploads a workspace media asset through the HTTP boundary', async () => {
    const file = new File(['image'], 'image.png', { type: 'image/png' })
    registerSuccessfulUploadHandlers()

    await expect(uploadApi.upload(file, ` ${WORKSPACE_ID} `)).resolves.toEqual({
      url: `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/content`,
      asset_id: ASSET_ID,
      width: 640,
      height: 480,
      mime: 'image/png',
    })
  })

  it('uploads a profile avatar through the v1 HTTP boundary (raw bytes)', async () => {
    const file = new File(['avatar'], 'avatar.jpg', { type: 'image/jpeg' })
    const receivedRequests: Array<{
      method: string
      path: string
      contentType: string | null
      idempotencyKey: string | null
      bodyText: string
    }> = []

    useMswHandlers(
      http.put('/api/v1/user/avatar', async ({ request }) => {
        receivedRequests.push({
          method: request.method,
          path: new URL(request.url).pathname,
          contentType: request.headers.get('Content-Type'),
          idempotencyKey: request.headers.get('Idempotency-Key'),
          bodyText: await request.text(),
        })

        return HttpResponse.json({ data: { avatar_url: '/api/v1/public/users/AVTR01/avatar' } })
      }),
    )

    await expect(uploadApi.uploadAvatar(file)).resolves.toEqual({
      avatar: '/api/v1/public/users/AVTR01/avatar',
      mime: 'image/jpeg',
      ext: '.jpg',
    })
    expect(receivedRequests).toHaveLength(1)
    expect(receivedRequests[0]?.method).toBe('PUT')
    expect(receivedRequests[0]?.path).toBe('/api/v1/user/avatar')
    expect(receivedRequests[0]?.contentType).toBe('image/jpeg')
    expect(receivedRequests[0]?.idempotencyKey).toMatch(/^avatar:upload:/)
    expect(receivedRequests[0]?.bodyText).toBe('avatar')
  })

  it('keeps the upload timeout long enough for slow HTTP responses', async () => {
    vi.useFakeTimers()

    try {
      const slowResponseDelayMs = 10_050
      const file = new File(['slow avatar'], 'slow-avatar.jpg', { type: 'image/jpeg' })
      let resolveUploadStarted: () => void = () => {}
      const uploadStarted = new Promise<void>((resolve) => {
        resolveUploadStarted = resolve
      })

      useMswHandlers(
        http.put('/api/v1/user/avatar', async ({ request }) => {
          await request.arrayBuffer()
          resolveUploadStarted()
          await new Promise((resolve) => setTimeout(resolve, slowResponseDelayMs))

          return HttpResponse.json({ data: { avatar_url: '/api/v1/public/users/MEMB1A/avatar' } })
        }),
      )

      const response = uploadApi.uploadAvatar(file)

      await uploadStarted
      expect(UPLOAD_OPERATION_TIMEOUT_MS).toBeGreaterThan(slowResponseDelayMs)
      await vi.advanceTimersByTimeAsync(slowResponseDelayMs)

      await expect(response).resolves.toEqual({
        avatar: '/api/v1/public/users/MEMB1A/avatar',
        mime: 'image/jpeg',
        ext: '.jpg',
      })
    } finally {
      vi.useRealTimers()
    }
  })

  it('surfaces canonical workspace upload failures from the HTTP response body', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/media-assets/direct', () => {
        return HttpResponse.json(
          {
            error: { code: 'upload_too_large', message: 'Upload too large', request_id: 'req-413' },
          },
          { status: 413 },
        )
      }),
    )

    await expect(
      uploadApi.upload(new File(['too large'], 'large.png', { type: 'image/png' }), WORKSPACE_ID),
    ).rejects.toThrow('Upload too large')
  })

  it('rejects successful upload envelopes without domain data', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/media-assets/direct', () => {
        return HttpResponse.json({ meta: { request_id: 'req-empty' } })
      }),
    )

    await expect(
      uploadApi.upload(new File(['image'], 'image.png', { type: 'image/png' }), WORKSPACE_ID),
    ).rejects.toThrow('Malformed API envelope')
  })

  it('rejects workspace uploads that were not registered as media assets', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/media-assets/direct', () =>
        HttpResponse.json({ data: null }),
      ),
    )

    await expect(
      uploadApi.upload(new File(['image'], 'image.png', { type: 'image/png' }), WORKSPACE_ID),
    ).rejects.toThrow('Upload failed. Please try again.')
  })

  it('preserves structured workspace transport errors without exposing unsafe legacy fields', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/media-assets/direct', () => {
        return HttpResponse.json(
          {
            error: {
              code: 'upstream_failure',
              message: 'Upload service unavailable',
              request_id: 'req-1',
            },
          },
          { status: 503 },
        )
      }),
    )

    await expect(
      uploadApi.upload(new File(['image'], 'image.png', { type: 'image/png' }), WORKSPACE_ID),
    ).rejects.toMatchObject({
      code: 'upstream_failure',
      message: 'Upload service unavailable',
      status: 503,
    })
  })

  it('rejects avatar responses without a usable URL', async () => {
    useMswHandlers(
      http.put('/api/v1/user/avatar', () => {
        return HttpResponse.json({ data: { avatar_url: '   ' } })
      }),
    )

    await expect(
      uploadApi.uploadAvatar(new File(['avatar'], 'avatar.png', { type: 'image/png' })),
    ).rejects.toThrow('Avatar upload failed. Please try again.')
  })
})

describe('upload board binding (#936)', () => {
  test('sends board_id when the board has a durable identity', async () => {
    const file = new File(['image'], 'image.png', { type: 'image/png' })
    const received: { formData?: FormData } = {}
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/media-assets/direct', async ({ request }) => {
        received.formData = await request.formData()
        return HttpResponse.json({ data: assetFixture() })
      }),
    )

    await uploadApi.upload(file, WORKSPACE_ID, { boardId: 'board_abc123456789' })
    expect(received.formData?.get('board_id')).toBe('board_abc123456789')
  })

  test('omits board_id for unsaved boards (owner-only, no workspace window)', async () => {
    const file = new File(['image'], 'image.png', { type: 'image/png' })
    const received: { formData?: FormData } = {}
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/media-assets/direct', async ({ request }) => {
        received.formData = await request.formData()
        return HttpResponse.json({ data: assetFixture() })
      }),
    )

    await uploadApi.upload(file, WORKSPACE_ID, { boardId: 'local-unsaved-board' })
    expect(received.formData?.has('board_id')).toBe(false)
    await uploadApi.upload(file, WORKSPACE_ID)
    expect(received.formData?.has('board_id')).toBe(false)
  })

  test('workspaceUploadApi.uploadDirect uploads directly with board_id', async () => {
    const file = new File(['image'], 'image.png', { type: 'image/png' })
    const received: { formData?: FormData } = {}
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/media-assets/direct', async ({ request }) => {
        received.formData = await request.formData()
        return HttpResponse.json({ data: assetFixture() })
      }),
    )

    const asset = await workspaceUploadApi.uploadDirect(WORKSPACE_ID, file, {
      boardId: 'board_abc123456789',
    })
    expect(asset.id).toBe(ASSET_ID)
    expect(received.formData?.get('board_id')).toBe('board_abc123456789')
  })
})

describe('media delivery has no public bypass (#936)', () => {
  test('board id shape guard mirrors the backend board_ rule', () => {
    expect(isBoardPublicIdShape('board_abc123456789')).toBe(true)
    expect(isBoardPublicIdShape('board_a-b_c')).toBe(true)
    expect(isBoardPublicIdShape('local-unsaved-board')).toBe(false)
    expect(isBoardPublicIdShape('board_')).toBe(false)
    expect(isBoardPublicIdShape('board_bad id')).toBe(false)
    expect(isBoardPublicIdShape('')).toBe(false)
    expect(isBoardPublicIdShape(null)).toBe(false)
  })

  test('delivery URLs stay same-origin without signatures', async () => {
    const { resolveWorkspaceMediaAssetOriginalURL, resolveWorkspaceMediaAssetDerivativeURL } =
      await import('./mediaAssetUrls')
    const original = resolveWorkspaceMediaAssetOriginalURL('ws_board1', 'asset_upload42')
    const derivative = resolveWorkspaceMediaAssetDerivativeURL(
      'ws_board1',
      'asset_upload42',
      'large-preview',
    )
    for (const url of [original, derivative]) {
      expect(url).toContain('/api/v1/workspaces/ws_board1/media-assets/asset_upload42/')
      expect(url).not.toContain('?')
      expect(url.toLowerCase()).not.toContain('sig')
    }
  })
})
