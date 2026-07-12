import { useAdminFeedback } from '../feedback/useAdminFeedback'

export function useCreditsBillingFeedback() {
  return useAdminFeedback().creditsBilling.operations
}
