import { toast } from 'vue-sonner'

type ToastLevelOptions = {
  description?: string
  duration?: number
}

type ToastLoadingOptions = {
  description?: string
  duration?: number
}

type ToastLoadingHandle = {
  id: string | number
  destroy: () => void
}

function normalizeContent(input: unknown): string {
  if (typeof input === 'string') return input
  if (input == null) return ''
  return String(input)
}

export function useAppToast() {
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
      description: options?.description,
      duration: options?.duration ?? Number.POSITIVE_INFINITY,
    })
    return {
      id,
      destroy: () => toast.dismiss(id),
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
