import { effectScope, nextTick, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import {
  defineDialogSurfaceIntent,
  useOverlayInteractionLifecycle,
} from './useOverlayInteractionLifecycle'

describe('useOverlayInteractionLifecycle', () => {
  it('resets on every open and restores focus once after close completes', async () => {
    const visible = ref(false)
    const onReset = vi.fn()
    const onVisibilityChange = vi.fn((nextVisible: boolean) => {
      visible.value = nextVisible
    })
    const scope = effectScope()
    const lifecycle = scope.run(() =>
      useOverlayInteractionLifecycle({
        visible,
        busy: false,
        surface: {
          dismissibleBy: ['close-control', 'escape'],
          restoreFocus: true,
        },
        onReset,
        onVisibilityChange,
      }),
    )!
    const firstTrigger = document.createElement('button')
    const secondTrigger = document.createElement('button')
    document.body.append(firstTrigger, secondTrigger)

    try {
      firstTrigger.focus()
      const firstFocus = vi.spyOn(firstTrigger, 'focus')
      visible.value = true
      await nextTick()

      expect(onReset).toHaveBeenCalledTimes(1)
      expect(lifecycle.requestClose('escape')).toBe(true)
      lifecycle.completeClose()
      lifecycle.completeClose()
      expect(firstFocus).toHaveBeenCalledTimes(1)

      secondTrigger.focus()
      const secondFocus = vi.spyOn(secondTrigger, 'focus')
      visible.value = true
      await nextTick()

      expect(onReset).toHaveBeenCalledTimes(2)
      expect(lifecycle.requestClose('close-control')).toBe(true)
      lifecycle.completeClose()
      expect(secondFocus).toHaveBeenCalledTimes(1)
    } finally {
      scope.stop()
      firstTrigger.remove()
      secondTrigger.remove()
    }
  })

  it('shares mask dismissal across dialog surfaces by default', () => {
    const intent = defineDialogSurfaceIntent()

    expect(intent.dismissibleBy).toEqual(['close-control', 'escape', 'mask'])
    expect(intent.restoreFocus).toBe(true)
    expect(defineDialogSurfaceIntent({ restoreFocus: false })).toMatchObject({
      dismissibleBy: ['close-control', 'escape', 'mask'],
      restoreFocus: false,
    })
  })

  it('aborts a cancellable operation instead of blocking dismissal while busy', () => {
    const visible = ref(true)
    const busy = ref(true)
    const onAbort = vi.fn()
    const onVisibilityChange = vi.fn((nextVisible: boolean) => {
      visible.value = nextVisible
    })
    const scope = effectScope()
    const lifecycle = scope.run(() =>
      useOverlayInteractionLifecycle({
        visible,
        busy,
        busyMode: 'abort',
        onAbort,
        surface: defineDialogSurfaceIntent(),
        onVisibilityChange,
      }),
    )!

    try {
      expect(lifecycle.canDismiss('mask')).toBe(true)
      expect(lifecycle.canDismiss('escape')).toBe(true)
      expect(lifecycle.requestClose('mask')).toBe(true)
      expect(onAbort).toHaveBeenCalledTimes(1)
      expect(visible.value).toBe(false)
    } finally {
      scope.stop()
    }
  })

  it('blocks every dismissal path while an irreversible commit is busy', () => {
    const visible = ref(true)
    const busy = ref(false)
    const onVisibilityChange = vi.fn()
    const scope = effectScope()
    const lifecycle = scope.run(() =>
      useOverlayInteractionLifecycle({
        visible,
        busy,
        surface: {
          dismissibleBy: ['close-control', 'escape'],
          restoreFocus: true,
        },
        onVisibilityChange,
      }),
    )!

    try {
      expect(lifecycle.canDismiss('escape')).toBe(true)
      expect(lifecycle.canDismiss('mask')).toBe(false)
      expect(lifecycle.requestClose('mask')).toBe(false)

      busy.value = true

      expect(lifecycle.ariaBusy.value).toBe(true)
      expect(lifecycle.canDismiss('escape')).toBe(false)
      expect(lifecycle.canDismiss('close-control')).toBe(false)
      expect(lifecycle.requestClose('escape')).toBe(false)
      expect(onVisibilityChange).not.toHaveBeenCalled()
    } finally {
      scope.stop()
    }
  })

  it('does not restore focus to a trigger removed before the overlay closes', () => {
    const visible = ref(false)
    const scope = effectScope()
    const lifecycle = scope.run(() =>
      useOverlayInteractionLifecycle({
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
    const trigger = document.createElement('button')
    document.body.append(trigger)

    try {
      trigger.focus()
      const focus = vi.spyOn(trigger, 'focus')
      visible.value = true
      trigger.remove()
      lifecycle.requestClose('close-control')
      lifecycle.completeClose()

      expect(focus).not.toHaveBeenCalled()
    } finally {
      scope.stop()
      trigger.remove()
    }
  })

  it('ignores a stale close completion after the overlay has reopened', () => {
    const visible = ref(false)
    const scope = effectScope()
    const lifecycle = scope.run(() =>
      useOverlayInteractionLifecycle({
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
    const firstTrigger = document.createElement('button')
    const secondTrigger = document.createElement('button')
    document.body.append(firstTrigger, secondTrigger)

    try {
      firstTrigger.focus()
      visible.value = true
      visible.value = false

      secondTrigger.focus()
      const secondFocus = vi.spyOn(secondTrigger, 'focus')
      visible.value = true

      expect(lifecycle.completeClose()).toBe(false)
      expect(secondFocus).not.toHaveBeenCalled()

      visible.value = false
      expect(lifecycle.completeClose()).toBe(true)
      expect(secondFocus).toHaveBeenCalledTimes(1)
    } finally {
      scope.stop()
      firstTrigger.remove()
      secondTrigger.remove()
    }
  })
})
