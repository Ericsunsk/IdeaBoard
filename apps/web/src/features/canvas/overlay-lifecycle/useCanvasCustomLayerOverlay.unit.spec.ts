import { createApp, defineComponent, h, nextTick, ref, type Ref } from 'vue'
import { describe, expect, it, vi, type Mock } from 'vitest'
import { useCanvasCustomLayerOverlay } from './useCanvasCustomLayerOverlay'
import type { OverlayDismissal } from '../../../shared/composables/useOverlayInteractionLifecycle'

type LayerHarness = {
  visible: Ref<boolean>
  root: Ref<HTMLElement | null>
  onDismiss: Mock<(dismissal: OverlayDismissal, event: KeyboardEvent | PointerEvent) => void>
}

function mountLayers(
  count: number,
  options: {
    ignore?: readonly string[]
    renderRoots?: boolean
    shouldDismiss?: (dismissal: OverlayDismissal, event: KeyboardEvent | PointerEvent) => boolean
  } = {},
) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const layers: LayerHarness[] = Array.from({ length: count }, () => ({
    visible: ref(false),
    root: ref<HTMLElement | null>(null),
    onDismiss: vi.fn(),
  }))

  const app = createApp(
    defineComponent({
      setup() {
        for (const layer of layers) {
          useCanvasCustomLayerOverlay({
            visible: layer.visible,
            roots: [layer.root],
            ignore: options.ignore,
            shouldDismiss: options.shouldDismiss,
            onDismiss: (dismissal, event) => {
              layer.onDismiss(dismissal, event)
              layer.visible.value = false
            },
          })
        }
        return () =>
          h(
            'div',
            options.renderRoots === false
              ? []
              : layers.map((layer, index) =>
                  h('div', { ref: layer.root, 'data-layer-root': String(index) }),
                ),
          )
      },
    }),
  )
  app.mount(host)

  return {
    host,
    layers,
    async unmount() {
      app.unmount()
      host.remove()
      await nextTick()
    },
  }
}

function dispatchPointerDown(target: EventTarget = document.body) {
  target.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0 }))
}

function dispatchEscape(target: EventTarget = document) {
  target.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, key: 'Escape' }))
}

describe('useCanvasCustomLayerOverlay', () => {
  it('keeps roots and ignored targets open, then dismisses by outside pointer or Escape', async () => {
    const ignored = document.createElement('div')
    ignored.className = 'canvas-node-context-menu'
    document.body.appendChild(ignored)
    const harness = mountLayers(1, { ignore: ['.canvas-node-context-menu'] })
    const layer = harness.layers[0]!

    try {
      layer.visible.value = true
      await nextTick()

      dispatchPointerDown(layer.root.value ?? undefined)
      dispatchPointerDown(ignored)
      expect(layer.onDismiss).not.toHaveBeenCalled()

      dispatchPointerDown()
      expect(layer.onDismiss).toHaveBeenCalledOnce()
      expect(layer.onDismiss.mock.calls[0]?.[0]).toBe('outside-pointer')

      layer.visible.value = true
      await nextTick()
      dispatchEscape()
      expect(layer.onDismiss).toHaveBeenCalledTimes(2)
      expect(layer.onDismiss.mock.calls[1]?.[0]).toBe('escape')
    } finally {
      ignored.remove()
      await harness.unmount()
    }
  })

  it('dismisses only the most recently opened layer', async () => {
    const harness = mountLayers(2)
    const lower = harness.layers[0]!
    const upper = harness.layers[1]!

    try {
      lower.visible.value = true
      await nextTick()
      upper.visible.value = true
      await nextTick()

      dispatchPointerDown()
      expect(upper.onDismiss).toHaveBeenCalledOnce()
      expect(lower.onDismiss).not.toHaveBeenCalled()

      dispatchEscape()
      expect(lower.onDismiss).toHaveBeenCalledOnce()
    } finally {
      await harness.unmount()
    }
  })

  it('preserves open order when an earlier layer attaches its root later', async () => {
    const harness = mountLayers(2, { renderRoots: false })
    const lower = harness.layers[0]!
    const upper = harness.layers[1]!

    try {
      lower.visible.value = true
      await nextTick()
      upper.visible.value = true
      await nextTick()

      lower.root.value = document.createElement('div')
      await nextTick()

      dispatchPointerDown()

      expect(upper.onDismiss).toHaveBeenCalledOnce()
      expect(lower.onDismiss).not.toHaveBeenCalled()
    } finally {
      await harness.unmount()
    }
  })

  it('removes document listeners when its scope is disposed', async () => {
    const harness = mountLayers(1)
    const layer = harness.layers[0]!
    layer.visible.value = true
    await nextTick()
    await harness.unmount()

    dispatchPointerDown()
    dispatchEscape()

    expect(layer.onDismiss).not.toHaveBeenCalled()
  })

  it('lets a Surface preserve pointer-button dismissal semantics', async () => {
    const harness = mountLayers(1, {
      shouldDismiss: (dismissal, event) =>
        dismissal !== 'outside-pointer' || !(event instanceof PointerEvent) || event.button === 0,
    })
    const layer = harness.layers[0]!

    try {
      layer.visible.value = true
      await nextTick()

      document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 2 }))

      expect(layer.onDismiss).not.toHaveBeenCalled()
      expect(layer.visible.value).toBe(true)
    } finally {
      await harness.unmount()
    }
  })
})
