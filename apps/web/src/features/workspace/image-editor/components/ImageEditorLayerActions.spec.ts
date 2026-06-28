import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ImageEditorLayerActions from './ImageEditorLayerActions.vue'
import type { ImageAdjustmentFinalApplyProgress } from '../runtime/imageAdjustmentFinalApplyContext'

function mountLayerActions(options: {
  applying?: boolean
  applyProgress?: ImageAdjustmentFinalApplyProgress | null
  onCancelApply?: () => void
} = {}) {
  const applying = ref(options.applying ?? false)
  const applyProgress = ref<ImageAdjustmentFinalApplyProgress | null>(
    options.applyProgress ?? null,
  )
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        return () =>
          h(ImageEditorLayerActions, {
            canCopyLayer: true,
            canMergeLayer: true,
            canRemoveLayer: true,
            applying: applying.value,
            applyDisabled: applying.value,
            applyProgress: applyProgress.value,
            onCancelApply: options.onCancelApply,
          })
      },
    }),
  )
  app.mount(host)

  return {
    applying,
    applyProgress,
    host,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

describe('ImageEditorLayerActions', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows final apply progress after a short disclosure delay and emits cancel', async () => {
    const onCancelApply = vi.fn()
    const harness = mountLayerActions({
      applying: true,
      applyProgress: { phase: 'render', loaded: 25, total: 100 },
      onCancelApply,
    })

    try {
      expect(harness.host.querySelector('[role="status"]')).toBeNull()

      vi.advanceTimersByTime(199)
      await nextTick()
      expect(harness.host.querySelector('[role="status"]')).toBeNull()

      vi.advanceTimersByTime(1)
      await nextTick()

      expect(harness.host.querySelector('[role="status"]')?.textContent).toContain('渲染调整')
      expect(harness.host.querySelector('[role="status"]')?.textContent).toContain('25%')
      expect(
        harness.host.querySelector<HTMLElement>('.image-editor-layer-actions__progress-bar')
          ?.style.width,
      ).toBe('25%')

      harness.host.querySelector<HTMLButtonElement>('[aria-label="取消应用"]')?.click()
      await nextTick()

      expect(onCancelApply).toHaveBeenCalledTimes(1)
    } finally {
      harness.unmount()
    }
  })

  it('does not flash final apply progress for quick apply tasks', async () => {
    const harness = mountLayerActions({
      applying: true,
      applyProgress: { phase: 'render', loaded: 25, total: 100 },
    })

    try {
      vi.advanceTimersByTime(199)
      await nextTick()
      harness.applying.value = false
      await nextTick()
      vi.advanceTimersByTime(1)
      await nextTick()

      expect(harness.host.querySelector('[role="status"]')).toBeNull()
    } finally {
      harness.unmount()
    }
  })

  it('hides final apply progress when idle', () => {
    const harness = mountLayerActions()

    try {
      expect(harness.host.querySelector('[role="status"]')).toBeNull()
      expect(harness.host.querySelector('[aria-label="取消应用"]')).toBeNull()
    } finally {
      harness.unmount()
    }
  })
})
