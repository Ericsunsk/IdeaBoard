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
})
