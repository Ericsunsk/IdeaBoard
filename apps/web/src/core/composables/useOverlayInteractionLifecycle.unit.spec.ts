import { effectScope, nextTick, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import {
  defineDialogSurfaceIntent,
  useOverlayInteractionLifecycle,
} from './useOverlayInteractionLifecycle'

describe('useOverlayInteractionLifecycle', () => {
  it('resets on every open and restores focus once after close completes', async () => {
    const isVisible = ref(false)
    const onReset = vi.fn()
    const onVisibilityChange = vi.fn((nextVisible: boolean) => {
      isVisible.value = nextVisible
    })
    const scope = effectScope()
    const lifecycle = scope.run(() =>
      useOverlayInteractionLifecycle({
        isVisible,
        busy: false,
        surface: {
          dismissibleBy: ['close-control', 'escape'],
          shouldRestoreFocus: true,
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
      isVisible.value = true
      await nextTick()

      expect(onReset).toHaveBeenCalledTimes(1)
      expect(lifecycle.requestClose('escape')).toBe(true)
      lifecycle.completeClose()
      lifecycle.completeClose()
      expect(firstFocus).toHaveBeenCalledTimes(1)

      secondTrigger.focus()
      const secondFocus = vi.spyOn(secondTrigger, 'focus')
      isVisible.value = true
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
    expect(intent.shouldRestoreFocus).toBe(true)
    expect(defineDialogSurfaceIntent({ shouldRestoreFocus: false })).toMatchObject({
      dismissibleBy: ['close-control', 'escape', 'mask'],
      shouldRestoreFocus: false,
    })
  })

  it('aborts a cancellable operation instead of blocking dismissal while busy', () => {
    const isVisible = ref(true)
    const busy = ref(true)
    const onAbort = vi.fn()
    const onVisibilityChange = vi.fn((nextVisible: boolean) => {
      isVisible.value = nextVisible
    })
    const scope = effectScope()
    const lifecycle = scope.run(() =>
      useOverlayInteractionLifecycle({
        isVisible,
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
      expect(isVisible.value).toBe(false)
    } finally {
      scope.stop()
    }
  })

  it('blocks every dismissal path while an irreversible commit is busy', () => {
    const isVisible = ref(true)
    const busy = ref(false)
    const onVisibilityChange = vi.fn()
    const scope = effectScope()
    const lifecycle = scope.run(() =>
      useOverlayInteractionLifecycle({
        isVisible,
        busy,
        surface: {
          dismissibleBy: ['close-control', 'escape'],
          shouldRestoreFocus: true,
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
    const isVisible = ref(false)
    const scope = effectScope()
    const lifecycle = scope.run(() =>
      useOverlayInteractionLifecycle({
        isVisible,
        surface: {
          dismissibleBy: ['close-control'],
          shouldRestoreFocus: true,
        },
        onVisibilityChange: (nextVisible) => {
          isVisible.value = nextVisible
        },
      }),
    )!
    const trigger = document.createElement('button')
    document.body.append(trigger)

    try {
      trigger.focus()
      const focus = vi.spyOn(trigger, 'focus')
      isVisible.value = true
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
    const isVisible = ref(false)
    const scope = effectScope()
    const lifecycle = scope.run(() =>
      useOverlayInteractionLifecycle({
        isVisible,
        surface: {
          dismissibleBy: ['close-control'],
          shouldRestoreFocus: true,
        },
        onVisibilityChange: (nextVisible) => {
          isVisible.value = nextVisible
        },
      }),
    )!
    const firstTrigger = document.createElement('button')
    const secondTrigger = document.createElement('button')
    document.body.append(firstTrigger, secondTrigger)

    try {
      firstTrigger.focus()
      isVisible.value = true
      isVisible.value = false

      secondTrigger.focus()
      const secondFocus = vi.spyOn(secondTrigger, 'focus')
      isVisible.value = true

      expect(lifecycle.completeClose()).toBe(false)
      expect(secondFocus).not.toHaveBeenCalled()

      isVisible.value = false
      expect(lifecycle.completeClose()).toBe(true)
      expect(secondFocus).toHaveBeenCalledTimes(1)
    } finally {
      scope.stop()
      firstTrigger.remove()
      secondTrigger.remove()
    }
  })
})
