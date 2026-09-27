// Inpaint mask asset spec: data-URL masks travel the workspace
// upload-session flow (declare -> stage bytes -> complete) and resolve to an
// opaque asset id for `params.mask_asset_id`. Transport, digest, clock, and
// fetch are injected so happy-dom never touches real bytes or crypto.
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  CanvasWorkspaceTaskUnmappableError,
  CanvasWorkspaceUnavailableError,
} from '../workspace/canvasWorkspaceTaskAdapter'
import { prepareInpaintMaskAsset } from './inpaintMask'

const WORKSPACE_ID = 'ws_test123'
const MASK_DATA_URL = 'data:image/png;base64,iVBORw0KGgo='
const CHECKSUM = 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'

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
  const calls: Array<{ step: string; workspace: string; upload?: string }> = []
  return {
    calls,
    transport: {
      createSession: vi.fn(async (workspace: string) => {
        calls.push({ step: 'create', workspace })
        return { id: 'upload_mask1' }
      }),
      stageBytes: vi.fn(async (workspace: string, upload: string) => {
        calls.push({ step: 'stage', workspace, upload })
        return { id: upload }
      }),
      completeSession: vi.fn(async (workspace: string, upload: string) => {
        calls.push({ step: 'complete', workspace, upload })
        return { upload: { id: upload }, asset: { id: 'asset_mask1' } }
      }),
    },
  }
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('prepareInpaintMaskAsset', () => {
  it('uploads mask bytes through the session flow and returns the asset id', async () => {
    stubDataUrlFetch()
    const { calls, transport } = fakeTransport()
    const minted: string[] = []

    const assetId = await prepareInpaintMaskAsset(MASK_DATA_URL, WORKSPACE_ID, {
      uploadTransport: transport as never,
      mintIdempotencyKey: (op) => {
        minted.push(op)
        return `upload:${op}:1`
      },
      digestSha256Hex: async () => CHECKSUM,
      clockNow: () => 1700000000000,
    })

    expect(assetId).toBe('asset_mask1')
    expect(calls).toEqual([
      { step: 'create', workspace: WORKSPACE_ID },
      { step: 'stage', workspace: WORKSPACE_ID, upload: 'upload_mask1' },
      { step: 'complete', workspace: WORKSPACE_ID, upload: 'upload_mask1' },
    ])
    expect(minted).toEqual(['create', 'bytes', 'complete'])
    expect(transport.createSession).toHaveBeenCalledWith(
      WORKSPACE_ID,
      {
        filename: 'flow-mask-1700000000000.png',
        mimeType: 'image/png',
        sizeBytes: 4,
        checksumSha256: CHECKSUM,
        mediaKind: 'image',
      },
      { idempotencyKey: 'upload:create:1' },
    )
    expect(transport.stageBytes).toHaveBeenCalledWith(
      WORKSPACE_ID,
      'upload_mask1',
      expect.any(Blob),
      { contentType: 'image/png', idempotencyKey: 'upload:bytes:1' },
    )
    expect(transport.completeSession).toHaveBeenCalledWith(
      WORKSPACE_ID,
      'upload_mask1',
      { checksumSha256: CHECKSUM, sizeBytes: 4, mimeType: 'image/png', mediaKind: 'image' },
      { idempotencyKey: 'upload:complete:1' },
    )
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
    expect(transport.createSession).not.toHaveBeenCalled()
  })
})
