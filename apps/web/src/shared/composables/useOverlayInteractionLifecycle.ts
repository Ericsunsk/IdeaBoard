import { computed, toValue, watch, type MaybeRefOrGetter } from 'vue'

export type OverlayDismissal = 'close-control' | 'escape' | 'mask' | 'outside-pointer'

export type OverlaySurfaceIntent = {
  dismissibleBy: readonly OverlayDismissal[]
  restoreFocus: boolean
}

type OverlayInteractionLifecycleOptions = {
  visible: MaybeRefOrGetter<boolean>
  busy?: MaybeRefOrGetter<boolean>
  surface: OverlaySurfaceIntent
  onReset?: () => void
  onVisibilityChange: (visible: boolean) => void
}

export function useOverlayInteractionLifecycle(options: OverlayInteractionLifecycleOptions) {
  const dismissibleBy = new Set(options.surface.dismissibleBy)
  const isBusy = computed(() => Boolean(options.busy && toValue(options.busy)))
  const ariaBusy = computed(() => (isBusy.value ? true : undefined))
  let focusReturnTarget: HTMLElement | null = null
  let closeRequested = false

  watch(
    () => toValue(options.visible),
    (visible, wasVisible) => {
      if (!visible || wasVisible) return

      closeRequested = false
      focusReturnTarget = resolveFocusReturnTarget()
      options.onReset?.()
    },
    { flush: 'sync', immediate: true },
  )

  function canDismiss(dismissal: OverlayDismissal) {
    return !isBusy.value && dismissibleBy.has(dismissal)
  }

  function requestClose(dismissal: OverlayDismissal) {
    if (!toValue(options.visible) || closeRequested || !canDismiss(dismissal)) return false

    closeRequested = true
    options.onVisibilityChange(false)
    return true
  }

  function handleVisibilityChange(visible: boolean) {
    if (visible) {
      if (toValue(options.visible)) return false
      options.onVisibilityChange(true)
      return true
    }

    return requestClose('close-control')
  }

  function completeClose() {
    if (toValue(options.visible)) return false

    closeRequested = false
    const target = focusReturnTarget
    focusReturnTarget = null

    if (!options.surface.restoreFocus || !target?.isConnected) return false
    target.focus()
    return true
  }

  return {
    ariaBusy,
    canDismiss,
    completeClose,
    handleVisibilityChange,
    isBusy,
    requestClose,
  }
}

function resolveFocusReturnTarget() {
  const activeElement = document.activeElement
  return activeElement instanceof HTMLElement && activeElement !== document.body
    ? activeElement
    : null
}
