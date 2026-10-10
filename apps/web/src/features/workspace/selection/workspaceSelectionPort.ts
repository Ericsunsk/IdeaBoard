// Workspace selection port (P2 `workspace/selection-singleton`).
//
// Installed singleton over the narrow persisted selection store: feature
// consumers (`board`, `search`, `account`, …) obtain the same port/reactive
// `selectedId` through `install`/`get`/`use` and observe each other's
// changes. The port owns only client selection state — it never constructs
// a TanStack Query client (server data stays in the `useWorkspaceSelection`
// query cache under `workspaceQueryKeys.list`).

import type { Ref } from 'vue'
import {
  installWorkspaceSelectionStore,
  memoryWorkspacePersist,
  resetWorkspaceSelectionStore,
  subscribeWorkspaceSelectionChange,
  WORKSPACE_SELECTION_STORAGE_KEY,
  type ProvisionWorkspaceInput,
  type WorkspaceProvisionFn,
  type WorkspaceSelectionPersist,
  type WorkspaceSelectionStore,
} from './workspaceContext'
import type { WorkspaceSummary } from './workspaceResource'

export { WORKSPACE_SELECTION_STORAGE_KEY }

export function memoryWorkspaceSelectionPersist(initial: string | null = null) {
  return memoryWorkspacePersist(initial)
}

export type WorkspaceSelectionPort = {
  readonly selectedId: Ref<string | null>
  getSelectedId: () => string | null
  select: (id: string) => void
  clear: () => void
  provision: (input: ProvisionWorkspaceInput) => Promise<WorkspaceSummary>
  subscribe: (listener: (id: string | null) => void) => () => void
}

function createWorkspaceSelectionPort(store: WorkspaceSelectionStore): WorkspaceSelectionPort {
  store.getSelectedId()
  return {
    selectedId: store.selectedId,
    getSelectedId: () => store.getSelectedId(),
    select: (id: string) => store.select(id),
    clear: () => store.clear(),
    provision: (input: ProvisionWorkspaceInput) => store.provision(input),
    subscribe: (listener: (id: string | null) => void) =>
      subscribeWorkspaceSelectionChange(listener),
  }
}

type InstallWorkspaceSelectionPortOptions = {
  persist?: WorkspaceSelectionPersist | undefined
  provision?: WorkspaceProvisionFn | undefined
}

let installedPort: WorkspaceSelectionPort | null = null
let installedPortStore: WorkspaceSelectionStore | null = null

/**
 * Installs the workspace selection port exactly once.
 *
 * The first call registers the shared store (honoring persist/provision
 * overrides); later calls return the already installed port so every
 * consumer shares one reactive `selectedId`.
 */
export function installWorkspaceSelectionPort(
  options?: InstallWorkspaceSelectionPortOptions,
): WorkspaceSelectionPort {
  const store = installWorkspaceSelectionStore(options)
  if (!installedPort || installedPortStore !== store) {
    installedPort = createWorkspaceSelectionPort(store)
    installedPortStore = store
  }
  return installedPort
}

/** Returns the installed selection port, installing with defaults if needed. */
export function getWorkspaceSelectionPort(): WorkspaceSelectionPort {
  return installWorkspaceSelectionPort()
}

/** Composable alias: returns the same installed selection port. */
export function useWorkspaceSelectionPort(): WorkspaceSelectionPort {
  return installWorkspaceSelectionPort()
}

/** Test-only reset: drops the installed port and its shared store. */
export function resetWorkspaceSelectionPort(): void {
  installedPort = null
  installedPortStore = null
  resetWorkspaceSelectionStore()
}
