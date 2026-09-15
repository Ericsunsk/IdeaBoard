import { toast, type ExternalToast } from 'vue-sonner'

type ToastLevelOptions = Pick<
  ExternalToast,
  'action' | 'description' | 'duration' | 'id' | 'onAutoClose' | 'onDismiss'
>

type ToastLoadingOptions = {
  description?: string
  duration?: number
}

type ToastLoadingHandle = {
  id: string | number
  destroy: () => void
  update: (content: unknown, options?: ToastLoadingOptions) => void
}

function normalizeContent(input: unknown): string {
  if (typeof input === 'string') return input
  if (input == null) return ''
  return String(input)
}

let clickToDismissInstalled = false

/**
 * Click-anywhere-to-dismiss for sonner toasts. vue-sonner only dismisses via
 * its close button, and the toast DOM carries no id to map back, so a single
 * delegated listener forwards surface clicks to the toast's own close button
 * (precise per-toast dismiss with exit animation). Clicks on interactive
 * controls (`action`/`cancel` buttons, links, close button itself) keep native
 * behavior — e.g. the recharge action still runs its own onClick + dismiss.
 */
function installClickToDismiss() {
  if (clickToDismissInstalled || typeof document === 'undefined') return
  clickToDismissInstalled = true
  document.addEventListener('click', (event) => {
    const target = event.target as Element | null
    const toastEl = target?.closest?.('[data-sonner-toast]')
    if (!toastEl) return
    // Interactive descendants own their clicks; sonner dismisses after actions itself.
    if (target?.closest?.('button, a, [data-button], [data-close-button]')) return
    toastEl.querySelector<HTMLButtonElement>('[data-close-button]')?.click()
  })
}

export function useAppToast() {
  installClickToDismiss()

  function success(content: unknown, options?: ToastLevelOptions) {
    return toast.success(normalizeContent(content), options)
  }

  function error(content: unknown, options?: ToastLevelOptions) {
    return toast.error(normalizeContent(content), options)
  }

  function warning(content: unknown, options?: ToastLevelOptions) {
    return toast.warning(normalizeContent(content), options)
  }

  function info(content: unknown, options?: ToastLevelOptions) {
    return toast.info(normalizeContent(content), options)
  }

  function loading(content: unknown, options?: ToastLoadingOptions): ToastLoadingHandle {
    const id = toast.loading(normalizeContent(content), {
      ...(options?.description === undefined ? {} : { description: options.description }),
      duration: options?.duration ?? Number.POSITIVE_INFINITY,
    })
    return {
      id,
      destroy: () => toast.dismiss(id),
      update: (nextContent, nextOptions) =>
        toast.loading(normalizeContent(nextContent), {
          id,
          ...(nextOptions?.description === undefined
            ? {}
            : { description: nextOptions.description }),
          duration: nextOptions?.duration ?? Number.POSITIVE_INFINITY,
        }),
    }
  }

  function dismiss(id?: string | number) {
    toast.dismiss(id)
  }

  return {
    success,
    error,
    warning,
    info,
    loading,
    dismiss,
  }
}

export function dismissAppToasts() {
  toast.dismiss()
}
