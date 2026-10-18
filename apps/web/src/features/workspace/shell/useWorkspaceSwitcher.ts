import { computed } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import { useQueryClient } from '@tanstack/vue-query'
import { useUserStore } from '../../auth'
import { i18n } from '../../../core/i18n'
import { userFacingErrorCopy } from '../../../core/errors/errorCatalog'
import type { UserAccountMenuWorkspace } from '../../account/ui/chrome/userAccountMenu.types'
import { USER_BOARDS_PATH } from '../routePaths'
import {
  WORKSPACE_NAME_MAX_LENGTH,
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
// Creation provisions (Idempotency-Key deduped in the store) and lands on
// the new gallery; rename writes through PATCH and only refreshes the list.
export function useWorkspaceSwitcher() {
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

  function switchToGalleryWorkspace(workspaceId: string): void {
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

  async function refreshWorkspaceList(): Promise<void> {
    await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.list })
  }

  async function createAndSwitchWorkspace(name: string): Promise<WorkspaceSummary> {
    const created = await selection.provisionWorkspace({ name })
    await refreshWorkspaceList()
    switchToGalleryWorkspace(created.publicId)
    return created
  }

  async function renameWorkspaceItem(workspaceId: string, name: string): Promise<WorkspaceSummary> {
    const updated = await renameWorkspace(workspaceId, { name })
    await refreshWorkspaceList()
    return updated
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
      onSelect: switchToGalleryWorkspace,
      onCreate: createAndSwitchWorkspace,
      onRename: renameWorkspaceItem,
      onRetry: () => {
        void selection.refetch()
      },
    }
  })

  return { menuProps, switchToGalleryWorkspace, createAndSwitchWorkspace, renameWorkspaceItem }
}
