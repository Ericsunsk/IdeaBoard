import { computed } from 'vue'
// Workspace selection for the account surface (W2 transports).
//
// Deep import (not the workspace entrypoint): the entrypoint re-exports the
// workspace shell, which itself imports the account feature, so going
// through it would close a module cycle (`no-circular`). The chain below
// (`workspaceContext -> workspaceResource -> shared`) never touches
// account, matching the existing board precedent.
import { useWorkspaceSelection } from '../../workspace/infrastructure/workspaceContext'

export function useAccountWorkspace() {
  const selection = useWorkspaceSelection()
  const workspaceId = computed(() => selection.selectedId.value)
  const selectionPending = computed(() => selection.status.value === 'loading')
  // Settled covers ready/empty/error/hidden/forbidden: anything past
  // loading with a null id fails closed with workspace guidance.
  const selectionSettled = computed(() => selection.status.value !== 'loading')
  return { workspaceId, selectionPending, selectionSettled }
}
