import { nextTick, ref, watchEffect } from 'vue'
import { useOverlayInteractionLifecycle } from './useOverlayInteractionLifecycle'

export type PrimePopupMenuOverlayRef = {
  container?: HTMLElement | null
  hide: () => void
  target?: HTMLElement | null
  toggle: (event: Event, target?: HTMLElement) => void
}

export function usePrimePopupMenuOverlayLifecycle(options: {
  onVisibilityChange?: (visible: boolean) => void
}) {
  const popupRef = ref<PrimePopupMenuOverlayRef | null>(null)
  const isOpen = ref(false)
  let skipFocusRestoreOnce = false
  const lifecycle = useOverlayInteractionLifecycle({
    visible: isOpen,
    surface: {
      dismissibleBy: ['close-control', 'escape', 'outside-pointer'],
      restoreFocus: true,
    },
    onVisibilityChange: (visible) => {
      if (!visible) {
        popupRef.value?.hide()
        settleClosed()
      }
    },
  })

  function toggle(event: Event, target?: HTMLElement) {
    if (isOpen.value) {
      lifecycle.requestClose('close-control')
      return
    }

    isOpen.value = true
    popupRef.value?.toggle(event, target)
  }

  function handleShow() {
    isOpen.value = true
    options.onVisibilityChange?.(true)
  }

  function handleHide() {
    settleClosed()
  }

  function settleClosed() {
    if (!isOpen.value) return
    isOpen.value = false
    options.onVisibilityChange?.(false)
    void nextTick(() => {
      if (skipFocusRestoreOnce) {
        skipFocusRestoreOnce = false
        return
      }
      lifecycle.completeClose()
    })
  }

  function handleKeydownCapture(event: KeyboardEvent) {
    if (event.key !== 'Escape' || !lifecycle.canDismiss('escape')) return
    event.stopImmediatePropagation()
    event.preventDefault()
    lifecycle.requestClose('escape')
  }

  function handlePointerDownCapture(event: PointerEvent) {
    const target = event.target
    if (!(target instanceof Node)) return
    const popup = popupRef.value
    if (popup?.container?.contains(target) || popup?.target?.contains(target)) return
    lifecycle.requestClose('outside-pointer')
  }

  function select(command: () => void) {
    skipFocusRestoreOnce = true
    if (!lifecycle.requestClose('close-control')) skipFocusRestoreOnce = false
    command()
  }

  watchEffect((onCleanup) => {
    if (!isOpen.value) return
    document.addEventListener('keydown', handleKeydownCapture, true)
    document.addEventListener('pointerdown', handlePointerDownCapture, true)
    onCleanup(() => {
      document.removeEventListener('keydown', handleKeydownCapture, true)
      document.removeEventListener('pointerdown', handlePointerDownCapture, true)
    })
  })

  return {
    ...lifecycle,
    popupRef,
    isOpen,
    toggle,
    handleShow,
    handleHide,
    handleKeydownCapture,
    handlePointerDownCapture,
    select,
  }
}
