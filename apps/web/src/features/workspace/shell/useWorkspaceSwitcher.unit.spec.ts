// Workspace switcher orchestration (avatar dropdown row + gear settings).
//
// The selection seam and the rename transport are mocked; the router,
// query client, and session store are stubbed. Assertions cover the
// gallery landing (no stale boardId), creation preserving canvas scope,
// rename refresh,
// and menu-prop mapping.
import { computed, defineComponent, h, ref } from 'vue'
import { beforeEach, describe, expect, test, vi } from 'vitest'
import { i18n } from '../../../core/i18n'
import { render } from '../../../test/render'
import { USER_BOARDS_PATH, USER_CANVAS_PATH } from '../routePaths'
import {
  installOrganizationScopeStore,
} from '../organization/scope'
import type { OrganizationScope } from '../organization/scope'
import { useWorkspaceSwitcher } from './useWorkspaceSwitcher'
import type { WorkspaceSummary } from '../selection/workspaceResource'

const routerMocks = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }))
const routeState = vi.hoisted(() => ({
  path: '/user/canvas',
  query: {} as Record<string, string | undefined>,
}))
const queryMocks = vi.hoisted(() => ({ invalidateQueries: vi.fn() }))
const selectionMocks = vi.hoisted(() => ({ useWorkspaceSelection: vi.fn() }))
const resourceMocks = vi.hoisted(() => ({
  renameWorkspace: vi.fn(),
  renameOrganization: vi.fn(),
  deleteWorkspace: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: routerMocks.push, replace: routerMocks.replace }),
}))

vi.mock('@tanstack/vue-query', async (importOriginal) => ({
  ...((await importOriginal()) as Record<string, unknown>),
  useQueryClient: () => ({ invalidateQueries: queryMocks.invalidateQueries }),
}))

vi.mock('../../auth/session/userSession', () => ({
  useUserStore: () => ({
    get user() {
      return currentUser.value
    },
  }),
}))

vi.mock('../selection/workspaceContext', () => ({
  WORKSPACE_URL_PARAM: 'workspace',
  useWorkspaceSelection: selectionMocks.useWorkspaceSelection,
}))

vi.mock('../selection/workspaceResource', () => ({
  WORKSPACE_NAME_MAX_LENGTH: 120,
  renameWorkspace: resourceMocks.renameWorkspace,
  renameOrganization: resourceMocks.renameOrganization,
  deleteWorkspace: resourceMocks.deleteWorkspace,
  fetchOrganizations: vi.fn(),
  fetchOrganizationMembers: vi.fn(),
  inviteOrganizationMember: vi.fn(),
  updateOrganizationMember: vi.fn(),
  removeOrganizationMember: vi.fn(),
  provisionOrganization: vi.fn(),
  workspaceQueryKeys: { list: ['workspaces'], organizations: ['organizations'] },
}))

const currentUser = ref<unknown>(null)

function workspaceSummary(overrides: Partial<WorkspaceSummary> & { publicId: string }) {
  return {
    name: overrides.publicId,
    organizationId: null,
    isDefault: false,
    ...overrides,
  } as WorkspaceSummary
}

const DEFAULT = workspaceSummary({ publicId: 'ws_default1', name: 'Default', isDefault: true })
const TEAM = workspaceSummary({ publicId: 'ws_team2', name: 'Team', organizationId: 'org_c3' })

function stubSelection(overrides: Record<string, unknown> = {}) {
  const selection = {
    items: ref([DEFAULT, TEAM]),
    selectedId: ref<string | null>('ws_default1'),
    status: ref('ready'),
    error: ref<unknown>(null),
    selectWorkspace: vi.fn(),
    provisionWorkspace: vi.fn(),
    refetch: vi.fn(),
    ...overrides,
  }
  selectionMocks.useWorkspaceSelection.mockReturnValue(selection)
  return selection
}

function memoryScopePersist(initial: OrganizationScope = { kind: 'personal' }) {
  let current = initial
  return {
    readScope: () => current,
    writeScope: (scope: OrganizationScope) => {
      current = scope
    },
  }
}

function renderSwitcher(options?: Parameters<typeof useWorkspaceSwitcher>[0]) {
  let switcher: ReturnType<typeof useWorkspaceSwitcher> | undefined
  const Consumer = defineComponent({
    setup() {
      switcher = useWorkspaceSwitcher(options)
      return () => h('div')
    },
  })
  render(Consumer)
  if (!switcher) throw new Error('Switcher harness failed to initialize')
  return switcher
}

describe('useWorkspaceSwitcher', () => {
  beforeEach(() => {
    i18n.global.locale.value = 'en'
    currentUser.value = { public_id: 'usr_demo1' }
    routeState.path = USER_CANVAS_PATH
    routeState.query = { boardId: 'board_1', workspace: 'ws_old1' }
    routerMocks.push.mockReset()
    routerMocks.replace.mockReset()
    queryMocks.invalidateQueries.mockReset()
    resourceMocks.renameWorkspace.mockReset()
    resourceMocks.renameOrganization.mockReset()
    resourceMocks.deleteWorkspace.mockReset()
    selectionMocks.useWorkspaceSelection.mockReset()
    installOrganizationScopeStore({ persist: memoryScopePersist() })
  })

  test('lands on the gallery without the stale boardId when switching from canvas', async () => {
    const selection = stubSelection()
    const switcher = renderSwitcher()

    await switcher.switchToGalleryWorkspace('ws_team2')

    expect(selection.selectWorkspace).toHaveBeenCalledWith('ws_team2')
    expect(routerMocks.push).toHaveBeenCalledWith({
      path: USER_BOARDS_PATH,
      query: { workspace: 'ws_team2' },
    })
    expect(routerMocks.replace).not.toHaveBeenCalled()
  })

  test('replaces the query when already on the gallery and preserves other params', async () => {
    stubSelection()
    routeState.path = USER_BOARDS_PATH
    routeState.query = { workspace: 'ws_old1', accountTab: 'profile' }
    const switcher = renderSwitcher()

    await switcher.switchToGalleryWorkspace('ws_team2')

    expect(routerMocks.replace).toHaveBeenCalledWith({
      path: USER_BOARDS_PATH,
      query: { workspace: 'ws_team2', accountTab: 'profile' },
    })
    expect(routerMocks.push).not.toHaveBeenCalled()
  })

  test.each(['select', 'delete'] as const)(
    'waits for the current draft before %s changes workspace scope',
    async (action) => {
      let finishSave!: (isSaved: boolean) => void
      const saved = new Promise<boolean>((resolve) => {
        finishSave = resolve
      })
      const beforeWorkspaceChange = vi.fn(() => saved)
      const selection = stubSelection({
        provisionWorkspace: vi.fn().mockResolvedValue(TEAM),
      })
      resourceMocks.deleteWorkspace.mockResolvedValue(undefined)
      const switcher = renderSwitcher({ beforeWorkspaceChange })
      const changing =
        action === 'select'
          ? switcher.switchToGalleryWorkspace(TEAM.publicId)
          : switcher.deleteWorkspaceItem(DEFAULT.publicId)

      expect(beforeWorkspaceChange).toHaveBeenCalledOnce()
      expect(selection.selectWorkspace).not.toHaveBeenCalled()
      expect(selection.provisionWorkspace).not.toHaveBeenCalled()
      expect(resourceMocks.deleteWorkspace).not.toHaveBeenCalled()
      expect(routerMocks.push).not.toHaveBeenCalled()
      finishSave(true)
      await changing
      expect(routerMocks.push).toHaveBeenCalledOnce()
    },
  )

  test.each(['select', 'delete'] as const)(
    'preserves workspace scope when the draft save fails during %s',
    async (action) => {
      const selection = stubSelection()
      const switcher = renderSwitcher({ beforeWorkspaceChange: async () => false })
      if (action === 'select') await switcher.switchToGalleryWorkspace(TEAM.publicId)
      else {
        await expect(switcher.deleteWorkspaceItem(DEFAULT.publicId)).rejects.toThrow('not saved')
      }
      expect(selection.selectWorkspace).not.toHaveBeenCalled()
      expect(selection.provisionWorkspace).not.toHaveBeenCalled()
      expect(resourceMocks.deleteWorkspace).not.toHaveBeenCalled()
      expect(routerMocks.push).not.toHaveBeenCalled()
    },
  )

  test('forwards explicit organization scope without a personal billing override', async () => {
    const selection = stubSelection({ provisionWorkspace: vi.fn().mockResolvedValue(TEAM) })
    const switcher = renderSwitcher()
    await switcher.createWorkspaceItem({ name: 'Department', organizationId: 'org_c3' })
    expect(selection.provisionWorkspace).toHaveBeenCalledWith({
      name: 'Department',
      organizationId: 'org_c3',
    })
    expect(routerMocks.push).not.toHaveBeenCalled()
  })

  test('preserves workspace scope when the draft flush rejects', async () => {
    const selection = stubSelection()
    const switcher = renderSwitcher({
      beforeWorkspaceChange: () => Promise.reject(new Error('save failed')),
    })
    await expect(switcher.switchToGalleryWorkspace(TEAM.publicId)).rejects.toThrow('save failed')
    expect(selection.selectWorkspace).not.toHaveBeenCalled()
    expect(routerMocks.push).not.toHaveBeenCalled()
  })

  test('creates a workspace and refreshes the list without changing canvas scope', async () => {
    const created = workspaceSummary({ publicId: 'ws_new4', name: 'New space' })
    const selection = stubSelection({
      provisionWorkspace: vi.fn().mockResolvedValue(created),
    })
    const beforeWorkspaceChange = vi.fn().mockResolvedValue(false)
    const originalQuery = { ...routeState.query }
    const switcher = renderSwitcher({ beforeWorkspaceChange })

    await expect(switcher.createWorkspaceItem({ name: 'New space' })).resolves.toEqual(created)

    expect(selection.provisionWorkspace).toHaveBeenCalledWith({ name: 'New space' })
    expect(queryMocks.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['workspaces'] })
    expect(beforeWorkspaceChange).not.toHaveBeenCalled()
    expect(selection.selectWorkspace).not.toHaveBeenCalled()
    expect(selection.selectedId.value).toBe(DEFAULT.publicId)
    expect(routeState.query).toEqual(originalQuery)
    expect(routerMocks.push).not.toHaveBeenCalled()
    expect(routerMocks.replace).not.toHaveBeenCalled()
  })

  test('does not select when creation fails', async () => {
    const selection = stubSelection({
      provisionWorkspace: vi.fn().mockRejectedValue(new Error('boom')),
    })
    const switcher = renderSwitcher()

    await expect(switcher.createWorkspaceItem({ name: 'New space' })).rejects.toThrow()

    expect(queryMocks.invalidateQueries).not.toHaveBeenCalled()
    expect(selection.selectWorkspace).not.toHaveBeenCalled()
    expect(routerMocks.push).not.toHaveBeenCalled()
    expect(routerMocks.replace).not.toHaveBeenCalled()
  })

  test('renames through PATCH, refreshes the list, and stays on the current route', async () => {
    const updated = workspaceSummary({ publicId: 'ws_team2', name: 'Renamed' })
    resourceMocks.renameWorkspace.mockResolvedValue(updated)
    stubSelection()
    const switcher = renderSwitcher()

    await expect(switcher.renameWorkspaceItem('ws_team2', 'Renamed')).resolves.toEqual(updated)

    expect(resourceMocks.renameWorkspace).toHaveBeenCalledWith('ws_team2', {
      name: 'Renamed',
    })
    expect(queryMocks.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['workspaces'] })
    expect(routerMocks.push).not.toHaveBeenCalled()
    expect(routerMocks.replace).not.toHaveBeenCalled()
  })

  test('rejects renaming the default row with the server code without touching the network', async () => {
    stubSelection()
    const switcher = renderSwitcher()

    await expect(switcher.renameWorkspaceItem('ws_default1', 'Renamed')).rejects.toMatchObject({
      code: 'workspace_default_protected',
    })
    expect(resourceMocks.renameWorkspace).not.toHaveBeenCalled()
    expect(queryMocks.invalidateQueries).not.toHaveBeenCalled()
    expect(routerMocks.push).not.toHaveBeenCalled()
    expect(routerMocks.replace).not.toHaveBeenCalled()
  })

  test('renames the organization through PATCH and refreshes the organization list', async () => {
    const updated = { publicId: 'org_c3', name: 'Renamed', status: 'active' as const }
    resourceMocks.renameOrganization.mockResolvedValue(updated)
    stubSelection()
    const switcher = renderSwitcher()

    await expect(switcher.renameOrganizationItem('org_c3', 'Renamed')).resolves.toEqual(updated)

    expect(resourceMocks.renameOrganization).toHaveBeenCalledWith('org_c3', { name: 'Renamed' })
    expect(queryMocks.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['organizations'] })
    expect(routerMocks.push).not.toHaveBeenCalled()
    expect(routerMocks.replace).not.toHaveBeenCalled()
  })

  test('exposes organization rename through menu props without leaving the route', async () => {
    const updated = { publicId: 'org_c3', name: 'Renamed', status: 'active' as const }
    resourceMocks.renameOrganization.mockResolvedValue(updated)
    stubSelection()
    const switcher = renderSwitcher()

    await expect(
      switcher.menuProps.value.onRenameOrganization?.('org_c3', 'Renamed'),
    ).resolves.toEqual(updated)

    expect(resourceMocks.renameOrganization).toHaveBeenCalledWith('org_c3', { name: 'Renamed' })
    expect(routerMocks.push).not.toHaveBeenCalled()
    expect(routerMocks.replace).not.toHaveBeenCalled()
  })

  test('maps selection state to menu props with gallery and retry commands', async () => {
    const selection = stubSelection({ status: ref('hidden'), error: ref({ status: 404 }) })
    const switcher = renderSwitcher()

    const menuProps = switcher.menuProps.value

    expect(menuProps.items).toEqual([DEFAULT, TEAM])
    expect(menuProps.selectedId).toBe('ws_default1')
    expect(menuProps.status).toBe('error')
    expect(menuProps.errorMessage).toBe('Could not load workspaces.')
    expect(menuProps.nameMaxLength).toBe(120)
    expect(menuProps.currentUserId).toBe('usr_demo1')

    menuProps.onRetry()
    expect(selection.refetch).toHaveBeenCalledOnce()

    await menuProps.onSelect('ws_team2')
    expect(selection.selectWorkspace).toHaveBeenCalledWith('ws_team2')
    expect(routerMocks.push).toHaveBeenCalledWith({
      path: USER_BOARDS_PATH,
      query: { workspace: 'ws_team2' },
    })
  })

  test('creates through menu props without leaving the current route', async () => {
    const created = workspaceSummary({ publicId: 'ws_new4', name: 'New space' })
    const selection = stubSelection({
      provisionWorkspace: vi.fn().mockResolvedValue(created),
    })
    const switcher = renderSwitcher()

    await expect(switcher.menuProps.value.onCreate({ name: 'New space' })).resolves.toEqual(created)

    expect(selection.provisionWorkspace).toHaveBeenCalledWith({ name: 'New space' })
    expect(selection.selectWorkspace).not.toHaveBeenCalled()
    expect(routerMocks.push).not.toHaveBeenCalled()
    expect(routerMocks.replace).not.toHaveBeenCalled()
  })

  test('leaves the deleted canvas after refresh falls selection back to default', async () => {
    resourceMocks.deleteWorkspace.mockResolvedValue(undefined)
    const items = ref([DEFAULT, TEAM])
    const selectedId = computed(
      () =>
        items.value.find((item) => item.publicId === TEAM.publicId)?.publicId ?? DEFAULT.publicId,
    )
    const selection = stubSelection({ items, selectedId })
    routeState.query = { boardId: 'board_1', workspace: TEAM.publicId }
    queryMocks.invalidateQueries.mockImplementation(async () => {
      items.value = [DEFAULT]
    })
    const switcher = renderSwitcher()

    await expect(switcher.deleteWorkspaceItem('ws_team2')).resolves.toBeUndefined()

    expect(resourceMocks.deleteWorkspace).toHaveBeenCalledWith('ws_team2')
    expect(queryMocks.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['workspaces'] })
    expect(selectedId.value).toBe(DEFAULT.publicId)
    expect(selection.selectWorkspace).toHaveBeenCalledWith('ws_default1')
    expect(routerMocks.push).toHaveBeenCalledWith({
      path: USER_BOARDS_PATH,
      query: { workspace: 'ws_default1' },
    })
  })

  test('stays on the current canvas when deleting another workspace', async () => {
    resourceMocks.deleteWorkspace.mockResolvedValue(undefined)
    const selection = stubSelection()
    routeState.query = { boardId: 'board_1', workspace: DEFAULT.publicId }
    const switcher = renderSwitcher()

    await switcher.deleteWorkspaceItem(TEAM.publicId)

    expect(queryMocks.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['workspaces'] })
    expect(selection.selectWorkspace).not.toHaveBeenCalled()
    expect(routerMocks.push).not.toHaveBeenCalled()
    expect(routerMocks.replace).not.toHaveBeenCalled()
  })

  test('propagates the server code when deleting the default row fails', async () => {
    const failure = Object.assign(new Error('request failed'), {
      code: 'workspace_default_protected',
    })
    resourceMocks.deleteWorkspace.mockRejectedValue(failure)
    stubSelection()
    const switcher = renderSwitcher()

    await expect(switcher.deleteWorkspaceItem('ws_default1')).rejects.toMatchObject({
      code: 'workspace_default_protected',
    })
    expect(queryMocks.invalidateQueries).not.toHaveBeenCalled()
    expect(routerMocks.push).not.toHaveBeenCalled()
  })
})
