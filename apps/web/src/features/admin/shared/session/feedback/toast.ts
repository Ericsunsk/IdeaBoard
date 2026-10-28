import { useAppToast } from '@/core/composables/useAppToast'
import { extractSafeApiErrorMessage, formatDisplayErrorMessage } from '@/core/utils/errorMessages'

export type AdminToastPort = Pick<
  ReturnType<typeof useAppToast>,
  'success' | 'warning' | 'error' | 'info'
>

export function formatAdminApiError(error: unknown, fallback: string) {
  return extractSafeApiErrorMessage(error, fallback)
}

export function formatAdminDisplayError(error: unknown, fallback: string) {
  return formatDisplayErrorMessage(error, fallback)
}

export function useAdminToast(): AdminToastPort {
  return useAppToast()
}
