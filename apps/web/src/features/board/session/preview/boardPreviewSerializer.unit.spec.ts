import { afterEach, describe, expect, it, vi } from 'vitest'
import { makeBoardSnapshot } from '../../testing'
import { renderBoardPreviewDataUrl } from './boardPreviewSerializer'
import type { ImageNodeItem } from '../../core/document/nodes'
import { BLANK_BOARD_PREVIEW_DOT_OFFSET, BLANK_BOARD_PREVIEW_DOT_RADIUS } from './boardPreviewStyle'
import {
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from '../../../../api/mediaWorkspaceScope'

function createMockCanvasContext() {
  return {
    set fillStyle(_value: unknown) {},
    set imageSmoothingEnabled(_value: unknown) {},
    set imageSmoothingQuality(_value: unknown) {},
    beginPath: vi.fn(),
    arc: vi.fn(),
    createLinearGradient: vi.fn(() => ({ addColorStop: vi.fn() })),
    fill: vi.fn(),
    fillRect: vi.fn(),
  }
}

describe('renderBoardPreviewDataUrl', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    resetMediaWorkspaceIdResolver()
  })

  it.each<{
    name: string
    node: Partial<ImageNodeItem>
    expectedPath: string
  }>([
    {
      name: 'imported source without a runtime URL',
      node: {
        sourceMediaRef: { assetId: 'asset_test123', derivativeTier: 'preview' },
      },
      expectedPath: '/api/v1/workspaces/ws_test123/media-assets/asset_test123/derivatives/preview',
    },
    {
      name: 'source with an expired runtime URL',
      node: {
        sourceImageUrl: '/expired.png',
        sourceMediaRef: { assetId: 'asset_test123', derivativeTier: 'preview' },
      },
      expectedPath: '/api/v1/workspaces/ws_test123/media-assets/asset_test123/derivatives/preview',
    },
    {
      name: 'latest output with a durable MediaRef',
      node: {
        sourceImageUrl: '/source.png',
        outputImages: [
          { url: '/old.png', ratio: 1 },
          {
            url: '/expired.png',
            ratio: 1,
            mediaRef: { assetId: 'asset_output123', derivativeTier: 'large-preview' },
          },
        ],
      },
      expectedPath:
        '/api/v1/workspaces/ws_test123/media-assets/asset_output123/derivatives/large-preview',
    },
    {
      name: 'source with a direct URL',
      node: { sourceImageUrl: '/source.png' },
      expectedPath: '/source.png',
    },
  ])('renders $name in the Board preview', async ({ node, expectedPath }) => {
    setMediaWorkspaceIdResolver(() => 'ws_test123')
    const loadedUrls: string[] = []
    vi.stubGlobal(
      'Image',
      class {
        onload?: () => void
        naturalWidth = 640
        naturalHeight = 640
        set src(url: string) {
          loadedUrls.push(url)
          this.onload?.()
        }
      },
    )
    const context = {
      ...createMockCanvasContext(),
      save: vi.fn(),
      restore: vi.fn(),
      moveTo: vi.fn(),
      arcTo: vi.fn(),
      lineTo: vi.fn(),
      quadraticCurveTo: vi.fn(),
      closePath: vi.fn(),
      clip: vi.fn(),
      stroke: vi.fn(),
      drawImage: vi.fn(),
    }
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(
      () => context as unknown as CanvasRenderingContext2D,
    )
    vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockReturnValue(
      'data:image/png;base64,preview',
    )

    await renderBoardPreviewDataUrl(
      makeBoardSnapshot({
        imageNodes: [
          {
            id: 1,
            type: 'image-node',
            x: 0,
            y: 0,
            width: 280,
            sourceImageUrl: '',
            sourceImageRatio: 1,
            outputStatus: 'ready',
            outputImages: [],
            ...node,
          },
        ],
      }),
    )

    expect(loadedUrls).toEqual([`${window.location.origin}${expectedPath}`])
    expect(context.drawImage).toHaveBeenCalledOnce()
  })

  it('renders an empty canvas preview for empty snapshots', async () => {
    const context = createMockCanvasContext()
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(
      () => context as unknown as CanvasRenderingContext2D,
    )
    vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockImplementation(
      (mimeType?: string) => `data:${mimeType || 'image/png'};base64,empty`,
    )

    const result = await renderBoardPreviewDataUrl(makeBoardSnapshot())

    expect(result).toBe('data:image/avif;base64,empty')
    expect(context.fillRect).toHaveBeenCalledWith(0, 0, 760, 490)
    expect(context.arc).toHaveBeenCalledWith(
      BLANK_BOARD_PREVIEW_DOT_OFFSET,
      BLANK_BOARD_PREVIEW_DOT_OFFSET,
      BLANK_BOARD_PREVIEW_DOT_RADIUS,
      0,
      Math.PI * 2,
    )
  })

  it('renders dark mode preview when isDark option is enabled', async () => {
    let lastFillStyle: unknown
    const context = {
      ...createMockCanvasContext(),
      set fillStyle(value: unknown) {
        lastFillStyle = value
      },
    }
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(
      () => context as unknown as CanvasRenderingContext2D,
    )
    vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockImplementation(
      (mimeType?: string) => `data:${mimeType || 'image/png'};base64,empty-dark`,
    )

    const result = await renderBoardPreviewDataUrl(makeBoardSnapshot(), { isDark: true })

    expect(result).toBe('data:image/avif;base64,empty-dark')
    expect(context.fillRect).toHaveBeenCalledWith(0, 0, 760, 490)
    expect(lastFillStyle).toBe('#282828')
  })
})
