// Board sync contract (3) — board focus controls.
// refetchOnWindowFocus stays false; the only focus path is the existing 5s
// throttled explicit visible-invalidate plus the hidden-flush save. All three
// halves already exist in the current tree, so this spec stays active.

import { computed, effectScope, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { queryClient } from '../../../../core/query/queryClient'
import type { BoardRepository } from '../catalog/repository'
import type { BoardMeta } from '../catalog/boards'
import type { BoardPreviewRefreshQueue } from '../preview/boardPreviewRefreshQueue'
import { createGalleryController } from '../../ui/gallery/controller'
import { createBoardAutosavePolicy, useBoardAutosave } from '../../../canvas/session/board-session'

const disposeControllers: (() => void)[] = []

afterEach(() => {
  disposeControllers.splice(0).forEach((dispose) => dispose())
  vi.restoreAllMocks()
})

function makeBoard(id: string, name = id, updatedAt = 1): BoardMeta {
  return { id, name, createdAt: 1, updatedAt }
}

function createHarness() {
  const boards = ref<readonly BoardMeta[]>([makeBoard('board-1')])
  const scope = ref<string | null>('user-1')
  const catalog: BoardRepository = {
    viewModel: {
      scope,
      boards,
      byId: computed(() => new Map(boards.value.map((board) => [board.id, board]))),
    },
    commands: {
      hydrate: vi.fn(async () => ({ isOk: true as const })),
      create: vi.fn(async () => ({ isOk: true as const, board: makeBoard('created') })),
      rename: vi.fn(async () => ({ isOk: true as const, board: makeBoard('board-1') })),
      duplicate: vi.fn(async () => ({ isOk: true as const, board: makeBoard('copy') })),
      delete: vi.fn(async () => ({ isOk: true as const, isRemoteWasMissing: false })),
    },
    dispose: vi.fn(),
  }
  const preview: BoardPreviewRefreshQueue = {
    previewMap: ref({}),
    reloadFromLocal: vi.fn(),
    schedule: vi.fn(),
    invalidate: vi.fn(),
    dispose: vi.fn(),
  }
  const controller = createGalleryController({
    catalog,
    preview,
    navigation: { openBoard: vi.fn() },
    feedback: {
      createFailed: vi.fn(),
      renameValidationFailed: vi.fn(),
      renameSyncFailed: vi.fn(),
      renamed: vi.fn(),
      copyFailed: vi.fn(),
      copied: vi.fn(),
      deleteFailed: vi.fn(),
      deleted: vi.fn(),
      catalogSyncFailed: vi.fn(),
      catalogReloaded: vi.fn(),
    },
    warn: vi.fn(),
  })
  disposeControllers.push(controller.dispose)
  return { catalog, controller }
}

async function flushFocus() {
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
}

describe('board focus controls', () => {
  it('keeps refetchOnWindowFocus disabled globally (explicit invalidate only)', () => {
    expect(queryClient.getDefaultOptions().queries?.refetchOnWindowFocus).toBe(false)
  })

  it('coalesces a visible double-touch into a single explicit invalidate', async () => {
    const harness = createHarness()
    await harness.controller.commands.initialize()
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(1)

    window.dispatchEvent(new Event('focus'))
    await flushFocus()
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(2)

    window.dispatchEvent(new Event('focus'))
    await flushFocus()
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(2)
  })

  it('never fetches while the tab stays hidden', async () => {
    const harness = createHarness()
    await harness.controller.commands.initialize()
    expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(1)

    const original = Object.getOwnPropertyDescriptor(document, 'visibilityState')
    Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true })
    try {
      window.dispatchEvent(new Event('focus'))
      await flushFocus()
      expect(harness.catalog.commands.hydrate).toHaveBeenCalledTimes(1)
    } finally {
      if (original) Object.defineProperty(document, 'visibilityState', original)
      else Reflect.deleteProperty(document, 'visibilityState')
    }
    expect(Object.getOwnPropertyDescriptor(document, 'visibilityState')).toEqual(original)
  })

  it('flushes the pending save when the document hides', async () => {
    let isDirty = true
    const save = vi.fn(async () => true)
    const policy = createBoardAutosavePolicy({
      isDirty: () => isDirty,
      isSaving: () => false,
      markDirty: () => {
        isDirty = true
      },
      save,
    })
    try {
      policy.handleHidden()
      await vi.waitFor(() => expect(save).toHaveBeenCalledExactlyOnceWith({ isSilent: true }))
    } finally {
      policy.dispose()
    }
  })

  it('binds the hidden visibility event to the pending save', async () => {
    const original = Object.getOwnPropertyDescriptor(document, 'visibilityState')
    const scope = effectScope()
    const save = vi.fn(async () => true)
    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true })
    try {
      scope.run(() => useBoardAutosave({ isDirty: ref(true), isSaving: ref(false), save }))
      expect(save).not.toHaveBeenCalled()
      Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true })
      document.dispatchEvent(new Event('visibilitychange'))
      await vi.waitFor(() => expect(save).toHaveBeenCalledExactlyOnceWith({ isSilent: true }))
    } finally {
      scope.stop()
      if (original) Object.defineProperty(document, 'visibilityState', original)
      else Reflect.deleteProperty(document, 'visibilityState')
    }
    expect(Object.getOwnPropertyDescriptor(document, 'visibilityState')).toEqual(original)
  })
})
