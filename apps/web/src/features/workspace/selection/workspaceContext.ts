// Workspace selection state (issue #801, frontend slice).
//
// State Ownership Contract mapping for workspace scope:
// - TanStack Query cache (`workspaceQueryKeys.list`) is the single source of
//   truth for server data; it is never copied into another global store.
// - The URL param (`?workspace=ws_...`) owns shareable/back-navigable nav state.
// - A narrow persisted store below owns only the selected opaque ID
//   (pure client state) and reload persistence. Board ACL (#802+) layers its
//   own scope on top of the selected workspace ID exposed here.

import { computed, shallowRef, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { useRoute, useRouter } from 'vue-router'
import {
  classifyWorkspaceAccessError,
  fetchWorkspacePage,
  provisionWorkspace,
  workspaceQueryKeys,
  type ProvisionWorkspaceBillingInput,
  type WorkspaceAccessKind,
  type WorkspaceSummary,
} from './workspaceResource'
import {
  readLocalStorageItem,
  removeLocalStorageItem,
  writeLocalStorageItem,
} from '../../../core/utils/storage'
import { isOpaqueId, opaqueIdPrefix } from '../../../api/contractKernel'
import type { AuthFailureMode } from '../../../api/client'
import { getStoredUser } from '../../../core/utils/auth'
import { stringParam } from '../../../core/router/queryParams'
import { accountScoped } from '../../../core/query/queryDefaults'
import { setMediaWorkspaceIdResolver } from '../../../api/mediaWorkspaceScope'

export const WORKSPACE_URL_PARAM = 'workspace'
export const WORKSPACE_SELECTION_STORAGE_KEY = 'ideaboard.workspace.selected.v1'

const workspaceSelectionListeners = new Set<(id: string | null) => void>()

function notifyWorkspaceSelectionChange(id: string | null) {
  workspaceSelectionListeners.forEach((listener) => listener(id))
}

/**
 * Subscribes to explicit workspace selection changes (select/clear).
 * Board catalog isolation listens here alongside the auth-scope
 * signal so per-scope caches invalidate on either axis.
 */
export function subscribeWorkspaceSelectionChange(
  listener: (id: string | null) => void,
): () => void {
  workspaceSelectionListeners.add(listener)
  return () => workspaceSelectionListeners.delete(listener)
}

export type WorkspaceSelectionSource = 'url' | 'stored' | 'auto-default' | 'auto-first' | 'none'

export type WorkspaceSelectionStatus =
  'loading' | 'empty' | 'ready' | 'error' | 'hidden' | 'forbidden' | 'disabled'

export function asWorkspaceId(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  if (!isOpaqueId(trimmed) || opaqueIdPrefix(trimmed) !== 'ws') return null
  return trimmed
}

// Resolution order: explicit URL share > persisted choice > Default
// workspace (`isDefault`) > first visible item > none.
export function resolveWorkspaceSelection(
  items: readonly WorkspaceSummary[],
  hints?: { urlId?: unknown; storedId?: unknown },
): { selectedId: string | null; source: WorkspaceSelectionSource } {
  const visible = new Set(items.map((item) => item.publicId))
  const urlId = asWorkspaceId(hints?.urlId)
  if (urlId && visible.has(urlId)) return { selectedId: urlId, source: 'url' }
  const storedId = asWorkspaceId(hints?.storedId)
  if (storedId && visible.has(storedId)) return { selectedId: storedId, source: 'stored' }
  const def = items.find((item) => item.isDefault)
  if (def) return { selectedId: def.publicId, source: 'auto-default' }
  const first = items[0]
  if (first) return { selectedId: first.publicId, source: 'auto-first' }
  return { selectedId: null, source: 'none' }
}

export function toWorkspaceSelectionStatus(input: {
  enabled: boolean
  isPending: boolean
  isError: boolean
  error?: unknown
  itemCount: number
}): WorkspaceSelectionStatus {
  if (!input.enabled) return 'disabled'
  if (input.isPending) return 'loading'
  if (input.isError) {
    const access: WorkspaceAccessKind | null = classifyWorkspaceAccessError(input.error)
    if (access === 'hidden') return 'hidden'
    if (access === 'forbidden') return 'forbidden'
    return 'error'
  }
  if (input.itemCount === 0) return 'empty'
  return 'ready'
}

export type WorkspaceSelectionPersist = {
  readSelectedId: () => string | null
  writeSelectedId: (id: string | null) => void
}

export function localStorageWorkspacePersist(
  key: string = WORKSPACE_SELECTION_STORAGE_KEY,
): WorkspaceSelectionPersist {
  return {
    readSelectedId: () => asWorkspaceId(readLocalStorageItem(key)),
    writeSelectedId: (id) => {
      if (id == null) {
        removeLocalStorageItem(key)
        return
      }
      if (!writeLocalStorageItem(key, id)) {
        console.warn('[workspace] persist selection failed')
      }
    },
  }
}

export function memoryWorkspacePersist(initial: string | null = null): WorkspaceSelectionPersist {
  let current = asWorkspaceId(initial)
  return {
    readSelectedId: () => current,
    writeSelectedId: (id) => {
      current = asWorkspaceId(id)
    },
  }
}

export type ProvisionWorkspaceInput = {
  name: string
  organizationId?: string | undefined
} & ProvisionWorkspaceBillingInput

export type WorkspaceProvisionFn = (
  input: ProvisionWorkspaceInput,
  request?: { idempotencyKey?: string | undefined; requestId?: string | undefined },
) => Promise<WorkspaceSummary>

// Narrow client-only store: holds the selected ID plus in-flight provision
// dedupe. Server data always comes from the TanStack Query cache.
export function createWorkspaceSelectionStore(deps?: {
  persist?: WorkspaceSelectionPersist | undefined
  provision?: WorkspaceProvisionFn | undefined
}) {
  const persist = deps?.persist ?? localStorageWorkspacePersist()
  const provision = deps?.provision ?? provisionWorkspace
  const selectedId = shallowRef<string | null>(null)
  let primed = false
  const inflight = new Map<string, Promise<WorkspaceSummary>>()

  function storedId(): string | null {
    if (!primed) {
      primed = true
      selectedId.value = persist.readSelectedId()
    }
    return selectedId.value
  }

  function provisionDedupeKey(input: ProvisionWorkspaceInput): string {
    const scope = input.organizationId?.trim() || '-'
    const billing =
      input.isNewBillingAccount === true
        ? 'new'
        : typeof input.billingAccountId === 'string' && input.billingAccountId.trim() !== ''
          ? `ba:${input.billingAccountId.trim()}`
          : 'default'
    return `${scope}:${input.name.trim().toLowerCase()}:${billing}`
  }

  return {
    selectedId,
    getSelectedId: storedId,
    select(id: string): void {
      const valid = asWorkspaceId(id)
      if (!valid) throw new Error('Invalid workspace selection')
      const changed = selectedId.value !== valid
      selectedId.value = valid
      primed = true
      persist.writeSelectedId(valid)
      if (changed) notifyWorkspaceSelectionChange(valid)
    },
    clear(): void {
      const changed = selectedId.value !== null
      selectedId.value = null
      primed = true
      persist.writeSelectedId(null)
      if (changed) notifyWorkspaceSelectionChange(null)
    },
    // Concurrent provisions for the same (name, organization) share one
    // promise and one idempotency key, so double-clicks cannot fork.
    // Creating a workspace does not change the current navigation scope.
    provision(input: ProvisionWorkspaceInput): Promise<WorkspaceSummary> {
      const key = provisionDedupeKey(input)
      const running = inflight.get(key)
      if (running) return running
      const task = provision(input).finally(() => {
        if (inflight.get(key) === task) inflight.delete(key)
      })
      inflight.set(key, task)
      return task
    },
  }
}

export type WorkspaceSelectionStore = ReturnType<typeof createWorkspaceSelectionStore>

let sharedStore: WorkspaceSelectionStore | null = null

/**
 * Single-registration install for the workspace selection store.
 *
 * The first call creates and registers the shared store (honoring the
 * caller's persist/provision overrides); later calls return the already
 * installed store so concurrent feature consumers always share one
 * reactive `selectedId` instead of forking state.
 */
export function installWorkspaceSelectionStore(deps?: {
  persist?: WorkspaceSelectionPersist | undefined
  provision?: WorkspaceProvisionFn | undefined
}): WorkspaceSelectionStore {
  if (!sharedStore) sharedStore = createWorkspaceSelectionStore(deps)
  // IoC registration for `api/mediaAssetDelivery` (which must not
  // import this feature module): resolve implicit workspace ids from the
  // shared singleton. Idempotent; runs on every access.
  setMediaWorkspaceIdResolver(() => sharedStore?.getSelectedId() ?? null)
  return sharedStore
}

export function getWorkspaceSelectionStore(): WorkspaceSelectionStore {
  return installWorkspaceSelectionStore()
}

export function resetWorkspaceSelectionStore(): void {
  sharedStore = null
}

export type UseWorkspaceSelectionOptions = {
  enabled?: MaybeRefOrGetter<boolean> | undefined
  routeWorkspaceId?: MaybeRefOrGetter<string | null | undefined> | undefined
  store?: WorkspaceSelectionStore | undefined
  // Explicit 401 policy override for the list query. When omitted, an
  // anonymous reader (no stored session — public/guest surfaces) fetches
  // silently so a 401 degrades to error/empty state instead of reloading
  // to login; an authed reader keeps the redirect so a revoked session
  // re-authenticates with its destination preserved.
  authFailure?: AuthFailureMode | undefined
}

// Shell-level selection: query owns server data, URL owns nav state, the
// narrow store owns the persisted ID. Covers loading/empty/error/disabled
// plus the hidden(404)-vs-forbidden(403) split per DESIGN.md.
export function useWorkspaceSelection(options?: UseWorkspaceSelectionOptions) {
  const store = options?.store ?? getWorkspaceSelectionStore()
  const route = useRoute()
  const router = useRouter()

  const query = useQuery({
    queryKey: workspaceQueryKeys.list,
    queryFn: ({ signal }) =>
      fetchWorkspacePage(undefined, {
        signal,
        authFailure: options?.authFailure ?? (getStoredUser() == null ? 'silent' : undefined),
      }),
    enabled: () => toValue(options?.enabled ?? true),
    ...accountScoped(),
  })

  const urlId = computed(() =>
    options?.routeWorkspaceId !== undefined
      ? asWorkspaceId(toValue(options.routeWorkspaceId))
      : asWorkspaceId(stringParam(route.query[WORKSPACE_URL_PARAM])),
  )

  // Keep the shared choice sticky across refetches without ever
  // copying server items into the store. Auto derivations stay pure so the
  // Default-first rule re-evaluates deterministically on every load.
  // Persist every resolved id (url, stored, auto-default, auto-first):
  // resource layers read the store directly (`getSelectedId()`), so an
  // unpersisted auto-resolution leaves boards/library fail-closed with
  // "select a workspace" errors on fresh profiles that have no picker UI.
  watch(
    () => query.data.value,
    (page) => {
      if (!page) return
      const resolved = resolveWorkspaceSelection(page.items, {
        urlId: urlId.value,
        storedId: store.getSelectedId(),
      })
      if (resolved.selectedId && resolved.source !== 'none') {
        store.select(resolved.selectedId)
      }
    },
    { immediate: true },
  )

  const items = computed(() => query.data.value?.items ?? [])
  const selection = computed(() =>
    resolveWorkspaceSelection(items.value, {
      urlId: urlId.value,
      storedId: store.getSelectedId(),
    }),
  )
  const selectedId = computed(() => selection.value.selectedId)
  const selected = computed(
    () => items.value.find((item) => item.publicId === selection.value.selectedId) ?? null,
  )
  const status = computed<WorkspaceSelectionStatus>(() =>
    toWorkspaceSelectionStatus({
      enabled: toValue(options?.enabled ?? true),
      isPending: query.isPending.value,
      isError: query.isError.value,
      error: query.error.value,
      itemCount: items.value.length,
    }),
  )

  function selectWorkspace(id: string): void {
    store.select(id)
    const next = store.getSelectedId()
    void router.replace({
      query: {
        ...route.query,
        ...(next == null ? {} : { [WORKSPACE_URL_PARAM]: next }),
      },
    })
  }

  return {
    items,
    selectedId,
    selected,
    selectionSource: computed(() => selection.value.source),
    status,
    isPending: computed(() => query.isPending.value),
    error: computed(() => query.error.value),
    refetch: () => query.refetch(),
    selectWorkspace,
    provisionWorkspace: (input: ProvisionWorkspaceInput) => store.provision(input),
    queryKey: workspaceQueryKeys.list,
  }
}
