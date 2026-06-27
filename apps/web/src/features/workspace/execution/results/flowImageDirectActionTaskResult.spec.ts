import { describe, expect, it, vi } from 'vitest'
import { resolveTaskImageResultDisplay } from './flowImageDirectActionTaskResult'

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
})
