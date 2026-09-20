import { afterEach, describe, expect, it, vi } from 'vitest'
import { useAppClipboard } from './useAppClipboard'

describe('useAppClipboard', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('copies through the Clipboard API', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })

    await expect(useAppClipboard().copyText('IdeaBoard')).resolves.toBe(true)
    expect(writeText).toHaveBeenCalledWith('IdeaBoard')
  })

  it('reports rejected clipboard writes', async () => {
    vi.stubGlobal('navigator', {
      clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) },
    })

    await expect(useAppClipboard().copyText('IdeaBoard')).resolves.toBe(false)
  })

  it('rejects empty text and unavailable clipboard access', async () => {
    vi.stubGlobal('navigator', { clipboard: undefined })

    await expect(useAppClipboard().copyText('')).resolves.toBe(false)
    await expect(useAppClipboard().copyText('IdeaBoard')).resolves.toBe(false)
  })
})
