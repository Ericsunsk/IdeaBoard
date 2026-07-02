import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageEditorCurvesParams } from '../types'
import ImageEditorCurvesControl from './ImageEditorCurvesControl.vue'

function mountCurvesControl(initialParams: ImageEditorCurvesParams) {
  const params = ref(initialParams)
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        return () =>
          h(ImageEditorCurvesControl, {
            params: params.value,
            'onUpdate:params': (nextParams: ImageEditorCurvesParams) => {
              params.value = nextParams
            },
          })
      },
    }),
  )
  app.mount(host)

  return {
    app,
    host,
    params,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

function dispatchDelete(target: Element | null) {
  target?.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'Delete', bubbles: true, cancelable: true }),
  )
}

describe('ImageEditorCurvesControl', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps keyboard focus on the next curve point after deleting the selected point', async () => {
    const harness = mountCurvesControl({
      channels: {
        rgb: [
          { id: 'shadow', input: 25, output: 20 },
          { id: 'midtone', input: 50, output: 55 },
          { id: 'highlight', input: 75, output: 82 },
        ],
      },
    })

    try {
      await nextTick()
      const bubbledKeydown = vi.fn()
      harness.host.addEventListener('keydown', bubbledKeydown)

      const midtonePoint = harness.host.querySelector<HTMLButtonElement>(
        '[data-point-id="midtone"]',
      )
      midtonePoint?.focus()
      await nextTick()
      dispatchDelete(midtonePoint)
      await nextTick()
      await nextTick()

      const highlightPoint = harness.host.querySelector<HTMLButtonElement>(
        '[data-point-id="highlight"]',
      )
      expect(document.activeElement).toBe(highlightPoint)
      expect(harness.params.value.channels.rgb?.map((point) => point.id)).toEqual([
        'shadow',
        'highlight',
      ])

      dispatchDelete(document.activeElement)
      await nextTick()
      await nextTick()

      expect(bubbledKeydown).not.toHaveBeenCalled()
      expect(harness.params.value.channels.rgb?.map((point) => point.id)).toEqual(['shadow'])
    } finally {
      harness.unmount()
    }
  })

  it('ignores cleared selected point number inputs instead of moving the point to zero', async () => {
    const harness = mountCurvesControl({
      channels: {
        rgb: [{ id: 'midtone', input: 50, output: 55 }],
      },
    })

    try {
      await nextTick()
      harness.host.querySelector<HTMLButtonElement>('[data-point-id="midtone"]')?.click()
      await nextTick()

      const [inputField, outputField] =
        harness.host.querySelectorAll<HTMLInputElement>('input[type="number"]')

      expect(inputField?.classList.contains('editing-controls-number-input')).toBe(true)
      expect(outputField?.classList.contains('editing-controls-number-input')).toBe(true)

      inputField.value = ''
      inputField.dispatchEvent(new Event('input', { bubbles: true }))
      await nextTick()

      outputField.value = ''
      outputField.dispatchEvent(new Event('input', { bubbles: true }))
      await nextTick()

      expect(harness.params.value.channels.rgb).toEqual([{ id: 'midtone', input: 50, output: 55 }])
    } finally {
      harness.unmount()
    }
  })
})
