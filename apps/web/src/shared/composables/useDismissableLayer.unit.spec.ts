import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useDismissableLayer } from './useDismissableLayer'

function mountDismissableLayer(options: { enabled?: boolean; ignore?: ReadonlyArray<string> } = {}) {
  const enabled = ref(options.enabled ?? true)
  const onDismiss = vi.fn()
  const host = document.createElement('div')
  document.body.appendChild(host)

  const app = createApp(
    defineComponent({
      setup() {
        useDismissableLayer({
          enabled,
          ignore: options.ignore,
          onDismiss,
          ownerDocument: () => document,
        })
        return () => h('div')
      },
    }),
  )
  app.mount(host)

  return {
    enabled,
    onDismiss,
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

function dispatchEscape() {
  document.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, key: 'Escape' }))
}

describe('useDismissableLayer', () => {
  it('dismisses on pointerdown and Escape while enabled', async () => {
    const harness = mountDismissableLayer()
    await nextTick()

    dispatchPointerDown()
    dispatchEscape()

    expect(harness.onDismiss).toHaveBeenCalledTimes(2)

    await harness.unmount()
  })

  it('does not dismiss from ignored layers', async () => {
    const harness = mountDismissableLayer({ ignore: ['[data-ignored-layer]'] })
    const ignoredLayer = document.createElement('div')
    ignoredLayer.dataset.ignoredLayer = ''
    document.body.appendChild(ignoredLayer)
    await nextTick()

    try {
      dispatchPointerDown(ignoredLayer)

      expect(harness.onDismiss).not.toHaveBeenCalled()
    } finally {
      ignoredLayer.remove()
      await harness.unmount()
    }
  })

  it('removes listeners when disabled and on scope dispose', async () => {
    const harness = mountDismissableLayer()
    await nextTick()

    harness.enabled.value = false
    await nextTick()
    dispatchPointerDown()

    await harness.unmount()
    dispatchEscape()

    expect(harness.onDismiss).not.toHaveBeenCalled()
  })
})
