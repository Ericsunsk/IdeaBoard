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
import { organizationClosureIdempotencyKey } from '../../../api/idempotencyKey'
import {
  WORKSPACE_NAME_MAX_LENGTH,
  closeOrganization,
  deleteWorkspace,
  fetchOrganizationMembers,
  fetchOrganizations,
  inviteOrganizationMember,
  provisionOrganization,
  removeOrganizationMember,
  renameWorkspace,
  updateOrganizationMember,
  workspaceQueryKeys,
  type WorkspaceSummary,
} from '../selection/workspaceResource'
import { getOrganizationScopeStore, type OrganizationScope } from '../organization/scope'
import {
  WORKSPACE_URL_PARAM,
  useWorkspaceSelection,
  type WorkspaceSelectionStatus,
} from '../selection/workspaceContext'
import OrganizationPanel from '../organization/ui/OrganizationPanel.vue'

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

  async function switchScope(scope: OrganizationScope): Promise<void> {
    const scopeStore = getOrganizationScopeStore()
    const currentScope = scopeStore.getScope()
    if (
      currentScope.kind === scope.kind &&
      (scope.kind === 'personal' ||
        (currentScope.kind === 'organization' &&
          currentScope.organizationId === scope.organizationId))
    ) {
      return
    }
    if ((await options?.beforeWorkspaceChange?.()) === false) return

    const scopedItems = selection.items.value.filter((item) =>
      scope.kind === 'personal'
        ? item.organizationId == null
        : item.organizationId === scope.organizationId,
    )
    const target =
      scopedItems.find((item) => item.publicId === selection.selectedId.value) ?? scopedItems[0]
    scopeStore.select(scope)
    if (target) {
      landOnGalleryWorkspace(target.publicId)
      return
    }

    // The scope itself remains useful for organization management when it has
    // no content workspaces. Drop the old workspace and board from the URL.
    const query: LocationQueryRaw = { ...route.query }
    delete query[WORKSPACE_URL_PARAM]
    delete query[CANVAS_BOARD_ID_PARAM]
    const destination = { path: USER_BOARDS_PATH, query }
    if (route.path === USER_BOARDS_PATH) void router.replace(destination)
    else void router.push(destination)
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
    const scopeStore = getOrganizationScopeStore()
    const scope: OrganizationScope = scopeStore.getScope()
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
      // Organization management panel (issue #951): the shell owns the
      // component and its transport callbacks; the account menu renders it
      // opaquely so the organization management surface appears as soon as
      // the caller holds an organization. Self-leave also drops back to the
      // Personal scope and refreshes the list (access is gone immediately).
      organizationPanel: {
        component: OrganizationPanel,
        onLoadMembers: fetchOrganizationMembers,
        onInviteMember: inviteOrganizationMember,
        onUpdateMember: updateOrganizationMember,
        onRemoveMember: removeOrganizationMember,
        onCloseOrganization: async (
          organizationId: string,
          input: { reason?: string | undefined },
        ) => {
          const closed = await closeOrganization(organizationId, input, {
            idempotencyKey: organizationClosureIdempotencyKey(),
          })
          // Closure permanently disables every organization workspace: drop
          // back to the Personal scope and refresh the list (access is
          // gone immediately, history is retained).
          scopeStore.selectPersonal()
          await refreshWorkspaceList()
          return closed
        },
        onLeaveMember: async (organizationId: string, memberId: string): Promise<void> => {
          await removeOrganizationMember(organizationId, memberId)
          scopeStore.selectPersonal()
          await refreshWorkspaceList()
        },
      },
      activeScope: scope,
      onSelectPersonal: () => void switchScope({ kind: 'personal' }),
      onSelectOrganization: (organizationId: string) =>
        switchScope({ kind: 'organization', organizationId }),
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
