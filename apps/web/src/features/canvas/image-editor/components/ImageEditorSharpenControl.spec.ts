import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { ImageEditorSharpenParams } from '../types'
import ImageEditorSharpenControl from './ImageEditorSharpenControl.vue'

function mountSharpenControl(initialParams: ImageEditorSharpenParams) {
  const params = ref(initialParams)
  const events = {
    updateParams: vi.fn((nextParams: ImageEditorSharpenParams) => {
      params.value = nextParams
    }),
  }
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        return () =>
          h(ImageEditorSharpenControl, {
            params: params.value,
            'onUpdate:params': events.updateParams,
          })
      },
    }),
  )
  app.mount(host)

  return {
    app,
    events,
    host,
    params,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

describe('ImageEditorSharpenControl', () => {
  it('updates sharpen amount, decimal radius, threshold, and color protection', async () => {
    const harness = mountSharpenControl({
      amount: 0,
      radius: 1,
      threshold: 4,
      protectColor: true,
    })

    try {
      const amountRange = harness.host.querySelector<HTMLInputElement>('input[aria-label="数量滑块"]')
      amountRange!.value = '120'
      amountRange?.dispatchEvent(new Event('input', { bubbles: true }))

      expect(harness.events.updateParams).toHaveBeenCalledWith({
        amount: 120,
        radius: 1,
        threshold: 4,
        protectColor: true,
      })

      await nextTick()
      const radiusInput = harness.host.querySelector<HTMLInputElement>('input[aria-label="半径数值"]')
      expect(radiusInput?.step).toBe('0.1')
      expect(radiusInput?.getAttribute('inputmode')).toBe('decimal')
      radiusInput!.value = '1.7'
      radiusInput?.dispatchEvent(new Event('input', { bubbles: true }))

      expect(harness.events.updateParams).toHaveBeenCalledWith({
        amount: 120,
        radius: 1.7,
        threshold: 4,
        protectColor: true,
      })

      await nextTick()
      const thresholdInput = harness.host.querySelector<HTMLInputElement>(
        'input[aria-label="阈值数值"]',
      )
      thresholdInput!.value = '60'
      thresholdInput?.dispatchEvent(new Event('input', { bubbles: true }))

      expect(harness.events.updateParams).toHaveBeenCalledWith({
        amount: 120,
        radius: 1.7,
        threshold: 50,
        protectColor: true,
      })

      await nextTick()
      const protectColorButton = harness.host.querySelector<HTMLButtonElement>(
        'button[aria-label="保护锐化颜色"]',
      )
      expect(protectColorButton?.getAttribute('aria-pressed')).toBe('true')
      protectColorButton?.click()

      expect(harness.events.updateParams).toHaveBeenCalledWith({
        amount: 120,
        radius: 1.7,
        threshold: 50,
        protectColor: false,
      })
    } finally {
      harness.unmount()
    }
  })
})
