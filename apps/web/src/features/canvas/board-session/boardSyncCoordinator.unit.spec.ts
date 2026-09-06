import { computed, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import {
  createEmptyCanvasBoardSnapshot,
  InvalidBoardStateError,
  type CanvasBoardSnapshot,
} from '../../canvas-board/domain'
import type { CanvasBoardSession, CanvasBoardSessionSaveResult } from './useCanvasBoardSession'
import { createBoardSyncCoordinator, type SaveStatus } from './boardSyncCoordinator'

function makeSnapshot(marker: string): CanvasBoardSnapshot {
  const snapshot = createEmptyCanvasBoardSnapshot()
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
  const boardSession: CanvasBoardSession = {
    currentBoardId: computed(() => routeBoardId.value),
    exists: vi.fn(() => true),
    load: vi.fn(async () => makeSnapshot('loaded')),
    save: vi.fn(async (snapshot: CanvasBoardSnapshot) => ({
      snapshot,
      remoteSync: { ok: true as const },
    })),
    refreshPreview: vi.fn(),
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
    canSaveBoard: () => routeBoardId.value.length > 0,
    canApplyReconciliation: () => true,
    onWarning,
    onSuccess,
    onError,
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
  }
}

describe('Board Sync Coordinator', () => {
  it('returns a loaded snapshot without applying renderer state', async () => {
    const harness = createHarness()
    const result = await harness.coordinator.loadInitialOrRouteBoard()

    expect(result).toEqual({ status: 'loaded', snapshot: expect.any(Object) })
    expect(harness.boardSession.load).toHaveBeenCalledWith('board-a')
    expect(harness.isDirty.value).toBe(false)
    expect(harness.saveStatus.value).toBe('saved')
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
    const result = await harness.coordinator.saveCurrentBoardSnapshot({ silent: false })

    expect(result.saved).toBe(true)
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
      remoteSync: { ok: false, reason: 'sync-failed', error: new Error('offline') },
    })

    const result = await harness.coordinator.saveCurrentBoardSnapshot()

    expect(result.saved).toBe(false)
    expect(result.reconciliation?.snapshot.promptNodes[0]?.text).toBe('saved-locally')
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('retry')
    expect(harness.onWarning).toHaveBeenCalledWith(
      'Saved locally. Remote sync failed; it will retry once the network recovers.',
    )
  })

  it('does not return a stale reconciliation after a route switch', async () => {
    const harness = createHarness()
    let resolveSave!: (result: CanvasBoardSessionSaveResult) => void
    vi.mocked(harness.boardSession.save).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveSave = resolve
        }),
    )

    const save = harness.coordinator.saveCurrentBoardSnapshot()
    await Promise.resolve()
    harness.routeBoardId.value = 'board-b'
    resolveSave({ snapshot: makeSnapshot('stale'), remoteSync: { ok: true } })

    const result = await save
    expect(result.saved).toBe(true)
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
    let resolveSave!: (result: CanvasBoardSessionSaveResult) => void
    vi.mocked(harness.boardSession.save).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveSave = resolve
        }),
    )

    const save = harness.coordinator.saveCurrentBoardSnapshot()
    await Promise.resolve()
    const advanced = structuredClone(captured)
    advanced.promptNodes[0]!.text = 'newer edit'
    harness.currentSnapshot.value = advanced
    harness.graphRevision.value += 1
    const stabilized = structuredClone(captured)
    stabilized.imageNodes[0]!.sourceImageUrl = '/uploads/stable.png'
    stabilized.imageNodes[0]!.sourceMediaAssetId = 42
    resolveSave({ snapshot: stabilized, remoteSync: { ok: true } })

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
        recoverable: true,
        issues: [{ path: 'promptNodes.0.text', message: 'Invalid text' }],
      }),
    )

    const result = await harness.coordinator.loadInitialOrRouteBoard()
    expect(result).toEqual({ status: 'failed' })
    expect(harness.onError).toHaveBeenCalledWith(
      'Canvas data validation failed. Your current canvas was kept. Please try again later.',
    )
  })
})
