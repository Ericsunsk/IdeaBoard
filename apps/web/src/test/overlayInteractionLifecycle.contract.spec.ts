import { effectScope, nextTick, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useCanvasCustomLayerOverlay } from '../features/canvas/presentation/overlays'
import {
  useOverlayInteractionLifecycle,
  type OverlayDismissal,
} from '../shared/composables/useOverlayInteractionLifecycle'
import { usePrimeDialogOverlayLifecycle } from '../shared/composables/usePrimeDialogOverlayLifecycle'
import { usePrimePopupMenuOverlayLifecycle } from '../shared/composables/usePrimePopupMenuOverlayLifecycle'

describe('overlay interaction lifecycle contract', () => {
  it('blocks shared lifecycle dismissal while busy and resets every reopen', () => {
    const visible = ref(false)
    const busy = ref(false)
    const onReset = vi.fn()
    const scope = effectScope()
    const lifecycle = scope.run(() =>
      useOverlayInteractionLifecycle({
        visible,
        busy,
        surface: {
          dismissibleBy: ['escape'],
          restoreFocus: false,
        },
        onReset,
        onVisibilityChange: (nextVisible) => {
          visible.value = nextVisible
        },
      }),
    )!

    try {
      visible.value = true
      expect(onReset).toHaveBeenCalledOnce()

      busy.value = true
      expect(lifecycle.requestClose('escape')).toBe(false)
      expect(visible.value).toBe(true)

      busy.value = false
      expect(lifecycle.requestClose('escape')).toBe(true)
      expect(visible.value).toBe(false)

      visible.value = true
      expect(onReset).toHaveBeenCalledTimes(2)
      expect(lifecycle.requestClose('escape')).toBe(true)
    } finally {
      scope.stop()
    }
  })

  it('restores trigger focus through the Prime Dialog adapter after close completes', () => {
    const visible = ref(false)
    const trigger = document.createElement('button')
    document.body.appendChild(trigger)
    trigger.focus()
    const focus = vi.spyOn(trigger, 'focus')
    const scope = effectScope()
    const dialog = scope.run(() =>
      usePrimeDialogOverlayLifecycle({
        visible,
        surface: {
          dismissibleBy: ['close-control'],
          restoreFocus: true,
        },
        onVisibilityChange: (nextVisible) => {
          visible.value = nextVisible
        },
      }),
    )!

    try {
      visible.value = true
      expect(dialog.requestClose('close-control')).toBe(true)
      expect(dialog.completeClose()).toBe(true)
      expect(focus).toHaveBeenCalledOnce()
    } finally {
      scope.stop()
      trigger.remove()
    }
  })

  it('routes Prime Menu outside-click and Escape through one lifecycle and tears listeners down', async () => {
    const trigger = document.createElement('button')
    const container = document.createElement('div')
    document.body.append(trigger, container)
    const hide = vi.fn()
    const scope = effectScope()
    const menu = scope.run(() => usePrimePopupMenuOverlayLifecycle({}))!
    menu.popupRef.value = {
      container,
      target: trigger,
      hide,
      toggle: vi.fn(),
    }

    try {
      trigger.focus()
      menu.toggle(new Event('click'), trigger)
      await nextTick()
      document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0 }))
      await nextTick()

      expect(hide).toHaveBeenCalledOnce()
      expect(document.activeElement).toBe(trigger)

      menu.toggle(new Event('click'), trigger)
      await nextTick()
      document.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, key: 'Escape' }))
      await nextTick()

      expect(hide).toHaveBeenCalledTimes(2)

      menu.toggle(new Event('click'), trigger)
      await nextTick()
      scope.stop()
      document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0 }))
      document.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, key: 'Escape' }))

      expect(hide).toHaveBeenCalledTimes(2)
    } finally {
      scope.stop()
      trigger.remove()
      container.remove()
    }
  })

  it('routes Canvas outside-click and Escape through its public adapter and tears listeners down', async () => {
    const visible = ref(false)
    const rootElement = document.createElement('div')
    const root = ref<HTMLElement | null>(rootElement)
    const onDismiss = vi.fn(
      (_dismissal: OverlayDismissal, _event: KeyboardEvent | PointerEvent) => {
        visible.value = false
      },
    )
    document.body.appendChild(rootElement)
    const scope = effectScope()
    scope.run(() =>
      useCanvasCustomLayerOverlay({
        visible,
        roots: [root],
        onDismiss,
      }),
    )

    try {
      visible.value = true
      await nextTick()
      document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0 }))
      expect(onDismiss).toHaveBeenLastCalledWith('outside-pointer', expect.any(PointerEvent))

      visible.value = true
      await nextTick()
      document.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, key: 'Escape' }))
      expect(onDismiss).toHaveBeenLastCalledWith('escape', expect.any(KeyboardEvent))

      visible.value = true
      await nextTick()
      scope.stop()
      document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0 }))
      document.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, key: 'Escape' }))

      expect(onDismiss).toHaveBeenCalledTimes(2)
    } finally {
      scope.stop()
      root.value?.remove()
    }
  })
})
