import { describe, expect, it, vi } from 'vitest'
import {
  resolveTaskImageResultAtIndexDisplay,
  resolveTaskImageResultDisplay,
} from './flowImageDirectActionTaskResult'

describe('flowImageDirectActionTaskResult', () => {
  it('keeps resolved task image dimensions for display sizing', async () => {
    const loadImageSize = vi.fn()

    await expect(
      resolveTaskImageResultDisplay(
        { url: ' https://example.com/output.png ', width: 400, height: 200 },
        1,
        loadImageSize,
      ),
    ).resolves.toEqual({
      url: 'https://example.com/output.png',
      ratio: 0.5,
      width: 400,
      height: 200,
    })
    expect(loadImageSize).not.toHaveBeenCalled()
  })

  it('preserves media asset id from task output metadata', async () => {
    const loadImageSize = vi.fn()

    await expect(
      resolveTaskImageResultDisplay(
        { url: 'https://example.com/output.png', width: 400, height: 200, mediaAssetId: 42 },
        1,
        loadImageSize,
      ),
    ).resolves.toMatchObject({
      url: 'https://example.com/output.png',
      mediaAssetId: 42,
    })
  })

  it('falls back to ratio when dimensions cannot be probed', async () => {
    const loadImageSize = vi.fn(async () => null)

    await expect(
      resolveTaskImageResultDisplay({ url: 'https://example.com/output.png' }, 1.25, loadImageSize),
    ).resolves.toEqual({
      url: 'https://example.com/output.png',
      ratio: 1.25,
    })
    expect(loadImageSize).toHaveBeenCalledWith('https://example.com/output.png')
  })

  it('resolves a stable batch slot by result index', async () => {
    await expect(
      resolveTaskImageResultAtIndexDisplay(
        [
          { url: 'https://example.com/second.png', resultIndex: 1 },
          { url: 'https://example.com/first.png', resultIndex: 0 },
        ],
        0,
        1,
        vi.fn(async () => ({ width: 100, height: 100 })),
      ),
    ).resolves.toMatchObject({ url: 'https://example.com/first.png', resultIndex: 0 })
  })

  it('fails a missing batch slot instead of falling through to another result', async () => {
    await expect(
      resolveTaskImageResultAtIndexDisplay(
        [
          { url: 'https://example.com/first.png', resultIndex: 0 },
          { url: 'https://example.com/third.png', resultIndex: 2 },
        ],
        1,
        1,
        vi.fn(async () => ({ width: 100, height: 100 })),
      ),
    ).rejects.toThrow('Task completed but did not return this image.')
  })
})
