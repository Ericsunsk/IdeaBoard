// Workspace switcher orchestration (avatar dropdown row + gear settings).
//
// The selection seam and the rename transport are mocked; the router,
// query client, and session store are stubbed. Assertions cover the
// gallery landing (no stale boardId), create-and-switch, rename refresh,
// and menu-prop mapping.
import { defineComponent, h, ref } from 'vue'
import { beforeEach, describe, expect, test, vi } from 'vitest'
import { i18n } from '../../../core/i18n'
import { render } from '../../../test/render'
import { USER_BOARDS_PATH, USER_CANVAS_PATH } from '../routePaths'
import { useWorkspaceSwitcher } from './useWorkspaceSwitcher'
import type { WorkspaceSummary } from '../selection/workspaceResource'

const routerMocks = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }))
const routeState = vi.hoisted(() => ({
  path: '/user/canvas',
  query: {} as Record<string, string | undefined>,
}))
const queryMocks = vi.hoisted(() => ({ invalidateQueries: vi.fn() }))
const selectionMocks = vi.hoisted(() => ({ useWorkspaceSelection: vi.fn() }))
const resourceMocks = vi.hoisted(() => ({ renameWorkspace: vi.fn() }))

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
  workspaceQueryKeys: { list: ['workspaces'] },
}))

const currentUser = ref<unknown>(null)

function workspaceSummary(overrides: Partial<WorkspaceSummary> & { publicId: string }) {
  return {
    name: overrides.publicId,
    organizationId: null,
    archivedAt: null,
    ...overrides,
  } as WorkspaceSummary
}

const PERSONAL = workspaceSummary({ publicId: 'ws_personal1', name: 'Personal' })
const TEAM = workspaceSummary({ publicId: 'ws_team2', name: 'Team', organizationId: 'org_c3' })

function stubSelection(overrides: Record<string, unknown> = {}) {
  const selection = {
    items: ref([PERSONAL, TEAM]),
    selectedId: ref<string | null>('ws_personal1'),
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

function renderSwitcher() {
  let switcher: ReturnType<typeof useWorkspaceSwitcher> | undefined
  const Consumer = defineComponent({
    setup() {
      switcher = useWorkspaceSwitcher()
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
    selectionMocks.useWorkspaceSelection.mockReset()
  })

  test('lands on the gallery without the stale boardId when switching from canvas', () => {
    const selection = stubSelection()
    const switcher = renderSwitcher()

    switcher.switchToGalleryWorkspace('ws_team2')

    expect(selection.selectWorkspace).toHaveBeenCalledWith('ws_team2')
    expect(routerMocks.push).toHaveBeenCalledWith({
      path: USER_BOARDS_PATH,
      query: { workspace: 'ws_team2' },
    })
    expect(routerMocks.replace).not.toHaveBeenCalled()
  })

  test('replaces the query when already on the gallery and preserves other params', () => {
    stubSelection()
    routeState.path = USER_BOARDS_PATH
    routeState.query = { workspace: 'ws_old1', accountTab: 'profile' }
    const switcher = renderSwitcher()

    switcher.switchToGalleryWorkspace('ws_team2')

    expect(routerMocks.replace).toHaveBeenCalledWith({
      path: USER_BOARDS_PATH,
      query: { workspace: 'ws_team2', accountTab: 'profile' },
    })
    expect(routerMocks.push).not.toHaveBeenCalled()
  })

  test('creates a workspace, refreshes the list, and lands on the new gallery', async () => {
    const created = workspaceSummary({ publicId: 'ws_new4', name: 'New space' })
    const selection = stubSelection({
      provisionWorkspace: vi.fn().mockResolvedValue(created),
    })
    const switcher = renderSwitcher()

    await expect(switcher.createAndSwitchWorkspace('New space')).resolves.toEqual(created)

    expect(selection.provisionWorkspace).toHaveBeenCalledWith({ name: 'New space' })
    expect(queryMocks.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['workspaces'] })
    expect(routerMocks.push).toHaveBeenCalledWith({
      path: USER_BOARDS_PATH,
      query: { workspace: 'ws_new4' },
    })
  })

  test('does not navigate when creation fails', async () => {
    stubSelection({ provisionWorkspace: vi.fn().mockRejectedValue(new Error('boom')) })
    const switcher = renderSwitcher()

    await expect(switcher.createAndSwitchWorkspace('New space')).rejects.toThrow()

    expect(queryMocks.invalidateQueries).not.toHaveBeenCalled()
    expect(routerMocks.push).not.toHaveBeenCalled()
    expect(routerMocks.replace).not.toHaveBeenCalled()
  })

  test('renames through PATCH, refreshes the list, and stays on the current route', async () => {
    const updated = workspaceSummary({ publicId: 'ws_team2', name: 'Renamed' })
    resourceMocks.renameWorkspace.mockResolvedValue(updated)
    const switcher = renderSwitcher()

    await expect(switcher.renameWorkspaceItem('ws_team2', 'Renamed')).resolves.toEqual(updated)

    expect(resourceMocks.renameWorkspace).toHaveBeenCalledWith('ws_team2', {
      name: 'Renamed',
    })
    expect(queryMocks.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['workspaces'] })
    expect(routerMocks.push).not.toHaveBeenCalled()
    expect(routerMocks.replace).not.toHaveBeenCalled()
  })

  test('maps selection state to menu props with gallery and retry commands', () => {
    const selection = stubSelection({ status: ref('hidden'), error: ref({ status: 404 }) })
    const switcher = renderSwitcher()

    const menuProps = switcher.menuProps.value

    expect(menuProps.items).toEqual([PERSONAL, TEAM])
    expect(menuProps.selectedId).toBe('ws_personal1')
    expect(menuProps.status).toBe('error')
    expect(menuProps.errorMessage).toBe('Could not load workspaces.')
    expect(menuProps.nameMaxLength).toBe(120)
    expect(menuProps.currentUserId).toBe('usr_demo1')

    menuProps.onRetry()
    expect(selection.refetch).toHaveBeenCalledOnce()

    menuProps.onSelect('ws_team2')
    expect(selection.selectWorkspace).toHaveBeenCalledWith('ws_team2')
    expect(routerMocks.push).toHaveBeenCalledWith({
      path: USER_BOARDS_PATH,
      query: { workspace: 'ws_team2' },
    })
  })
})
