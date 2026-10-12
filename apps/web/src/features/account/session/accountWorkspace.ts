import { computed, type MaybeRefOrGetter } from 'vue'
// Workspace selection for the account surface (W2 transports).
//
// The selection public subpath avoids the Workspace shell entrypoint,
// which imports the account feature and would close a module cycle
// (`no-circular`). The chain below
// (`selection/workspaceContext -> selection/workspaceResource`) never touches
// account, matching the existing board precedent.
import { useWorkspaceSelection } from '../../workspace/selection/workspaceContext'

export function useAccountWorkspace(options?: { enabled?: MaybeRefOrGetter<boolean> }) {
  // Thread the caller's gate into the selection query: a bare call fires the
  // workspace list on mount, and an anonymous 401 reloads the whole page to
  // login via the shared auth-failure handler. Defaults to true so existing
  // authed account surfaces behave unchanged.
  const selection = useWorkspaceSelection({ enabled: options?.enabled })
  const workspaceId = computed(() => selection.selectedId.value)
  const isSelectionPending = computed(() => selection.status.value === 'loading')
  // Settled covers ready/empty/error/hidden/forbidden: anything past
  // loading with a null id fails closed with workspace guidance.
  const isSelectionSettled = computed(() => selection.status.value !== 'loading')
  return { workspaceId, isSelectionPending, isSelectionSettled }
}
