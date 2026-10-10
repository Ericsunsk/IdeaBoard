import { computed, toValue, watch, type MaybeRefOrGetter } from 'vue'

export type OverlayDismissal = 'close-control' | 'escape' | 'mask' | 'outside-pointer'

export type OverlaySurfaceIntent = {
  dismissibleBy: readonly OverlayDismissal[]
  restoreFocus: boolean
}

/**
 * Unified dialog dismissal contract: every modal dialog is dismissible by
 * close-control, Escape, and mask while idle. Only irreversible commits
 * (busyMode 'block', the default) lock all dismissal paths while busy;
 * cancellable operations opt into busyMode 'abort' and expose abort through
 * onAbort (wired to an AbortController where the operation supports one).
 */
export const defaultDialogDismissals = ['close-control', 'escape', 'mask'] as const

export function defineDialogSurfaceIntent(
  overrides: Partial<OverlaySurfaceIntent> = {},
): OverlaySurfaceIntent {
  return {
    dismissibleBy: overrides.dismissibleBy ?? [...defaultDialogDismissals],
    restoreFocus: overrides.restoreFocus ?? true,
  }
}

export type OverlayBusyMode = 'block' | 'abort'

type OverlayInteractionLifecycleOptions = {
  visible: MaybeRefOrGetter<boolean>
  busy?: MaybeRefOrGetter<boolean>
  /**
   * 'block' (default): irreversible commits lock every dismissal path while busy.
   * 'abort' (cancellable): dismissal stays available while busy; the close
   * first runs onAbort so the pending operation can settle, then closes.
   */
  busyMode?: OverlayBusyMode
  onAbort?: () => void
  surface: OverlaySurfaceIntent
  onReset?: () => void
  onVisibilityChange: (visible: boolean) => void
}

export function useOverlayInteractionLifecycle(options: OverlayInteractionLifecycleOptions) {
  const dismissibleBy = new Set(options.surface.dismissibleBy)
  const busyMode = options.busyMode ?? 'block'
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
    if (!dismissibleBy.has(dismissal)) return false
    // Irreversible commits lock every path while busy; cancellable operations
    // stay dismissible and abort through requestClose instead.
    if (isBusy.value && busyMode === 'block') return false
    return true
  }

  function requestClose(dismissal: OverlayDismissal) {
    if (!toValue(options.visible) || closeRequested || !canDismiss(dismissal)) return false

    if (isBusy.value && busyMode === 'abort') options.onAbort?.()
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
