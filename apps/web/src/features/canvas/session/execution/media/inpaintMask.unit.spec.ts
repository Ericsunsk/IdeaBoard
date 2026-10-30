import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  CanvasWorkspaceTaskUnmappableError,
  CanvasWorkspaceUnavailableError,
} from '../workspace/canvasWorkspaceTaskAdapter'
import { prepareInpaintMaskAsset } from './inpaintMask'

const WORKSPACE_ID = 'ws_test123'
const MASK_DATA_URL = 'data:image/png;base64,iVBORw0KGgo='

function maskBytes(): Uint8Array<ArrayBuffer> {
  return new Uint8Array([137, 80, 78, 71])
}

function stubDataUrlFetch() {
  const blob = new Blob([maskBytes()], { type: 'image/png' })
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    expect(String(input)).toBe(MASK_DATA_URL)
    return { blob: async () => blob } as Response
  })
}

function fakeTransport() {
  const uploadDirect = vi.fn(async (workspace: string, _file: Blob | File) => {
    expect(workspace).toBe(WORKSPACE_ID)
    return { id: 'asset_mask1' }
  })
  return {
    uploadDirect,
    transport: {
      uploadDirect,
    },
  }
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('prepareInpaintMaskAsset', () => {
  it('uploads mask directly in 1 RTT and returns the asset id', async () => {
    stubDataUrlFetch()
    const { uploadDirect, transport } = fakeTransport()

    const assetId = await prepareInpaintMaskAsset(MASK_DATA_URL, WORKSPACE_ID, {
      uploadTransport: transport as never,
      clockNow: () => 1700000000000,
    })

    expect(assetId).toBe('asset_mask1')
    expect(uploadDirect).toHaveBeenCalledWith(WORKSPACE_ID, expect.any(File))
    expect(uploadDirect.mock.calls[0]?.[1]).toMatchObject({
      name: 'flow-mask-1700000000000.png',
      type: 'image/png',
    })
  })

  it('propagates direct upload failure', async () => {
    stubDataUrlFetch()
    const { uploadDirect, transport } = fakeTransport()
    const failure = new Error('upload unavailable')
    uploadDirect.mockRejectedValueOnce(failure)
    await expect(
      prepareInpaintMaskAsset(MASK_DATA_URL, WORKSPACE_ID, {
        uploadTransport: transport as never,
      }),
    ).rejects.toBe(failure)
  })

  it('fails closed on empty masks, missing workspaces, and non-data-URL masks', async () => {
    const { transport } = fakeTransport()
    await expect(
      prepareInpaintMaskAsset('   ', WORKSPACE_ID, { uploadTransport: transport as never }),
    ).rejects.toThrow()
    await expect(
      prepareInpaintMaskAsset(MASK_DATA_URL, '  ', { uploadTransport: transport as never }),
    ).rejects.toBeInstanceOf(CanvasWorkspaceUnavailableError)
    try {
      await prepareInpaintMaskAsset('https://cdn.example/mask.png', WORKSPACE_ID, {
        uploadTransport: transport as never,
      })
      expect.unreachable()
    } catch (error) {
      expect(error).toBeInstanceOf(CanvasWorkspaceTaskUnmappableError)
      expect((error as CanvasWorkspaceTaskUnmappableError).reason).toBe('media-urls')
    }
    expect(transport.uploadDirect).not.toHaveBeenCalled()
  })
})
