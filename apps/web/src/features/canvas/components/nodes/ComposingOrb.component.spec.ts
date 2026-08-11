import { cleanup } from '@testing-library/vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { render } from '../../../../test/render'
import ComposingOrb from './ComposingOrb.vue'

function installBrowserMocks(prefersReducedMotion: boolean) {
  const context = {
    fillStyle: '',
    setTransform: vi.fn(),
    clearRect: vi.fn(),
    beginPath: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
  } as unknown as CanvasRenderingContext2D
  const requestAnimationFrame = vi.fn(() => 7)
  const cancelAnimationFrame = vi.fn()
  const addMotionListener = vi.fn()
  const removeMotionListener = vi.fn()
  const disconnectObserver = vi.fn()

  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(context)
  vi.stubGlobal('requestAnimationFrame', requestAnimationFrame)
  vi.stubGlobal('cancelAnimationFrame', cancelAnimationFrame)
  vi.stubGlobal('matchMedia', () => ({
    matches: prefersReducedMotion,
    media: '(prefers-reduced-motion: reduce)',
    onchange: null,
    addEventListener: addMotionListener,
    removeEventListener: removeMotionListener,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }))
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      private readonly callback: IntersectionObserverCallback

      constructor(callback: IntersectionObserverCallback) {
        this.callback = callback
      }

      observe() {
        this.callback([{ isIntersecting: true } as IntersectionObserverEntry], this as never)
      }

      disconnect() {
        disconnectObserver()
      }

      unobserve() {}

      takeRecords() {
        return []
      }

      readonly root = null
      readonly rootMargin = '0px'
      readonly thresholds = [0]
    },
  )

  return {
    cancelAnimationFrame,
    context,
    disconnectObserver,
    removeMotionListener,
    requestAnimationFrame,
  }
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('ComposingOrb', () => {
  it('starts the composing animation while visible', () => {
    const browser = installBrowserMocks(false)
    const view = render(ComposingOrb)

    expect(view.container.querySelector('canvas')).toHaveAttribute('aria-hidden', 'true')
    expect(browser.context.clearRect).toHaveBeenCalledWith(0, 0, 171, 171)
    expect(browser.requestAnimationFrame).toHaveBeenCalledTimes(1)

    cleanup()
    expect(browser.cancelAnimationFrame).toHaveBeenCalledWith(7)
    expect(browser.disconnectObserver).toHaveBeenCalledTimes(1)
    expect(browser.removeMotionListener).toHaveBeenCalledTimes(1)
  })

  it('renders a static frame when reduced motion is requested', () => {
    const browser = installBrowserMocks(true)
    render(ComposingOrb)

    expect(browser.context.arc).toHaveBeenCalledTimes(566)
    expect(browser.requestAnimationFrame).not.toHaveBeenCalled()

    cleanup()
  })
})
