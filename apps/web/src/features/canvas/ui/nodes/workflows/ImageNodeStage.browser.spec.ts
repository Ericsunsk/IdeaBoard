import { afterEach, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { render } from '../../../../../test/render'
import { createDeferred } from '../../../../../test/deferred'
import type { MediaAssetDeliveryMetadata } from '@/api/media/mediaAssetMetadata'
import {
  setMediaWorkspaceIdResolver,
  resetMediaWorkspaceIdResolver,
} from '@/api/media/mediaWorkspaceScope'
import ImageNodeStage from './ImageNodeStage.vue'

const media = vi.hoisted(() => ({ getMetadata: vi.fn() }))
vi.mock('@/api/media/mediaAssetMetadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/api/media/mediaAssetMetadata')>()),
  getMediaAssetMetadata: media.getMetadata,
  ensureWorkspaceMediaInvalidation: () => () => {},
  subscribeAssetInvalidation: () => () => {},
}))

afterEach(() => {
  resetMediaWorkspaceIdResolver()
  vi.clearAllMocks()
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
  setMediaWorkspaceIdResolver(() => 'ws_test123')
  const metadata = createDeferred<MediaAssetDeliveryMetadata>()
  media.getMetadata.mockReturnValue(metadata.promise)
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

    metadata.resolve({
      asset_id: 'asset_handoff',
      status: 'ready',
      mime_type: 'image/png',
      width: 32,
      height: 32,
      size_bytes: 100,
      original: { status: 'ready', url: managedUrl },
      derivatives: { preview: { status: 'ready', url: managedUrl } },
    })
    await expect.poll(() => view.container.querySelector('img')?.src).toBe(managedUrl)
    await expect.poll(() => view.container.querySelector('img')?.naturalWidth).toBe(32)
  } finally {
    URL.revokeObjectURL(localUrl)
    URL.revokeObjectURL(managedUrl)
  }
})
