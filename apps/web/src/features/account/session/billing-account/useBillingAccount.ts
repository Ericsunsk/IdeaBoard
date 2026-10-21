import { toValue, type MaybeRefOrGetter } from 'vue'
import { useUserStore } from '../../../auth'
import { useAccountWorkspace } from '../accountWorkspace'
import { useBillingAccountContext } from './useBillingAccountContext'

// The global selection owns its target. Local settings callers supply theirs
// directly to the same identity-aware account context.
export function useBillingAccount(enabled: MaybeRefOrGetter<boolean> = true) {
  const userStore = useUserStore()
  const { workspaceId, isSelectionSettled } = useAccountWorkspace({ enabled })
  return useBillingAccountContext({
    workspaceId,
    identity: () => userStore.user?.public_id ?? '',
    isEnabled: () => toValue(enabled) && isSelectionSettled.value,
  })
}
