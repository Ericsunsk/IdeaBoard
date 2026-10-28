import { cleanup } from '@testing-library/vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick, type Ref } from 'vue'
import { render } from '../../../../../test/render'
import ComposingOrb from './ComposingOrb.vue'

const themeBox = vi.hoisted(() => ({ isDark: null as Ref<boolean> | null }))

vi.mock('../../../../../theme/composables/useTheme', async () => {
  const { ref } = await import('vue')
  const isDark = ref(false)
  themeBox.isDark = isDark
  return {
    isDarkModeActive: () => isDark.value,
    useTheme: () => ({ isDark }),
  }
})

function themeIsDark() {
  const isDark = themeBox.isDark
  if (!isDark) throw new Error('theme mock not installed')
  return isDark
}

function setThemeDark(isDark: boolean) {
  themeIsDark().value = isDark
}

function installBrowserMocks(isReducedMotionPreferred: boolean) {
  const colors: string[] = []
  const context = {
    fillStyle: '',
    setTransform: vi.fn(),
    clearRect: vi.fn(),
    beginPath: vi.fn(),
    arc: vi.fn(() => {
      colors.push(String(context.fillStyle))
    }),
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
  vi.stubGlobal(
    'matchMedia',
    () =>
      ({
        matches: isReducedMotionPreferred,
        media: '(prefers-reduced-motion: reduce)',
        onchange: null,
        addEventListener: addMotionListener,
        removeEventListener: removeMotionListener,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        dispatchEvent: vi.fn(),
      }) satisfies Partial<MediaQueryList>,
  )
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
    colors,
    context,
    disconnectObserver,
    removeMotionListener,
    requestAnimationFrame,
  }
}

afterEach(() => {
  setThemeDark(false)
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

  it('paints the inverted palette when the dark theme is active', () => {
    setThemeDark(false)
    const light = installBrowserMocks(true)
    render(ComposingOrb)
    const lightColors = [...light.colors]
    cleanup()

    setThemeDark(true)
    const dark = installBrowserMocks(true)
    render(ComposingOrb)
    const darkColors = [...dark.colors]
    cleanup()

    expect(darkColors).toHaveLength(566)
    expect(darkColors).not.toEqual(lightColors)
    const averageInk = (colors: string[]) => {
      const grays = colors.map((color) => Number(/rgba\((\d+),/.exec(color)?.[1] ?? NaN))
      expect(grays.every(Number.isFinite)).toBe(true)
      return grays.reduce((sum, gray) => sum + gray, 0) / grays.length
    }
    expect(Math.abs(averageInk(darkColors) - averageInk(lightColors))).toBeGreaterThan(10)
  })

  it('repaints the static frame when the theme changes under reduced motion', async () => {
    const browser = installBrowserMocks(true)
    render(ComposingOrb)
    expect(browser.context.arc).toHaveBeenCalledTimes(566)
    expect(browser.requestAnimationFrame).not.toHaveBeenCalled()
    const lightColors = [...browser.colors]

    setThemeDark(true)
    await nextTick()

    expect(browser.context.arc).toHaveBeenCalledTimes(566 * 2)
    expect(browser.requestAnimationFrame).not.toHaveBeenCalled()
    const repaintedColors = browser.colors.slice(566)
    expect(repaintedColors).toHaveLength(566)
    expect(repaintedColors).not.toEqual(lightColors)

    cleanup()
  })
})
