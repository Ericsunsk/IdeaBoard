// Guest disabled mapping regression (issue: unauthenticated search page toasts
// "Could not load workspaces").
//
// Selection reports `disabled` for anonymous readers (query disabled, no
// request). The menu view-model must not fold it into `error`, otherwise the
// account menu treats the fallback copy as a load failure and toasts on mount.
import { defineComponent, h, ref } from 'vue'
import { beforeEach, describe, expect, test, vi } from 'vitest'
import { i18n } from '../../../core/i18n'
import { render } from '../../../test/render'
import { useWorkspaceSwitcher } from './useWorkspaceSwitcher'

const routerMocks = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }))
const routeState = vi.hoisted(() => ({
  path: '/search',
  query: {} as Record<string, string | undefined>,
}))
const queryMocks = vi.hoisted(() => ({ invalidateQueries: vi.fn() }))
const organizationMocks = vi.hoisted(() => ({ useManagement: vi.fn() }))
const selectionMocks = vi.hoisted(() => ({ useWorkspaceSelection: vi.fn() }))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: routerMocks.push, replace: routerMocks.replace }),
}))

vi.mock('@tanstack/vue-query', async (importOriginal) => ({
  ...((await importOriginal()) as Record<string, unknown>),
  useQueryClient: () => ({ invalidateQueries: queryMocks.invalidateQueries }),
}))

vi.mock('../../auth/session/userSession', () => ({
  useUserStore: () => ({ user: null }),
}))

vi.mock('../organization/session/useManagement', () => organizationMocks)

vi.mock('../selection/workspaceContext', () => ({
  WORKSPACE_URL_PARAM: 'workspace',
  useWorkspaceSelection: selectionMocks.useWorkspaceSelection,
}))

vi.mock('../selection/workspaceResource', () => ({
  WORKSPACE_NAME_MAX_LENGTH: 120,
  renameWorkspace: vi.fn(),
  deleteWorkspace: vi.fn(),
  fetchOrganizations: vi.fn(),
  provisionOrganization: vi.fn(),
  workspaceQueryKeys: { list: ['workspaces'], organizations: ['organizations'] },
}))

function stubSelection(overrides: Record<string, unknown> = {}) {
  const selection = {
    items: ref([]),
    selectedId: ref<string | null>(null),
    status: ref('disabled'),
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

describe('useWorkspaceSwitcher guest disabled mapping', () => {
  beforeEach(() => {
    i18n.global.locale.value = 'en'
    organizationMocks.useManagement.mockReset()
    organizationMocks.useManagement.mockReturnValue({
      organization: ref(null),
      refresh: vi.fn(),
    })
    selectionMocks.useWorkspaceSelection.mockReset()
  })

  test('maps anonymous disabled selection to a non-error menu status', () => {
    stubSelection({ status: ref('disabled'), error: ref(null) })
    const switcher = renderSwitcher()

    expect(switcher.menuProps.value.status).not.toBe('error')
    expect(switcher.menuProps.value.status).toBe('empty')
  })

  test('keeps real load failures as error for signed-in readers', () => {
    for (const status of ['error', 'hidden', 'forbidden'] as const) {
      stubSelection({ status: ref(status), error: ref({ status: 500 }) })
      const switcher = renderSwitcher()
      expect(switcher.menuProps.value.status).toBe('error')
    }
  })
})
