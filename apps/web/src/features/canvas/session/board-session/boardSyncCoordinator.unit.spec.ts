import { computed, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import {
  createEmptyBoardSnapshot,
  InvalidBoardStateError,
  type BoardSnapshot,
} from '../../../board'
import type { BoardSession, BoardSessionSaveResult } from './useBoardSession'
import {
  BOARD_SESSION_AUTH_UNAVAILABLE_CODE,
  BOARD_SESSION_WORKSPACE_UNAVAILABLE_CODE,
} from './useBoardSession'
import { createBoardSyncCoordinator, type SaveStatus } from './boardSyncCoordinator'

function makeSnapshot(marker: string): BoardSnapshot {
  const snapshot = createEmptyBoardSnapshot()
  snapshot.promptNodes = [
    {
      id: 1,
      type: 'prompt',
      x: 40,
      y: 40,
      width: 280,
      text: marker,
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
    },
  ]
  return snapshot
}

function createHarness() {
  const routeBoardId = ref('board-a')
  const currentSnapshot = ref(makeSnapshot('current'))
  const isDirty = ref(true)
  const saveStatus = ref<SaveStatus>('idle')
  const isLoadingBoardSnapshot = ref(false)
  const isSavingBoard = ref(false)
  const graphRevision = ref(0)
  const onWarning = vi.fn()
  const onSuccess = vi.fn()
  const onError = vi.fn()
  const onArchived = vi.fn()
  const boardSession: BoardSession = {
    currentBoardId: computed(() => routeBoardId.value),
    exists: vi.fn(() => true),
    load: vi.fn(async () => makeSnapshot('loaded')),
    save: vi.fn(async (snapshot: BoardSnapshot) => ({
      snapshot,
      remoteSync: { isOk: true as const },
    })),
  }
  const coordinator = createBoardSyncCoordinator({
    buildCurrentSnapshot: () => currentSnapshot.value,
    isDirty,
    saveStatus,
    graphRevision,
    isLoadingBoardSnapshot,
    isSavingBoard,
    boardSession,
    readBoardAttachmentEpoch: () => 1,
    canApplyReconciliation: () => true,
    onWarning,
    onSuccess,
    onError,
    onArchived,
  })
  return {
    coordinator,
    boardSession,
    routeBoardId,
    currentSnapshot,
    isDirty,
    saveStatus,
    isLoadingBoardSnapshot,
    isSavingBoard,
    graphRevision,
    onWarning,
    onSuccess,
    onError,
    onArchived,
  }
}

describe('Board Sync Coordinator', () => {
  it('returns a loaded snapshot without applying renderer state', async () => {
    const harness = createHarness()
    const result = await harness.coordinator.loadInitialOrRouteBoard()

    expect(result).toEqual({ status: 'loaded', snapshot: expect.any(Object) })
    expect(harness.boardSession.load).toHaveBeenCalledWith('board-a')
    expect(harness.isDirty.value).toBe(false)
    // The loading flag is still set while the snapshot is applied, so the
    // real canSaveBoard predicate reports false here — same as production.
    expect(harness.saveStatus.value).toBe('idle')
  })

  it('gates saving on board identity, existence, and load state', () => {
    const harness = createHarness()
    expect(harness.coordinator.canSaveBoard()).toBe(true)

    harness.routeBoardId.value = ''
    expect(harness.coordinator.canSaveBoard()).toBe(false)
    harness.routeBoardId.value = 'board-a'

    vi.mocked(harness.boardSession.exists).mockReturnValueOnce(false)
    expect(harness.coordinator.canSaveBoard()).toBe(false)

    harness.isLoadingBoardSnapshot.value = true
    expect(harness.coordinator.canSaveBoard()).toBe(false)
    harness.isLoadingBoardSnapshot.value = false
    expect(harness.coordinator.canSaveBoard()).toBe(true)
  })

  it('returns an empty snapshot for a route without a board', async () => {
    const harness = createHarness()
    harness.routeBoardId.value = ''

    const result = await harness.coordinator.loadInitialOrRouteBoard()

    expect(result.status).toBe('empty')
    expect(harness.boardSession.load).not.toHaveBeenCalled()
    expect(harness.saveStatus.value).toBe('idle')
  })

  it('returns not-found for an unknown route board ID', async () => {
    const harness = createHarness()
    vi.mocked(harness.boardSession.load).mockResolvedValueOnce(null)
    vi.mocked(harness.boardSession.exists).mockReturnValueOnce(false)

    const result = await harness.coordinator.loadInitialOrRouteBoard()

    expect(result).toEqual({ status: 'not-found' })
    expect(harness.saveStatus.value).toBe('idle')
  })

  it('returns a save reconciliation for the graph runtime to apply', async () => {
    const harness = createHarness()
    const result = await harness.coordinator.saveCurrentBoardSnapshot({ isSilent: false })

    expect(result.isSaved).toBe(true)
    expect(result.reconciliation).toEqual({
      snapshot: expect.any(Object),
      hasLocalDraftAdvanced: false,
    })
    expect(harness.onSuccess).toHaveBeenCalledWith('Saved')
  })

  it('keeps dirty state and returns reconciliation when remote sync fails', async () => {
    const harness = createHarness()
    vi.mocked(harness.boardSession.save).mockResolvedValueOnce({
      snapshot: makeSnapshot('saved-locally'),
      remoteSync: { isOk: false, reason: 'sync-failed', error: new Error('offline') },
    })

    const result = await harness.coordinator.saveCurrentBoardSnapshot()

    expect(result.isSaved).toBe(false)
    expect(result.reconciliation?.snapshot.promptNodes[0]?.text).toBe('saved-locally')
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('retry')
    expect(harness.onWarning).toHaveBeenCalledWith(
      'Saved locally. Remote sync failed; it will retry once the network recovers.',
    )
  })

  it('names the server status when remote sync fails with HTTP 5xx', async () => {
    const harness = createHarness()
    const failure = Object.assign(new Error('boom'), { status: 500 })
    vi.mocked(harness.boardSession.save).mockResolvedValueOnce({
      snapshot: makeSnapshot('saved-locally'),
      remoteSync: { isOk: false, reason: 'sync-failed', error: failure },
    })

    await harness.coordinator.saveCurrentBoardSnapshot()

    expect(harness.onWarning).toHaveBeenCalledWith(
      'Saved locally. Remote sync failed (HTTP 500); it will retry on your next edit.',
    )
  })

  it('surfaces validation failures without the network-recovery copy', async () => {
    const harness = createHarness()
    const failure = Object.assign(new Error('bad'), { status: 400 })
    vi.mocked(harness.boardSession.save).mockResolvedValueOnce({
      snapshot: makeSnapshot('saved-locally'),
      remoteSync: { isOk: false, reason: 'sync-failed', error: failure },
    })

    await harness.coordinator.saveCurrentBoardSnapshot()

    expect(harness.onError).toHaveBeenCalledWith(
      'Canvas data validation failed. Your current canvas was kept. Please try again later.',
    )
  })

  it('surfaces auth failures without the network-recovery copy', async () => {
    const harness = createHarness()
    const failure = Object.assign(new Error('denied'), { status: 403 })
    vi.mocked(harness.boardSession.save).mockResolvedValueOnce({
      snapshot: makeSnapshot('saved-locally'),
      remoteSync: { isOk: false, reason: 'sync-failed', error: failure },
    })

    await harness.coordinator.saveCurrentBoardSnapshot()

    expect(harness.onError).toHaveBeenCalledWith(
      'Save failed: user session is missing. Please sign in and try again.',
    )
  })

  it('routes auth-unavailable codes to the session copy instead of the network copy', async () => {
    const harness = createHarness()
    const infoSpy = vi.spyOn(console, 'info').mockImplementation(() => {})
    try {
      const failure = Object.assign(new Error('missing session'), {
        code: BOARD_SESSION_AUTH_UNAVAILABLE_CODE,
      })
      vi.mocked(harness.boardSession.save).mockResolvedValueOnce({
        snapshot: makeSnapshot('saved-locally'),
        remoteSync: { isOk: false, reason: 'sync-failed', error: failure },
      })

      await harness.coordinator.saveCurrentBoardSnapshot()

      expect(harness.onError).toHaveBeenCalledWith(
        'Save failed: user session is missing. Please sign in and try again.',
      )
      expect(harness.onWarning).not.toHaveBeenCalled()
      expect(infoSpy).toHaveBeenCalledWith('[canvas] save skipped remote sync: auth unavailable')
    } finally {
      infoSpy.mockRestore()
    }
  })

  it('routes workspace-unavailable codes to the workspace copy instead of the network copy', async () => {
    const harness = createHarness()
    const infoSpy = vi.spyOn(console, 'info').mockImplementation(() => {})
    try {
      const failure = Object.assign(new Error('no workspace'), {
        code: BOARD_SESSION_WORKSPACE_UNAVAILABLE_CODE,
      })
      vi.mocked(harness.boardSession.save).mockResolvedValueOnce({
        snapshot: makeSnapshot('saved-locally'),
        remoteSync: { isOk: false, reason: 'sync-failed', error: failure },
      })

      await harness.coordinator.saveCurrentBoardSnapshot()

      expect(harness.onError).toHaveBeenCalledWith(
        'No workspace selected. Saved locally; select a workspace and try again.',
      )
      expect(harness.onWarning).not.toHaveBeenCalled()
      expect(infoSpy).toHaveBeenCalledWith(
        '[canvas] save skipped remote sync: workspace unavailable',
      )
    } finally {
      infoSpy.mockRestore()
    }
  })

  it('routes untagged workspace-missing errors to the workspace copy via the cause chain', async () => {
    const harness = createHarness()
    const infoSpy = vi.spyOn(console, 'info').mockImplementation(() => {})
    try {
      const workspaceCause = Object.assign(new Error('no workspace selected'), {
        name: 'BoardWorkspaceUnavailableError',
      })
      const failure = new Error('BOARD_REMOTE_SYNC_FAILED', { cause: workspaceCause })
      vi.mocked(harness.boardSession.save).mockResolvedValueOnce({
        snapshot: makeSnapshot('saved-locally'),
        remoteSync: { isOk: false, reason: 'sync-failed', error: failure },
      })

      await harness.coordinator.saveCurrentBoardSnapshot()

      expect(harness.onError).toHaveBeenCalledWith(
        'No workspace selected. Saved locally; select a workspace and try again.',
      )
      expect(harness.onWarning).not.toHaveBeenCalled()
    } finally {
      infoSpy.mockRestore()
    }
  })

  it('keeps the network-recovery copy for status-less errors without a machine code', async () => {
    const harness = createHarness()
    const infoSpy = vi.spyOn(console, 'info').mockImplementation(() => {})
    try {
      vi.mocked(harness.boardSession.save).mockResolvedValueOnce({
        snapshot: makeSnapshot('saved-locally'),
        remoteSync: { isOk: false, reason: 'sync-failed', error: new Error('fetch timeout') },
      })

      await harness.coordinator.saveCurrentBoardSnapshot()

      expect(harness.onWarning).toHaveBeenCalledWith(
        'Saved locally. Remote sync failed; it will retry once the network recovers.',
      )
      expect(harness.onError).not.toHaveBeenCalled()
      expect(infoSpy).not.toHaveBeenCalled()
    } finally {
      infoSpy.mockRestore()
    }
  })

  it('does not return a stale reconciliation after a route switch', async () => {
    const harness = createHarness()
    let resolveSave!: (result: BoardSessionSaveResult) => void
    vi.mocked(harness.boardSession.save).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveSave = resolve
        }),
    )

    const save = harness.coordinator.saveCurrentBoardSnapshot()
    await Promise.resolve()
    harness.routeBoardId.value = 'board-b'
    resolveSave({ snapshot: makeSnapshot('stale'), remoteSync: { isOk: true } })

    const result = await save
    expect(result.isSaved).toBe(true)
    expect(result.reconciliation).toBeUndefined()
  })

  it('keeps the current graph revision while rebasing stabilized media', async () => {
    const harness = createHarness()
    const captured = makeSnapshot('current')
    captured.imageNodes = [
      {
        id: 1,
        type: 'image-node',
        x: 0,
        y: 0,
        width: 280,
        sourceImageUrl: 'blob:source',
        sourceImageRatio: 1,
        outputStatus: 'ready',
        outputImages: [],
      },
    ]
    harness.currentSnapshot.value = captured
    let saveOwnedSnapshot!: BoardSnapshot
    let resolveSave!: (result: BoardSessionSaveResult) => void
    vi.mocked(harness.boardSession.save).mockImplementationOnce((snapshot) => {
      saveOwnedSnapshot = snapshot
      return new Promise((resolve) => {
        resolveSave = resolve
      })
    })

    const save = harness.coordinator.saveCurrentBoardSnapshot()
    await Promise.resolve()
    const advanced = structuredClone(captured)
    advanced.promptNodes[0]!.text = 'newer edit'
    harness.currentSnapshot.value = advanced
    harness.graphRevision.value += 1
    saveOwnedSnapshot.imageNodes[0]!.sourceImageUrl = '/uploads/stable.png'
    saveOwnedSnapshot.imageNodes[0]!.sourceMediaAssetId = 42
    resolveSave({ snapshot: saveOwnedSnapshot, remoteSync: { isOk: true } })

    const result = await save
    expect(result.reconciliation?.hasLocalDraftAdvanced).toBe(true)
    expect(result.reconciliation?.snapshot.promptNodes[0]?.text).toBe('newer edit')
    expect(result.reconciliation?.snapshot.imageNodes[0]?.sourceImageUrl).toBe(
      '/uploads/stable.png',
    )
    expect(harness.isDirty.value).toBe(true)
  })

  it('reports validation failures without replacing local state', async () => {
    const harness = createHarness()
    vi.mocked(harness.boardSession.load).mockRejectedValueOnce(
      new InvalidBoardStateError({
        code: 'INVALID_BOARD_STATE',
        boundary: 'local-state-read',
        isRecoverable: true,
        issues: [{ path: 'promptNodes.0.text', message: 'Invalid text' }],
      }),
    )

    const result = await harness.coordinator.loadInitialOrRouteBoard()
    expect(result).toEqual({ status: 'failed' })
    expect(harness.onError).toHaveBeenCalledWith(
      'Canvas data validation failed. Your current canvas was kept. Please try again later.',
    )
  })

  it('maps 409 board_archived to the archived banner instead of the generic local-save toast', async () => {
    const harness = createHarness()
    const archivedError = Object.assign(new Error('board is archived'), {
      status: 409,
      code: 'board_archived',
    })
    vi.mocked(harness.boardSession.save).mockResolvedValueOnce({
      snapshot: makeSnapshot('saved-locally'),
      remoteSync: { isOk: false, reason: 'archived', error: archivedError },
    })

    const result = await harness.coordinator.saveCurrentBoardSnapshot()

    expect(result.isSaved).toBe(false)
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('archived')
    // Banner signal, never the generic "Saved locally..." toast.
    expect(harness.onArchived).toHaveBeenCalledWith('board-a')
    expect(harness.onWarning).not.toHaveBeenCalled()
    expect(harness.onError).not.toHaveBeenCalled()
  })

  it('dedupes repeated silent archived saves without re-notifying', async () => {
    const harness = createHarness()
    const archivedError = Object.assign(new Error('board is archived'), {
      status: 409,
      code: 'board_archived',
    })
    vi.mocked(harness.boardSession.save).mockResolvedValue({
      snapshot: makeSnapshot('saved-locally'),
      remoteSync: { isOk: false, reason: 'archived', error: archivedError },
    })

    await harness.coordinator.saveCurrentBoardSnapshot({ isSilent: true })
    await harness.coordinator.saveCurrentBoardSnapshot({ isSilent: true })

    expect(harness.onArchived).toHaveBeenCalledTimes(1)
  })

  it('falls back to an explicit error toast for archived saves without a banner handler', async () => {
    const harness = createHarness()
    const coordinatorWithoutBanner = createBoardSyncCoordinator({
      buildCurrentSnapshot: () => harness.currentSnapshot.value,
      isDirty: harness.isDirty,
      saveStatus: harness.saveStatus,
      graphRevision: harness.graphRevision,
      isLoadingBoardSnapshot: harness.isLoadingBoardSnapshot,
      isSavingBoard: harness.isSavingBoard,
      boardSession: harness.boardSession,
      readBoardAttachmentEpoch: () => 1,
      canApplyReconciliation: () => true,
      onWarning: harness.onWarning,
      onSuccess: harness.onSuccess,
      onError: harness.onError,
    })
    vi.mocked(harness.boardSession.save).mockResolvedValueOnce({
      snapshot: makeSnapshot('saved-locally'),
      remoteSync: {
        isOk: false,
        reason: 'archived',
        error: Object.assign(new Error('board is archived'), {
          status: 409,
          code: 'board_archived',
        }),
      },
    })

    await coordinatorWithoutBanner.saveCurrentBoardSnapshot()

    expect(harness.onError).toHaveBeenCalledWith(
      'This board is archived. Restore it to keep editing.',
    )
  })
})
