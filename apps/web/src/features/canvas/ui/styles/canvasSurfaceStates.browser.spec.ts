import '@testing-library/jest-dom/vitest'

// Canvas surface states browser contract.
//
// Observable replacement for the retired
// `canvasSurfaceSkin.contract.spec.ts` "retains the focus, selected,
// active, disabled, loading, reduced-motion, and responsive contracts"
// source-string assertions (`computed-style/b611fda8`, 22 IDs): every
// retired constraint needs an effective computed-style replacement passing
// first. Follows the board/search P3 precedent
// (`boardCardMotion.browser.spec.ts`, `searchChromeStyle.browser.spec.ts`):
// assert rendered output — computed paint, live token provenance and state
// classes — never file text.
//
// Coverage map (ledger
// `src/features/canvas/presentation/styles/canvasSurfaceSkin.contract.spec.ts`,
// scenario "retains ...", lines 80-101):
// - chrome icon-button focus ring + responsive status rail (L80, L83-84):
//   focus lands the shared focus-ring paint; the status rail keeps its
//   viewport-relative max width.
// - chrome ownership negatives (L81-82): the focus paint rides the shared
//   page-chrome token live (moving it moves the paint), and the retired
//   bordered-control node never renders.
// - edge selected/focus + minimap (L85-87, L90): a selected edge paints the
//   selected token; the minimap mounts the shared surface classes and paints
//   nodes from the minimap token.
// - adapter ownership negatives (L88-89): edge paint does NOT follow the
//   page-chrome tokens when they move.
// - node running ring (L91-92): the running ::before layer animates by
//   default and stops under reduced motion.
// - output-count active/disabled (L93-94): active paints the accent pair;
//   disabled waits at reduced opacity.
// - stage-frame states (L95-100): loading and connection-drop-target raise
//   the emphasis glow; the frame owns no border, paints its ::after ring,
//   and activates it.
// - inpaint generate wiring (L101): the composer disables while generating.

import { commands, page, userEvent as browserUser } from 'vitest/browser'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, ref, type App } from 'vue'
import { IconoirProvider } from '@iconoir/vue'
import { VueFlow, useVueFlow } from '@vue-flow/core'
import { i18n } from '../../../../core/i18n'
import '../../../../app/style.css'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import '../styles/canvas-surface-skin.css'
import '../styles/canvas-style-contracts.css'
import '../styles/canvas-vue-flow-adapter-skin.css'
import CanvasMiniMap from '../chrome/CanvasMiniMap.vue'
import { NodeStageFrame } from '../nodes/primitives'
import { ImageInpaintChrome } from '../../tools/inpaint'

vi.mock('../../session/execution', () => ({
  useCanvasBillingQuote: () => ({ value: { status: 'ready' } }),
}))

declare module 'vitest/internal/browser' {
  interface BrowserCommands {
    emulateMotion: (reducedMotion: 'reduce' | 'no-preference') => Promise<void>
  }
}

function styleOf(element: Element): CSSStyleDeclaration {
  return window.getComputedStyle(element)
}

async function setMotion(isReduced: boolean) {
  await commands.emulateMotion(isReduced ? 'reduce' : 'no-preference')
}

function mountApp(node: () => unknown) {
  const root = document.createElement('div')
  root.className = 'canvas-surface-skin'
  root.style.cssText = 'width: 1200px; height: 800px;'
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
    query: <T extends Element = HTMLElement>(selector: string) => root.querySelector<T>(selector),
    queryAll: <T extends Element = HTMLElement>(selector: string) => [
      ...root.querySelectorAll<T>(selector),
    ],
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

async function waitFor<T>(read: () => T | null, label: string): Promise<T> {
  const started = Date.now()
  for (;;) {
    const value = read()
    if (value) return value
    if (Date.now() - started > 8000) throw new Error(`${label} missing`)
    await new Promise((resolve) => setTimeout(resolve, 25))
  }
}

const ANCHOR_RECT = { top: 0, left: 0, width: 320, height: 480 }

function flowHost() {
  const nodes = ref([
    {
      id: 'node-a',
      position: { x: 50, y: 60 },
      data: {},
      style: { width: '120px', height: '60px' },
    },
    {
      id: 'node-b',
      position: { x: 400, y: 60 },
      data: {},
      style: { width: '120px', height: '60px' },
    },
  ])
  const edges = ref([{ id: 'edge-a-b', source: 'node-a', target: 'node-b' }])
  let selectEdge: (() => void) | null = null
  const SelectionDriver = defineComponent({
    setup() {
      const { addSelectedEdges } = useVueFlow()
      selectEdge = () => addSelectedEdges([{ ...edges.value[0]! }] as never)
      return () => null
    },
  })
  const host = mountApp(() =>
    h('div', { class: 'canvas-flow-wrap', style: 'width: 1000px; height: 700px;' }, [
      h(
        VueFlow,
        { nodes: nodes.value, edges: edges.value as never },
        { default: () => [h(SelectionDriver), h(CanvasMiniMap)] },
      ),
    ]),
  )
  return { edges, host, selectEdge: () => selectEdge?.() }
}

describe('Canvas surface states browser contract', () => {
  beforeEach(async () => {
    await page.viewport(1400, 800)
    document.documentElement.classList.remove('dark')
    i18n.global.locale.value = 'en'
    await setMotion(false)
  })

  afterEach(async () => {
    document.body.innerHTML = ''
    await setMotion(false)
  })

  it('keeps the chrome focus ring on the shared token and the status rail responsive', async () => {
    const chrome = mountApp(() =>
      h('div', { class: 'canvas-chrome-region' }, [
        h('div', { class: 'canvas-chrome-region--status' }),
        h('button', { class: 'canvas-chrome-board-icon-button', type: 'button' }, 'Board'),
      ]),
    )
    try {
      const button = chrome.query('button')!
      const status = chrome.query('.canvas-chrome-region--status')!
      button.focus()
      expect(button.matches(':focus-visible')).toBe(true)
      // Focus lands the shared ring paint with no outline fallback. The
      // ring rides the 160ms control transition; poll to its end state.
      expect(styleOf(button).outlineStyle).toBe('none')
      await expect.poll(() => styleOf(button).boxShadow).toContain('3px')
      const ring = styleOf(document.documentElement)
        .getPropertyValue('--color-page-chrome-focus-ring')
        .trim()
      expect(ring.length).toBeGreaterThan(0)
      // Provenance: the focus paint rides the shared token live — moving
      // it moves the ring, so a retired account-var alias cannot satisfy
      // this.
      document.documentElement.style.setProperty('--color-page-chrome-focus-ring', 'rgb(255, 0, 0)')
      try {
        await expect.poll(() => styleOf(button).boxShadow).toBe('rgb(255, 0, 0) 0px 0px 0px 3px')
      } finally {
        document.documentElement.style.removeProperty('--color-page-chrome-focus-ring')
      }
      await expect.poll(() => styleOf(button).boxShadow).toContain('3px')
      // The retired bordered-control node never renders.
      expect(chrome.query('.canvas-chrome-bordered-control')).toBeNull()
      // Responsive contract: the status rail caps at the viewport minus 32px.
      expect(styleOf(status).maxWidth).toBe('1368px')
    } finally {
      chrome.unmount()
    }
  })

  it('paints selected edges and minimap nodes from the surface contract', async () => {
    const { host, selectEdge } = flowHost()
    try {
      const path = await waitFor(() => host.query<SVGElement>('.vue-flow__edge-path'), 'edge path')
      const restStroke = styleOf(path).stroke
      expect(restStroke.length).toBeGreaterThan(0)

      // Selection rides the real VueFlow selection store (the same store a
      // pointer click drives; the zero-height edge geometry defeats pointer
      // actionability checks, so the store entry point is used directly).
      selectEdge()
      const selectedPath = await waitFor(
        () => host.query<SVGElement>('.vue-flow__edge.selected .vue-flow__edge-path'),
        'selected edge path',
      )
      const selectedToken = styleOf(document.documentElement)
        .getPropertyValue('--color-canvas-edge-selected')
        .trim()
      const selectedProbe = document.createElement('div')
      selectedProbe.style.color = 'var(--color-canvas-edge-selected)'
      host.root.append(selectedProbe)
      try {
        expect(styleOf(selectedPath).stroke).toBe(styleOf(selectedProbe).color)
        expect(selectedToken.length).toBeGreaterThan(0)
        // Provenance: moving the selected token moves the edge paint.
        host.root.ownerDocument.documentElement.style.setProperty(
          '--color-canvas-edge-selected',
          'rgb(0, 200, 0)',
        )
        try {
          expect(styleOf(selectedPath).stroke).toBe('rgb(0, 200, 0)')
        } finally {
          host.root.ownerDocument.documentElement.style.removeProperty(
            '--color-canvas-edge-selected',
          )
        }
        // Negative provenance: the adapter owns no page-chrome alias —
        // moving those tokens leaves edge paint untouched.
        withToken('--color-page-chrome-border', 'rgb(255, 0, 0)', () => {
          expect(styleOf(selectedPath).stroke).not.toBe('rgb(255, 0, 0)')
        })
        withToken('--color-page-chrome-bg', 'rgb(255, 0, 0)', () => {
          expect(styleOf(selectedPath).stroke).not.toBe('rgb(255, 0, 0)')
        })
      } finally {
        selectedProbe.remove()
      }

      // The minimap mounts the shared surface classes and paints nodes
      // from the minimap token.
      const minimap = await waitFor(
        () => host.query('.vue-flow__minimap.canvas-minimap'),
        'minimap',
      )
      expect(minimap.classList.contains('page-chrome-surface')).toBe(true)
      const minimapNode = await waitFor(
        () => host.query<SVGElement>('.vue-flow__minimap-node'),
        'minimap node',
      )
      const nodeProbe = document.createElement('div')
      nodeProbe.style.color = 'var(--canvas-minimap-node)'
      host.root.append(nodeProbe)
      try {
        expect(styleOf(minimapNode).fill).toBe(styleOf(nodeProbe).color)
      } finally {
        nodeProbe.remove()
      }
    } finally {
      host.unmount()
    }
  })

  it('keeps the node running ring, output-count states and stage-frame states', async () => {
    const nodes = mountApp(() =>
      h('div', [
        h('div', { class: 'workflow-node running', style: 'position: relative;' }),
        h('button', { class: 'node-tool-output-count active', type: 'button' }, '4'),
        h(
          'button',
          {
            class: 'node-tool-output-count',
            type: 'button',
            disabled: true,
          } satisfies import('vue').ButtonHTMLAttributes,
          '2',
        ),
        h(NodeStageFrame, { isLoading: true }, { default: () => 'stage' }),
        h(NodeStageFrame, { isConnectionDropTarget: true }, { default: () => 'target' }),
        h(NodeStageFrame, { isActive: true }, { default: () => 'active' }),
      ]),
    )
    try {
      const running = nodes.query('.workflow-node.running')!
      expect(window.getComputedStyle(running, '::before').animationName).not.toBe('none')
      await setMotion(true)
      expect(window.getComputedStyle(running, '::before').animationName).toBe('none')
      await setMotion(false)

      const activeCount = nodes.query('.node-tool-output-count.active')!
      const accentProbe = document.createElement('div')
      accentProbe.style.backgroundColor = 'var(--canvas-node-accent-soft)'
      accentProbe.style.borderColor = 'var(--node-tool-button-accent)'
      nodes.root.append(accentProbe)
      try {
        expect(styleOf(activeCount).backgroundColor).toBe(styleOf(accentProbe).backgroundColor)
        expect(styleOf(activeCount).borderColor).toBe(styleOf(accentProbe).borderColor)
      } finally {
        accentProbe.remove()
      }
      const disabledCount = nodes.query<HTMLButtonElement>('.node-tool-output-count:disabled')!
      expect(disabledCount.disabled).toBe(true)
      expect(styleOf(disabledCount).opacity).toBe('0.55')
      expect(styleOf(disabledCount).cursor).toBe('wait')

      const frames = nodes.queryAll('.node-stage-frame')
      expect(frames).toHaveLength(3)
      const [loading, dropTarget, active] = frames as HTMLElement[]
      // Loading and connection-drop-target raise the emphasis glow, which
      // resolves through the surface accent token (not a frame-local value).
      const emphasisProbe = document.createElement('div')
      emphasisProbe.style.color = 'var(--canvas-accent-shadow)'
      nodes.root.append(emphasisProbe)
      try {
        const emphasis = styleOf(emphasisProbe).color
        expect(styleOf(loading!).boxShadow).toContain(emphasis)
        expect(styleOf(dropTarget!).boxShadow).toContain(emphasis)
      } finally {
        emphasisProbe.remove()
      }
      for (const frame of frames as HTMLElement[]) {
        // The frame owns no border of its own; the ring lives on ::after.
        expect(styleOf(frame).borderTopWidth).toBe('0px')
        expect(window.getComputedStyle(frame, '::after').content).not.toBe('none')
      }
      // The active frame lights its ::after ring.
      expect(window.getComputedStyle(active!, '::after').opacity).toBe('1')
    } finally {
      nodes.unmount()
    }
  })

  it('disables the inpaint composer while generating', async () => {
    const idle = mountApp(() =>
      h(ImageInpaintChrome, {
        drawMode: 'paint',
        brushSize: 24,
        maskLayerOpacity: 60,
        anchorRect: ANCHOR_RECT,
      }),
    )
    try {
      const idleInput = idle.query<HTMLTextAreaElement>('textarea')!
      expect(idleInput.disabled).toBe(false)
    } finally {
      idle.unmount()
    }

    const generating = mountApp(() =>
      h(ImageInpaintChrome, {
        drawMode: 'paint',
        brushSize: 24,
        maskLayerOpacity: 60,
        isDirectGenerating: true,
        anchorRect: ANCHOR_RECT,
      }),
    )
    try {
      // The retired `:disabled="directGenerating"` wiring renders as a
      // genuinely disabled composer, not a decorative class.
      const busyInput = generating.query<HTMLTextAreaElement>('textarea')!
      expect(busyInput.disabled).toBe(true)
      await browserUser.click(busyInput).catch(() => {})
      expect(busyInput.disabled).toBe(true)
    } finally {
      generating.unmount()
    }
  })
})
