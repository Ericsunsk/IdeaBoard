import { describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../test/msw'
import { UPLOAD_OPERATION_TIMEOUT_MS, uploadApi } from './upload'

const WORKSPACE_ID = 'ws_board1'
const UPLOAD_ID = 'upload_upload42'
const ASSET_ID = 'asset_upload42'
const CHECKSUM = 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'

function sessionFixture(overrides: Record<string, unknown> = {}) {
  return {
    id: UPLOAD_ID,
    workspace_id: WORKSPACE_ID,
    status: 'pending',
    filename: 'image.png',
    mime_type: 'image/png',
    size_bytes: 5,
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
    http.post('/api/v1/workspaces/:workspaceId/uploads', () =>
      HttpResponse.json({ data: sessionFixture() }),
    ),
    http.put('/api/v1/workspaces/:workspaceId/uploads/:uploadId/bytes', () =>
      HttpResponse.json({ data: sessionFixture({ status: 'transferring' }) }),
    ),
    http.post('/api/v1/workspaces/:workspaceId/uploads/:uploadId/complete', () =>
      HttpResponse.json({
        data: {
          upload: sessionFixture({ status: 'completed', asset_id: ASSET_ID }),
          asset: assetFixture(),
        },
      }),
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

        return HttpResponse.json({ data: { avatar_url: '/uploads/profile/user/avatar' } })
      }),
    )

    await expect(uploadApi.uploadAvatar(file)).resolves.toEqual({
      avatar: '/uploads/profile/user/avatar',
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

          return HttpResponse.json({ data: { avatar_url: '/uploads/profile/me/slow-avatar' } })
        }),
      )

      const response = uploadApi.uploadAvatar(file)

      await uploadStarted
      expect(UPLOAD_OPERATION_TIMEOUT_MS).toBeGreaterThan(slowResponseDelayMs)
      await vi.advanceTimersByTimeAsync(slowResponseDelayMs)

      await expect(response).resolves.toEqual({
        avatar: '/uploads/profile/me/slow-avatar',
        mime: 'image/jpeg',
        ext: '.jpg',
      })
    } finally {
      vi.useRealTimers()
    }
  })

  it('surfaces canonical workspace upload failures from the HTTP response body', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/uploads', () => {
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
      http.post('/api/v1/workspaces/:workspaceId/uploads', () => {
        return HttpResponse.json({ meta: { request_id: 'req-empty' } })
      }),
    )

    await expect(
      uploadApi.upload(new File(['image'], 'image.png', { type: 'image/png' }), WORKSPACE_ID),
    ).rejects.toThrow('Malformed API envelope')
  })

  it('rejects workspace uploads that were not registered as media assets', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/uploads', () =>
        HttpResponse.json({ data: sessionFixture() }),
      ),
      http.put('/api/v1/workspaces/:workspaceId/uploads/:uploadId/bytes', () =>
        HttpResponse.json({ data: sessionFixture({ status: 'transferring' }) }),
      ),
      http.post('/api/v1/workspaces/:workspaceId/uploads/:uploadId/complete', () =>
        HttpResponse.json({ data: { upload: sessionFixture({ status: 'completed' }) } }),
      ),
    )

    await expect(
      uploadApi.upload(new File(['image'], 'image.png', { type: 'image/png' }), WORKSPACE_ID),
    ).rejects.toThrow('Upload failed. Please try again.')
  })

  it('preserves structured workspace transport errors without exposing unsafe legacy fields', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/uploads', () => {
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
