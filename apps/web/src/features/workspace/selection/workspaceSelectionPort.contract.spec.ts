// Workspace selection singleton contract (P2 `workspace/selection-singleton`).
//
// Two feature consumers obtain the same installed selection port/reactive
// state; changing one is observed by the other without constructing another
// Query client. Written BEFORE the port migration: red while no such port
// exists under `selection/`.
import { beforeEach, describe, expect, it, vi } from 'vitest'

const queryMocks = vi.hoisted(() => ({ useQuery: vi.fn(), constructQueryClient: vi.fn() }))

vi.mock('@tanstack/vue-query', async (importOriginal) => {
  const original = await importOriginal<typeof import('@tanstack/vue-query')>()
  return {
    ...original,
    useQuery: queryMocks.useQuery,
    QueryClient: new Proxy(original.QueryClient, {
      construct(target, args, newTarget) {
        queryMocks.constructQueryClient()
        return Reflect.construct(target, args, newTarget)
      },
    }),
  }
})

import { getWorkspaceSelectionStore, subscribeWorkspaceSelectionChange } from './workspaceContext'

import {
  getWorkspaceSelectionPort,
  installWorkspaceSelectionPort,
  memoryWorkspaceSelectionPersist,
  resetWorkspaceSelectionPort,
  useWorkspaceSelectionPort,
  WORKSPACE_SELECTION_STORAGE_KEY,
} from './workspaceSelectionPort'

beforeEach(() => {
  queryMocks.useQuery.mockClear()
  queryMocks.constructQueryClient.mockClear()
  resetWorkspaceSelectionPort()
})

describe('workspace selection singleton port', () => {
  it('installs once: every consumer obtains the same port instance', () => {
    const installed = installWorkspaceSelectionPort({
      persist: memoryWorkspaceSelectionPersist(),
    })
    const viaGetter = getWorkspaceSelectionPort()
    const consumerA = useWorkspaceSelectionPort()
    const consumerB = useWorkspaceSelectionPort()

    expect(viaGetter).toBe(installed)
    expect(consumerA).toBe(installed)
    expect(consumerB).toBe(installed)
    expect(consumerA.selectedId).toBe(installed.selectedId)
    expect(consumerB.selectedId).toBe(consumerA.selectedId)
    expect(getWorkspaceSelectionStore().selectedId).toBe(installed.selectedId)
  })

  it('observes a change made through one consumer on the other', () => {
    installWorkspaceSelectionPort({ persist: memoryWorkspaceSelectionPersist() })
    const consumerA = useWorkspaceSelectionPort()
    const consumerB = getWorkspaceSelectionPort()

    consumerA.select('ws_team2')

    expect(consumerB.selectedId.value).toBe('ws_team2')
    expect(consumerB.getSelectedId()).toBe('ws_team2')

    consumerB.clear()

    expect(consumerA.selectedId.value).toBeNull()
    expect(consumerA.getSelectedId()).toBeNull()
  })

  it('keeps single registration when install runs again', () => {
    const first = installWorkspaceSelectionPort({
      persist: memoryWorkspaceSelectionPersist(),
    })
    first.select('ws_personal1')

    const second = installWorkspaceSelectionPort({
      persist: memoryWorkspaceSelectionPersist(),
    })

    expect(second).toBe(first)
    expect(second.getSelectedId()).toBe('ws_personal1')
  })

  it('restores the persisted selection into reactive state after reload', () => {
    const key = WORKSPACE_SELECTION_STORAGE_KEY
    const previous = localStorage.getItem(key)
    try {
      localStorage.removeItem(key)
      installWorkspaceSelectionPort().select('ws_team2')
      expect(localStorage.getItem(key)).toBe('ws_team2')

      resetWorkspaceSelectionPort()

      expect(installWorkspaceSelectionPort().selectedId.value).toBe('ws_team2')
      expect(getWorkspaceSelectionPort().getSelectedId()).toBe('ws_team2')
    } finally {
      if (previous === null) localStorage.removeItem(key)
      else localStorage.setItem(key, previous)
    }
  })

  it('hydrates selectedId from persisted state during first port installation', () => {
    const persist = {
      readSelectedId: vi.fn(() => 'ws_team2'),
      writeSelectedId: vi.fn(),
    }
    const listener = vi.fn()

    const unsubscribe = subscribeWorkspaceSelectionChange(listener)
    try {
      const port = installWorkspaceSelectionPort({ persist })

      expect(port.selectedId.value).toBe('ws_team2')
      expect(port.getSelectedId()).toBe('ws_team2')
      expect(persist.readSelectedId).toHaveBeenCalledTimes(1)
      expect(persist.writeSelectedId).not.toHaveBeenCalled()
      expect(listener).not.toHaveBeenCalled()
    } finally {
      unsubscribe()
    }
  })

  it('constructs no Query client while obtaining or mutating the port', () => {
    installWorkspaceSelectionPort({ persist: memoryWorkspaceSelectionPersist() })
    const consumerA = useWorkspaceSelectionPort()
    const consumerB = getWorkspaceSelectionPort()

    consumerA.select('ws_team2')
    expect(consumerB.selectedId.value).toBe('ws_team2')
    consumerB.clear()

    expect(queryMocks.useQuery).not.toHaveBeenCalled()
    expect(queryMocks.constructQueryClient).not.toHaveBeenCalled()
  })
})
