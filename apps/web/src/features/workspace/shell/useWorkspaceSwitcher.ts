import { computed } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import { useQueryClient } from '@tanstack/vue-query'
import { useUserStore } from '../../auth'
import { i18n } from '../../../core/i18n'
import { userFacingErrorCopy } from '../../../core/errors/errorCatalog'
import type {
  UserAccountMenuWorkspace,
  UserAccountMenuWorkspaceCreateInput,
} from '../../account/ui/chrome/userAccountMenu.types'
import { USER_BOARDS_PATH } from '../routePaths'
import {
  WORKSPACE_NAME_MAX_LENGTH,
  deleteWorkspace,
  fetchOrganizations,
  provisionOrganization,
  renameWorkspace,
  workspaceQueryKeys,
  type WorkspaceSummary,
} from '../selection/workspaceResource'
import {
  WORKSPACE_URL_PARAM,
  useWorkspaceSelection,
  type WorkspaceSelectionStatus,
} from '../selection/workspaceContext'

// Canvas board identity rides the URL as a literal `boardId` query param (no
// shared constant); switching workspaces always drops it so the gallery never
// reopens a board from the previous workspace.
const CANVAS_BOARD_ID_PARAM = 'boardId'

function toMenuStatus(status: WorkspaceSelectionStatus): UserAccountMenuWorkspace['status'] {
  switch (status) {
    case 'loading':
      return 'loading'
    case 'ready':
      return 'ready'
    case 'empty':
      return 'empty'
    default:
      return 'error'
  }
}

// Workspace switcher for the avatar dropdown (workspace + admin headers).
//
// Owns the gallery landing after a switch: `selectWorkspace` persists the
// choice and rewrites the current URL, then this layer routes to
// `/user/boards` with the new workspace param and no stale `boardId`.
// Creation provisions (Idempotency-Key deduped in the store), refreshes the
// list without changing workspace scope. The settings dialog owns its local
// new-row focus, so the current canvas keeps its workspace/board pairing.
// Rename writes through PATCH and only refreshes the list.
export function useWorkspaceSwitcher(options?: { beforeWorkspaceChange?: () => Promise<boolean> }) {
  const userStore = useUserStore()
  const route = useRoute()
  const router = useRouter()
  const queryClient = useQueryClient()
  // Lazy getter (not a boolean-typed declaration): anonymous readers keep
  // the list query disabled, so public/guest mounts fire no request. Authed
  // readers use the shared silent-on-anonymous default.
  const selection = useWorkspaceSelection({
    enabled: () => userStore.user != null,
  })

  function landOnGalleryWorkspace(workspaceId: string): void {
    selection.selectWorkspace(workspaceId)
    const query: LocationQueryRaw = {
      ...route.query,
      [WORKSPACE_URL_PARAM]: workspaceId,
    }
    delete query[CANVAS_BOARD_ID_PARAM]
    const target = { path: USER_BOARDS_PATH, query }
    if (route.path === USER_BOARDS_PATH) void router.replace(target)
    else void router.push(target)
  }

  async function switchToGalleryWorkspace(workspaceId: string): Promise<void> {
    if ((await options?.beforeWorkspaceChange?.()) === false) return
    landOnGalleryWorkspace(workspaceId)
  }

  async function refreshWorkspaceList(): Promise<void> {
    await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.list })
  }

  async function createWorkspaceItem(
    input: UserAccountMenuWorkspaceCreateInput,
  ): Promise<WorkspaceSummary> {
    const created = await selection.provisionWorkspace({
      name: input.name,
      ...(input.organizationId === undefined ? {} : { organizationId: input.organizationId }),
      ...(input.billingAccountId === undefined ? {} : { billingAccountId: input.billingAccountId }),
      ...(input.isNewBillingAccount === true ? { isNewBillingAccount: true } : {}),
    })
    await refreshWorkspaceList()
    return created
  }

  async function renameWorkspaceItem(workspaceId: string, name: string): Promise<WorkspaceSummary> {
    // The Default workspace is never renamed: fail fast with the server
    // machine code so callers branch on code, never on copy. No transport
    // call and no list refresh on this path.
    const current = selection.items.value.find((item) => item.publicId === workspaceId)
    if (current?.isDefault === true) {
      throw Object.assign(new Error('Default workspace cannot be renamed'), {
        code: 'workspace_default_protected',
        status: 403,
      })
    }
    const updated = await renameWorkspace(workspaceId, { name })
    await refreshWorkspaceList()
    return updated
  }

  // Reuse the selection seam's fallback after deleting the selected workspace.
  async function deleteWorkspaceItem(workspaceId: string): Promise<void> {
    const isSelectedWorkspace = selection.selectedId.value === workspaceId
    if (isSelectedWorkspace && (await options?.beforeWorkspaceChange?.()) === false) {
      throw new Error('Workspace change cancelled: current draft was not saved')
    }
    await deleteWorkspace(workspaceId)
    // Refresh can already resolve the selection to Default while the URL still names the deleted row.
    await refreshWorkspaceList()
    if (isSelectedWorkspace) {
      const fallbackId = selection.selectedId.value
      if (fallbackId) landOnGalleryWorkspace(fallbackId)
    }
  }

  const menuProps = computed<UserAccountMenuWorkspace>(() => {
    // Track locale so the load-error copy follows language switches.
    void i18n.global.locale.value
    return {
      items: selection.items.value,
      selectedId: selection.selectedId.value,
      status: toMenuStatus(selection.status.value),
      errorMessage: userFacingErrorCopy(
        selection.error.value,
        i18n.global.t('workspace.switcher.loadFailed'),
      ),
      nameMaxLength: WORKSPACE_NAME_MAX_LENGTH,
      currentUserId: userStore.user?.public_id ?? null,
      onLoadOrganizations: fetchOrganizations,
      onCreateOrganization: provisionOrganization,
      onSelect: switchToGalleryWorkspace,
      onCreate: createWorkspaceItem,
      onRename: renameWorkspaceItem,
      onDelete: deleteWorkspaceItem,
      onRetry: () => {
        void selection.refetch()
      },
    }
  })

  return {
    menuProps,
    switchToGalleryWorkspace,
    createWorkspaceItem,
    renameWorkspaceItem,
    deleteWorkspaceItem,
  }
}
