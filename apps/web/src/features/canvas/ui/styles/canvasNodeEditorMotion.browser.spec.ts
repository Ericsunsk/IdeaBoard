import '@testing-library/jest-dom/vitest'

// Canvas Node and editor motion browser contract.
//
// Observable replacement for the retired
// `canvasNodeEditorMotion.contract.spec.ts` source-string assertions:
// every retired constraint needs an effective computed-style replacement
// passing first. Follows the board/search P3 precedent
// (`boardCardMotion.browser.spec.ts`, `searchChromeStyle.browser.spec.ts`):
// assert rendered output — computed durations, live token provenance and
// reduced-motion states — never file text.
//
// Coverage map (ledger
// `src/features/canvas/presentation/styles/canvasNodeEditorMotion.contract.spec.ts`):
// - motion token mapping (scenario 1, `computed-style/7bbbb76c`, 5 IDs):
//   control/enter/exit/feedback durations land on probes through the live
//   motion tokens (moving a token moves the paint; a hardcoded value or a
//   legacy `--canvas-motion-*` indirection cannot satisfy this).
// - reduced-motion stops (scenario 2, `computed-style/d05c9530`, 2 IDs):
//   the running border cycle and the fusion apply ray animate by default
//   and stop when reduced motion is preferred.
// - precision cursors (scenario 3, `computed-style/86d199c8`, 1 ID):
//   the inpaint brush cursor never rides a transform transition.

import { commands, page } from 'vitest/browser'
import { beforeEach, afterEach, describe, expect, it } from 'vitest'
import { createApp, h, type App } from 'vue'
import { IconoirProvider } from '@iconoir/vue'
import { i18n } from '../../../../core/i18n'
import '../../../../app/style.css'
import '../styles/canvas-surface-skin.css'
import '../styles/canvas-style-contracts.css'
import '../styles/node-input-thumbnails.css'
import { FusionStage } from '../nodes/workflows'
import { ImageInpaintOverlay } from '../../tools/inpaint'

declare module 'vitest/internal/browser' {
  interface BrowserCommands {
    emulateMotion: (reducedMotion: 'reduce' | 'no-preference') => Promise<void>
  }
}

const CONTROL_EASING = 'cubic-bezier(0.2, 0.8, 0.2, 1)'

function styleOf(element: Element): CSSStyleDeclaration {
  return window.getComputedStyle(element)
}

function durationsOf(element: Element): string[] {
  return styleOf(element)
    .transitionDuration.split(',')
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0)
}

async function setMotion(isReduced: boolean) {
  await commands.emulateMotion(isReduced ? 'reduce' : 'no-preference')
}

function mountHost(inner: string) {
  const root = document.createElement('div')
  root.className = 'canvas-surface-skin'
  root.innerHTML = inner
  document.body.append(root)
  return {
    root,
    query: <T extends HTMLElement = HTMLElement>(selector: string) =>
      root.querySelector<T>(selector),
    unmount() {
      root.remove()
    },
  }
}

function mountApp(node: () => unknown) {
  const root = document.createElement('div')
  root.className = 'canvas-surface-skin'
  root.style.cssText = 'width: 800px; height: 600px;'
  document.body.append(root)
  const app: App = createApp({
    setup() {
      return () =>
        h(
          IconoirProvider,
          { iconProps: { color: 'currentColor', width: '1.5em', height: '1.5em' } },
          { default: () => node() },
        )
    },
  })
  app.use(i18n)
  app.mount(root)
  return {
    root,
    query: <T extends HTMLElement = HTMLElement>(selector: string) =>
      root.querySelector<T>(selector),
    unmount() {
      app.unmount()
      root.remove()
    },
  }
}

function withToken(token: string, value: string, run: () => void) {
  document.documentElement.style.setProperty(token, value)
  try {
    run()
  } finally {
    document.documentElement.style.removeProperty(token)
  }
}

describe('Canvas Node and editor motion browser contract', () => {
  beforeEach(async () => {
    await page.viewport(1400, 800)
    i18n.global.locale.value = 'en'
    await setMotion(false)
  })

  afterEach(async () => {
    document.body.innerHTML = ''
    await setMotion(false)
  })

  it('maps Node and editor presentation to the semantic Motion Policy tokens', async () => {
    const host = mountHost(`
      <div class="workflow-node-header">Title</div>
      <div class="canvas-chrome-region"><div class="canvas-chrome-region--top"></div></div>
      <div class="canvas-tool-tooltip-trigger"><span class="canvas-tool-tooltip">Tip</span></div>
    `)
    try {
      const header = host.query('.workflow-node-header')!
      const chrome = host.query('.canvas-chrome-region')!
      const tooltip = host.query('.canvas-tool-tooltip')!
      const rootStyle = styleOf(document.documentElement)

      // Resting paints ride the live tokens (control 160ms, enter 220ms,
      // feedback 160ms) — a hardcoded duration cannot satisfy this.
      expect(rootStyle.getPropertyValue('--motion-control-duration').trim()).toBe('160ms')
      expect(rootStyle.getPropertyValue('--motion-enter-duration').trim()).toBe('220ms')
      expect(rootStyle.getPropertyValue('--motion-exit-duration').trim()).toBe('220ms')
      expect(rootStyle.getPropertyValue('--motion-feedback-duration').trim()).toBe('160ms')
      expect(durationsOf(header)).toEqual(['0.16s', '0.16s'])
      expect(styleOf(header).transitionTimingFunction).toContain(CONTROL_EASING)
      expect(durationsOf(chrome)).toEqual(['0.22s', '0.22s', '0.22s'])
      expect(durationsOf(tooltip)).toEqual(['0.16s', '0.16s'])

      // Provenance: moving the control token moves the Node paint, so a
      // hardcoded Node-side value (or a legacy --canvas-motion-* alias)
      // cannot satisfy this.
      withToken('--motion-control-duration', '999ms', () => {
        expect(durationsOf(header)).toEqual(['0.999s', '0.999s'])
      })
      expect(durationsOf(header)).toEqual(['0.16s', '0.16s'])

      // Provenance: moving the feedback token moves the tooltip reveal.
      withToken('--motion-feedback-duration', '999ms', () => {
        expect(durationsOf(tooltip)).toEqual(['0.999s', '0.999s'])
      })

      // The hidden chrome state rides the exit token, not the enter one.
      chrome.classList.add('is-hidden')
      expect(durationsOf(chrome)).toEqual(['0.22s', '0.22s', '0.22s'])
      withToken('--motion-exit-duration', '999ms', () => {
        expect(durationsOf(chrome)).toEqual(['0.999s', '0.999s', '0.999s'])
      })
    } finally {
      host.unmount()
    }
  })

  it('stops continuous running and apply motion when reduced motion is requested', async () => {
    const running = mountHost(
      '<div class="workflow-node running" style="position: relative"></div>',
    )
    try {
      const node = running.query('.workflow-node.running')!
      expect(window.getComputedStyle(node, '::before').animationName).toBe(
        'node-running-border-flow',
      )

      await setMotion(true)
      expect(matchMedia('(prefers-reduced-motion: reduce)').matches).toBe(true)
      // The app motion policy zeroes every interaction token under reduce.
      expect(
        styleOf(document.documentElement).getPropertyValue('--motion-control-duration').trim(),
      ).toBe('0ms')
      expect(window.getComputedStyle(node, '::before').animationName).toBe('none')
    } finally {
      running.unmount()
    }
    await setMotion(false)

    const fusion = mountApp(() =>
      h(FusionStage, {
        isActive: true,
        blendPoint: { x: 0.5, y: 0.5 },
        inputImages: [
          { id: 'image-1', url: 'https://example.com/a.png' },
          { id: 'image-2', url: 'https://example.com/b.png' },
        ] as never,
        width: 320,
      }),
    )
    try {
      const ray = await waitForRay(fusion.root)
      // Vue hashes scoped keyframe names; what matters is that the apply
      // ray cycles by default and stops under reduced motion.
      expect(window.getComputedStyle(ray).animationName).not.toBe('none')

      await setMotion(true)
      expect(window.getComputedStyle(ray).animationName).toBe('none')
    } finally {
      fusion.unmount()
    }
  })

  it('keeps precision cursors free from transform animation', async () => {
    const overlay = mountApp(() =>
      h(ImageInpaintOverlay, {
        canvasWidth: 320,
        canvasHeight: 240,
        maskLayerOpacity: 60,
        isImageLayerVisible: true,
        brushSize: 24,
        setCanvasRef: () => {},
      }),
    )
    try {
      const cursor = overlay.query('.inpaint-brush-cursor')!
      // The cursor fades (opacity, feedback token) but never slides: a
      // transform transition would smear the brush while painting.
      expect(styleOf(cursor).transitionProperty).toBe('opacity')
      expect(styleOf(cursor).transitionProperty).not.toContain('transform')
      expect(durationsOf(cursor)).toEqual(['0.16s'])
    } finally {
      overlay.unmount()
    }
  })
})

async function waitForRay(root: HTMLElement): Promise<HTMLElement> {
  const started = Date.now()
  for (;;) {
    const ray = root.querySelector<HTMLElement>('.fusion-node-ray')
    if (ray) return ray
    if (Date.now() - started > 5000) throw new Error('fusion ray missing')
    await new Promise((resolve) => setTimeout(resolve, 25))
  }
}
