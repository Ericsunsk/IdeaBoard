import { ref } from 'vue'
import {
  useOverlayInteractionLifecycle,
  type OverlaySurfaceIntent,
} from './useOverlayInteractionLifecycle'
import type { MaybeRefOrGetter } from 'vue'

type PrimeDialogFocusAdapter = {
  target: HTMLElement | null
}

export function usePrimeDialogOverlayLifecycle(options: {
  visible: MaybeRefOrGetter<boolean>
  busy?: MaybeRefOrGetter<boolean>
  surface: OverlaySurfaceIntent
  onReset?: () => void
  onVisibilityChange: (visible: boolean) => void
}) {
  const dialogRef = ref<PrimeDialogFocusAdapter | null>(null)
  const lifecycle = useOverlayInteractionLifecycle(options)

  function disableVendorFocusRestore() {
    if (dialogRef.value) dialogRef.value.target = null
  }

  return {
    ...lifecycle,
    dialogRef,
    disableVendorFocusRestore,
  }
}
