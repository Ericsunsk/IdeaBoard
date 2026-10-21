// Workspace selection state tests (issue #801). No network: the resource
// seam is mocked, storage is in-memory, and useQuery is stubbed per the
// useCreditWallet unit pattern.
import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  asWorkspaceId,
  createWorkspaceSelectionStore,
  localStorageWorkspacePersist,
  memoryWorkspacePersist,
  resolveWorkspaceSelection,
  subscribeWorkspaceSelectionChange,
  toWorkspaceSelectionStatus,
  useWorkspaceSelection,
  WORKSPACE_SELECTION_STORAGE_KEY,
  WORKSPACE_URL_PARAM,
  type WorkspaceSelectionStore,
} from './workspaceContext'
import type { WorkspaceSummary } from './workspaceResource'

const queryMocks = vi.hoisted(() => ({ useQuery: vi.fn() }))
const routerMocks = vi.hoisted(() => ({
  useRoute: vi.fn(),
  useRouter: vi.fn(),
  replace: vi.fn(),
}))
const resourceMocks = vi.hoisted(() => ({
  fetchWorkspacePage: vi.fn(),
  provisionWorkspace: vi.fn(),
}))

vi.mock('@tanstack/vue-query', async (importOriginal) => ({
  ...((await importOriginal()) as Record<string, unknown>),
  useQuery: queryMocks.useQuery,
}))
vi.mock('vue-router', () => ({
  useRoute: routerMocks.useRoute,
  useRouter: routerMocks.useRouter,
}))
vi.mock('./workspaceResource', () => ({
  classifyWorkspaceAccessError: (error: unknown) => {
    const status = (error as { status?: unknown }).status
    if (status === 404) return 'hidden'
    if (status === 403) return 'forbidden'
    return null
  },
  fetchWorkspacePage: resourceMocks.fetchWorkspacePage,
  provisionWorkspace: resourceMocks.provisionWorkspace,
  workspaceQueryKeys: { list: ['workspaces'] },
}))

function workspace(overrides: Partial<WorkspaceSummary> & { publicId: string }): WorkspaceSummary {
  return {
    name: overrides.publicId,
    organizationId: null,
    isDefault: false,
    ...overrides,
  }
}

const DEFAULT = workspace({ publicId: 'ws_default1', name: 'Default', isDefault: true })
const PLAIN = workspace({ publicId: 'ws_plain3', name: 'Plain' })
const TEAM = workspace({ publicId: 'ws_team2', name: 'Team', organizationId: 'org_c3' })

function mockQuery(state: {
  items?: WorkspaceSummary[] | undefined
  isPending?: boolean | undefined
  error?: unknown
}) {
  const data = ref(state.items === undefined ? undefined : { items: state.items })
  queryMocks.useQuery.mockReturnValue({
    data,
    isPending: ref(state.isPending ?? false),
    isError: ref(state.error !== undefined && state.error !== null),
    error: ref(state.error ?? null),
    refetch: vi.fn(),
  })
}

function mockRoute(query: Record<string, unknown> = {}) {
  routerMocks.useRoute.mockReturnValue({ query })
  routerMocks.useRouter.mockReturnValue({ replace: routerMocks.replace })
}

function freshStore(): WorkspaceSelectionStore {
  return createWorkspaceSelectionStore({ persist: memoryWorkspacePersist() })
}

describe('resolveWorkspaceSelection', () => {
  it('prefers the shared URL choice over the persisted one', () => {
    expect(
      resolveWorkspaceSelection([DEFAULT, TEAM], { urlId: 'ws_team2', storedId: 'ws_default1' }),
    ).toEqual({ selectedId: 'ws_team2', source: 'url' })
  })

  it('falls back to the persisted choice and then to default', () => {
    expect(resolveWorkspaceSelection([DEFAULT, TEAM], { storedId: 'ws_team2' })).toEqual({
      selectedId: 'ws_team2',
      source: 'stored',
    })
    expect(resolveWorkspaceSelection([TEAM, DEFAULT], {})).toEqual({
      selectedId: 'ws_default1',
      source: 'auto-default',
    })
  })

  it('uses the first visible item when no default exists', () => {
    expect(resolveWorkspaceSelection([TEAM], {})).toEqual({
      selectedId: 'ws_team2',
      source: 'auto-first',
    })
    expect(resolveWorkspaceSelection([], {})).toEqual({ selectedId: null, source: 'none' })
  })

  it('never treats a plain personal row as the default', () => {
    expect(resolveWorkspaceSelection([PLAIN, TEAM], {})).toEqual({
      selectedId: 'ws_plain3',
      source: 'auto-first',
    })
    expect(resolveWorkspaceSelection([TEAM, PLAIN, DEFAULT], {})).toEqual({
      selectedId: 'ws_default1',
      source: 'auto-default',
    })
  })

  it('ignores stale or malformed hints instead of selecting them', () => {
    expect(resolveWorkspaceSelection([DEFAULT], { urlId: 'ws_gone9', storedId: '123' })).toEqual({
      selectedId: 'ws_default1',
      source: 'auto-default',
    })
  })
})

describe('selection persistence', () => {
  it('validates workspace IDs at the boundary', () => {
    expect(asWorkspaceId('ws_abc1')).toBe('ws_abc1')
    expect(asWorkspaceId('usr_u1')).toBeNull()
    expect(asWorkspaceId('123')).toBeNull()
    expect(asWorkspaceId(null)).toBeNull()
  })

  it('persists through localStorage under a stable key', () => {
    const persist = localStorageWorkspacePersist()

    persist.writeSelectedId('ws_default1')

    expect(window.localStorage.getItem(WORKSPACE_SELECTION_STORAGE_KEY)).toBe('ws_default1')
    expect(persist.readSelectedId()).toBe('ws_default1')

    persist.writeSelectedId(null)
    expect(window.localStorage.getItem(WORKSPACE_SELECTION_STORAGE_KEY)).toBeNull()
  })

  it('survives reloads through the persisted ID', () => {
    const persist = memoryWorkspacePersist()
    const first = createWorkspaceSelectionStore({ persist })
    first.select('ws_team2')

    const reloaded = createWorkspaceSelectionStore({ persist })
    expect(reloaded.getSelectedId()).toBe('ws_team2')
    expect(
      resolveWorkspaceSelection([DEFAULT, TEAM], { storedId: reloaded.getSelectedId() }).source,
    ).toBe('stored')
  })

  it('rejects non-opaque selections without touching storage', () => {
    const store = freshStore()

    expect(() => store.select('123')).toThrow()
    expect(store.getSelectedId()).toBeNull()
  })
})

describe('selection status', () => {
  it('splits hidden (404) from forbidden (403)', () => {
    expect(
      toWorkspaceSelectionStatus({
        enabled: true,
        isPending: false,
        isError: true,
        error: { status: 404 },
        itemCount: 0,
      }),
    ).toBe('hidden')
    expect(
      toWorkspaceSelectionStatus({
        enabled: true,
        isPending: false,
        isError: true,
        error: { status: 403 },
        itemCount: 0,
      }),
    ).toBe('forbidden')
    expect(
      toWorkspaceSelectionStatus({
        enabled: true,
        isPending: false,
        isError: true,
        error: { status: 500 },
        itemCount: 0,
      }),
    ).toBe('error')
  })

  it('covers disabled, loading, empty, and ready', () => {
    const base = { enabled: true, isPending: false, isError: false, itemCount: 2 }
    expect(toWorkspaceSelectionStatus({ ...base, enabled: false })).toBe('disabled')
    expect(toWorkspaceSelectionStatus({ ...base, isPending: true })).toBe('loading')
    expect(toWorkspaceSelectionStatus({ ...base, itemCount: 0 })).toBe('empty')
    expect(toWorkspaceSelectionStatus(base)).toBe('ready')
  })
})

describe('concurrent provision idempotency', () => {
  it('dedupes concurrent creates for the same intent', async () => {
    const created = workspace({ publicId: 'ws_new1', name: 'New' })
    let release!: () => void
    const gate = new Promise<void>((resolve) => {
      release = resolve
    })
    const provision = vi.fn(async (): Promise<WorkspaceSummary> => {
      await gate
      return created
    })
    const store = createWorkspaceSelectionStore({ persist: memoryWorkspacePersist(), provision })

    const first = store.provision({ name: 'New' })
    const second = store.provision({ name: '  NEW ' })

    expect(provision).toHaveBeenCalledTimes(1)
    release()
    await expect(first).resolves.toEqual(created)
    await expect(second).resolves.toEqual(created)
    expect(store.getSelectedId()).toBeNull()

    await expect(store.provision({ name: 'New' })).resolves.toEqual(created)
    expect(provision).toHaveBeenCalledTimes(2)
  })

  it('keeps the same workspace name in different organizations on separate flights', async () => {
    let release!: () => void
    const gate = new Promise<void>((resolve) => {
      release = resolve
    })
    const provision = vi.fn(async () => {
      await gate
      return TEAM
    })
    const store = createWorkspaceSelectionStore({ persist: memoryWorkspacePersist(), provision })
    const first = store.provision({ name: 'Design', organizationId: 'org_first' })
    const second = store.provision({ name: 'Design', organizationId: 'org_second' })
    expect(provision).toHaveBeenCalledTimes(2)
    expect(provision).toHaveBeenCalledWith({ name: 'Design', organizationId: 'org_first' })
    expect(provision).toHaveBeenCalledWith({ name: 'Design', organizationId: 'org_second' })
    release()
    await Promise.all([first, second])
  })

  it('preserves the selected and persisted workspace without emitting a scope change', async () => {
    const persist = memoryWorkspacePersist(DEFAULT.publicId)
    const store = createWorkspaceSelectionStore({
      persist,
      provision: vi.fn().mockResolvedValue(TEAM),
    })
    expect(store.getSelectedId()).toBe(DEFAULT.publicId)
    const listener = vi.fn()
    const unsubscribe = subscribeWorkspaceSelectionChange(listener)
    try {
      await expect(store.provision({ name: 'Team' })).resolves.toEqual(TEAM)

      expect(store.selectedId.value).toBe(DEFAULT.publicId)
      expect(persist.readSelectedId()).toBe(DEFAULT.publicId)
      expect(listener).not.toHaveBeenCalled()
    } finally {
      unsubscribe()
    }
  })

  it('keeps different intents on separate flights', async () => {
    const provision = vi.fn(async (input: { name: string }) =>
      workspace({ publicId: `ws_${input.name}`, name: input.name }),
    )
    const store = createWorkspaceSelectionStore({ persist: memoryWorkspacePersist(), provision })

    await Promise.all([store.provision({ name: 'A' }), store.provision({ name: 'B' })])

    expect(provision).toHaveBeenCalledTimes(2)
  })
})

describe('useWorkspaceSelection', () => {
  beforeEach(() => {
    queryMocks.useQuery.mockReset()
    routerMocks.useRoute.mockReset()
    routerMocks.useRouter.mockReset()
    routerMocks.replace.mockReset()
  })

  it('auto-selects the default workspace, persists it, and reports ready', () => {
    mockQuery({ items: [TEAM, DEFAULT] })
    mockRoute()

    const store = freshStore()
    const selection = useWorkspaceSelection({ store })

    expect(selection.selectedId.value).toBe('ws_default1')
    // Auto-resolution is persisted so cross-layer readers
    // (`getSelectedId()`) see the same workspace; the source then
    // reports 'stored' on re-evaluation.
    expect(store.getSelectedId()).toBe('ws_default1')
    expect(selection.selectionSource.value).toBe('stored')
    expect(selection.status.value).toBe('ready')
  })

  it('persists explicit selection and reflects it in the URL', () => {
    mockQuery({ items: [DEFAULT, TEAM] })
    mockRoute()

    const selection = useWorkspaceSelection({ store: freshStore() })
    selection.selectWorkspace('ws_team2')

    expect(selection.selectedId.value).toBe('ws_team2')
    expect(routerMocks.replace).toHaveBeenCalledWith(
      expect.objectContaining({
        query: expect.objectContaining({ [WORKSPACE_URL_PARAM]: 'ws_team2' }),
      }),
    )
  })

  it('maps pending and hidden states without extra stores', () => {
    mockQuery({ isPending: true })
    mockRoute()
    expect(useWorkspaceSelection({ store: freshStore() }).status.value).toBe('loading')

    mockQuery({ error: { status: 404 } })
    mockRoute()
    expect(useWorkspaceSelection({ store: freshStore() }).status.value).toBe('hidden')

    mockQuery({ items: [] })
    mockRoute()
    const empty = useWorkspaceSelection({ store: freshStore(), enabled: false })
    expect(empty.status.value).toBe('disabled')
  })

  it('fetches the list silently when anonymous, redirect by default when authed', async () => {
    mockQuery({ items: [] })
    mockRoute()
    resourceMocks.fetchWorkspacePage.mockReset()

    localStorage.clear()
    useWorkspaceSelection({ store: freshStore() })
    const anonymousQueryFn = queryMocks.useQuery.mock.calls[0]?.[0]?.queryFn
    await anonymousQueryFn({ signal: undefined })
    expect(resourceMocks.fetchWorkspacePage).toHaveBeenCalledWith(
      undefined,
      expect.objectContaining({ authFailure: 'silent' }),
    )

    localStorage.setItem('user', JSON.stringify({ public_id: 'usr_anonymous1' }))
    resourceMocks.fetchWorkspacePage.mockClear()
    useWorkspaceSelection({ store: freshStore() })
    const authedQueryFn = queryMocks.useQuery.mock.calls[1]?.[0]?.queryFn
    await authedQueryFn({ signal: undefined })
    expect(resourceMocks.fetchWorkspacePage).toHaveBeenCalledWith(
      undefined,
      expect.not.objectContaining({ authFailure: 'silent' }),
    )
    localStorage.clear()
  })

  it('honors an explicit silent override for authed readers', async () => {
    mockQuery({ items: [] })
    mockRoute()
    resourceMocks.fetchWorkspacePage.mockReset()
    localStorage.setItem('user', JSON.stringify({ public_id: 'usr_explicit1' }))

    useWorkspaceSelection({ store: freshStore(), authFailure: 'silent' })
    const queryFn = queryMocks.useQuery.mock.calls[0]?.[0]?.queryFn
    await queryFn({ signal: undefined })

    expect(resourceMocks.fetchWorkspacePage).toHaveBeenCalledWith(
      undefined,
      expect.objectContaining({ authFailure: 'silent' }),
    )
    localStorage.clear()
  })
})
