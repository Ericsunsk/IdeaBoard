import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAppClipboard } from './useAppClipboard'

describe('useAppClipboard', () => {
  const clipboardDescriptor = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
  const execCommandDescriptor = Object.getOwnPropertyDescriptor(document, 'execCommand')
  const writeText = vi.fn()
  const execCommand = vi.fn()

  beforeEach(() => {
    writeText.mockReset().mockResolvedValue(undefined)
    execCommand.mockReset().mockReturnValue(false)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText, write: writeText },
    })
    Object.defineProperty(document, 'execCommand', { configurable: true, value: execCommand })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    if (clipboardDescriptor) Object.defineProperty(navigator, 'clipboard', clipboardDescriptor)
    else Reflect.deleteProperty(navigator, 'clipboard')
    if (execCommandDescriptor) Object.defineProperty(document, 'execCommand', execCommandDescriptor)
    else Reflect.deleteProperty(document, 'execCommand')
  })

  it('reports success only after the native text write completes', async () => {
    let finishWrite!: () => void
    writeText.mockReturnValue(new Promise<void>((resolve) => (finishWrite = resolve)))
    const result = useAppClipboard().copyText('IdeaBoard')
    const resolved = vi.fn()
    void result.then(resolved)

    expect(writeText).toHaveBeenCalledWith('IdeaBoard')
    await Promise.resolve()
    expect(resolved).not.toHaveBeenCalled()
    finishWrite()
    await expect(result).resolves.toBe(true)
    expect(execCommand).not.toHaveBeenCalled()
  })

  it('reports a denied native write even when the copy fallback resolves without copying', async () => {
    writeText.mockRejectedValue(new DOMException('Denied', 'NotAllowedError'))

    await expect(useAppClipboard().copyText('one-time reward code')).resolves.toBe(false)
    expect(execCommand).not.toHaveBeenCalled()
  })

  it('reports unavailable clipboard support without using a copy fallback', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined })

    await expect(useAppClipboard().copyText('IdeaBoard')).resolves.toBe(false)
    expect(execCommand).not.toHaveBeenCalled()
  })

  it('reports a missing text-write method', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: {} })

    await expect(useAppClipboard().copyText('IdeaBoard')).resolves.toBe(false)
    expect(execCommand).not.toHaveBeenCalled()
  })

  it('is safe without a browser navigator', async () => {
    vi.stubGlobal('navigator', undefined)

    await expect(useAppClipboard().copyText('IdeaBoard')).resolves.toBe(false)
  })

  it('rejects empty text without attempting to copy', async () => {
    await expect(useAppClipboard().copyText('')).resolves.toBe(false)
    expect(writeText).not.toHaveBeenCalled()
    expect(execCommand).not.toHaveBeenCalled()
  })
})
