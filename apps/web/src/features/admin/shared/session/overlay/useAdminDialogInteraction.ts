import { onScopeDispose, type MaybeRefOrGetter } from 'vue'
import {
  defineDialogSurfaceIntent,
  useOverlayInteractionLifecycle,
} from '../../../../../core/composables/useOverlayInteractionLifecycle'

type AdminDialogInteractionOptions = {
  isVisible: MaybeRefOrGetter<boolean>
  isBusy: MaybeRefOrGetter<boolean>
  onReset: () => void
  onVisibilityChange: (visible: boolean) => void
}

export function useAdminDialogInteraction(options: AdminDialogInteractionOptions) {
  const {
    ariaBusy: hasAriaBusy,
    canDismiss,
    completeClose,
    handleVisibilityChange,
    requestClose,
  } = useOverlayInteractionLifecycle({
    isVisible: options.isVisible,
    busy: options.isBusy,
    surface: defineDialogSurfaceIntent(),
    onReset: options.onReset,
    onVisibilityChange: options.onVisibilityChange,
  })

  // Defer focus restoration so reopening during close keeps focus inside.
  let restoreTimer: ReturnType<typeof setTimeout> | undefined
  function scheduleFocusRestore() {
    cancelScheduledFocusRestore()
    restoreTimer = setTimeout(() => {
      restoreTimer = undefined
      completeClose()
    }, 0)
  }

  function cancelScheduledFocusRestore() {
    if (restoreTimer !== undefined) {
      clearTimeout(restoreTimer)
      restoreTimer = undefined
    }
  }

  onScopeDispose(cancelScheduledFocusRestore)

  function handleOpenUpdate(isOpen: boolean) {
    if (isOpen) {
      cancelScheduledFocusRestore()
      handleVisibilityChange(true)
      return
    }
    if (handleVisibilityChange(false)) scheduleFocusRestore()
  }

  function handleEscape(event: KeyboardEvent) {
    if (!canDismiss('escape')) event.preventDefault()
  }

  // Mask clicks use their explicit dismissal path, independently of Reka's
  // outside-pointer detection.
  function blockOutsideDismiss(event: Event) {
    event.preventDefault()
  }

  function requestCloseFromMask() {
    requestClose('mask')
  }

  function requestCloseFromControl() {
    if (requestClose('close-control')) scheduleFocusRestore()
  }

  return {
    hasAriaBusy,
    canDismiss,
    handleOpenUpdate,
    handleEscape,
    blockOutsideDismiss,
    requestCloseFromMask,
    requestCloseFromControl,
  }
}
