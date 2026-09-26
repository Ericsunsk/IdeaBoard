import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { setMediaWorkspaceIdResolver, resetMediaWorkspaceIdResolver } from '@/shared/api/mediaWorkspaceScope'
import { getWorkspaceMediaAssetMetadata } from '@/shared/api/mediaAssetDelivery'

vi.mock('@/shared/api/workspaceEventStream', () => ({
  subscribeWorkspaceMediaInvalidation: vi.fn(() => vi.fn()),
}))

describe('debug delivery', () => {
  beforeEach(() => {
    setMediaWorkspaceIdResolver(() => 'ws_test123')
  })
  afterEach(() => {
    resetMediaWorkspaceIdResolver()
  })
  it('fetches and parses', async () => {
    const calls: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        calls.push(typeof input === 'string' ? input : input.toString())
        return new Response(
          JSON.stringify({
            data: {
              id: 'asset-42',
              workspace_id: 'ws_test123',
              status: 'ready',
              media_kind: 'image',
              mime_type: 'image/jpeg',
              width: 2048,
              height: 1024,
              size_bytes: 100,
              checksum_sha256:
                'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
              content_url: '/api/v1/workspaces/ws_test123/media-assets/asset-42/content',
              derivatives: {
                preview: {
                  status: 'ready',
                  format: 'webp',
                  url: '/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/preview',
                },
              },
              created_at: '2026-09-25T00:00:00.000Z',
              updated_at: '2026-09-25T00:00:00.000Z',
            },
          }),
          { headers: { 'content-type': 'application/json' } },
        )
      }),
    )
    try {
      const meta = await getWorkspaceMediaAssetMetadata('ws_test123', 'asset-42')
      console.log('META_OK', JSON.stringify(meta.derivatives))
    } catch (error) {
      console.log('META_FAIL', String(error))
    }
    console.log('CALLS', JSON.stringify(calls))
    expect(true).toBe(true)
  })
})
