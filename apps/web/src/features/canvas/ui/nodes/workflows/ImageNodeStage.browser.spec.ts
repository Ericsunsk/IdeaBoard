import { afterEach, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { render } from '../../../../../test/render'
import ImageNodeStage from './ImageNodeStage.vue'

vi.mock('@/api/media/mediaWorkspaceScope', () => ({
  readMediaWorkspaceId: () => 'ws_test123',
  setMediaWorkspaceIdResolver: () => {},
  resetMediaWorkspaceIdResolver: () => {},
}))

afterEach(() => {
  vi.restoreAllMocks()
})

async function imageBlob(color: string) {
  const canvas = document.createElement('canvas')
  canvas.width = 32
  canvas.height = 32
  canvas.getContext('2d')!.fillStyle = color
  canvas.getContext('2d')!.fillRect(0, 0, 32, 32)
  return new Promise<Blob>((resolve) => canvas.toBlob((blob) => resolve(blob!), 'image/png'))
}

it('keeps the decoded local image through upload handoff until the managed image is ready', async () => {
  let resolveMetadata!: (response: Response) => void
  const metadata = new Promise<Response>((resolve) => {
    resolveMetadata = resolve
  })
  const nativeFetch = globalThis.fetch
  vi.spyOn(globalThis, 'fetch').mockImplementation((input, options) => {
    const url = input instanceof Request ? input.url : input.toString()
    if (url.endsWith('/media-assets/asset_handoff')) return metadata
    if (url.includes('/events')) {
      return new Promise<Response>((_resolve, reject) => {
        options?.signal?.addEventListener('abort', () =>
          reject(new DOMException('Aborted', 'AbortError')),
        )
      })
    }
    return nativeFetch(input, options)
  })
  const localUrl = URL.createObjectURL(await imageBlob('red'))
  const managedUrl = URL.createObjectURL(await imageBlob('blue'))
  const view = render(ImageNodeStage, {
    props: {
      nodeId: 'image-1',
      stageWidth: 280,
      stageHeight: 280,
      displayImage: null,
      localUpload: { previewUrl: localUrl },
      isActive: false,
      isDragging: false,
    },
  })
  try {
    const image = view.container.querySelector('img')!
    await expect.poll(() => image.naturalWidth).toBe(32)
    // Complete the browser-preview work before upload finishes, as with a
    // normal network upload. Small images keep their decoded original frame.
    await expect.poll(() => image.classList.contains('is-tier-loading')).toBe(false)
    URL.revokeObjectURL(localUrl)
    await view.rerender({
      localUpload: undefined,
      displayImage: {
        url: '/api/v1/workspaces/ws_test123/media-assets/asset_handoff/derivatives/preview',
        assetId: 'asset_handoff',
      },
      isDragging: true,
    })
    await nextTick()
    expect(view.container.querySelector('img')?.naturalWidth).toBe(32)
    expect(view.container.querySelector('img')).toBe(image)
    expect(image).not.toHaveClass('is-tier-loading')
    expect(view.queryByLabelText('Preparing image preview')).toBeNull()

    resolveMetadata(
      new Response(
        JSON.stringify({
          data: {
            id: 'asset_handoff',
            workspace_id: 'ws_test123',
            status: 'ready',
            media_kind: 'image',
            mime_type: 'image/png',
            width: 32,
            height: 32,
            size_bytes: 100,
            content_url: '/api/v1/workspaces/ws_test123/media-assets/asset_handoff/content',
            derivatives: { preview: { status: 'ready', url: managedUrl } },
            created_at: '2026-11-17T00:00:00Z',
            updated_at: '2026-11-17T00:00:00Z',
          },
        }),
        { headers: { 'content-type': 'application/json' } },
      ),
    )
    await expect.poll(() => view.container.querySelector('img')?.src).toBe(managedUrl)
    await expect.poll(() => view.container.querySelector('img')?.naturalWidth).toBe(32)
  } finally {
    URL.revokeObjectURL(localUrl)
    URL.revokeObjectURL(managedUrl)
  }
})
