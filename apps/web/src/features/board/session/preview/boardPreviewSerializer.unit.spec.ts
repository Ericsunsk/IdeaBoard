import { afterEach, describe, expect, it, vi } from 'vitest'
import { makeBoardSnapshot } from '../../testing'
import { renderBoardPreviewDataUrl } from './boardPreviewSerializer'
import { BLANK_BOARD_PREVIEW_DOT_OFFSET, BLANK_BOARD_PREVIEW_DOT_RADIUS } from './boardPreviewStyle'

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
