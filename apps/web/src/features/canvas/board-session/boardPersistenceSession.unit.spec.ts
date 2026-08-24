import { computed, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import {
  createEmptyCanvasBoardSnapshot,
  InvalidBoardStateError,
  type CanvasBoardSnapshot,
  type ImageNodeItem,
} from '../../canvas-board/domain'
import type { FlowAdapterNode } from '../utils/flowSnapshotAdapter'
import type { CanvasBoardSession, CanvasBoardSessionSaveResult } from './useCanvasBoardSession'
import { createBoardPersistenceSession, type SaveStatus } from './boardPersistenceSession'

function makePromptSnapshot(id = 1) {
  return {
    id,
    type: 'prompt' as const,
    x: 40,
    y: 40,
    width: 280,
    text: `prompt ${id}`,
    outputCount: 1,
    aspectRatio: 'auto' as const,
    inputImages: [],
  }
}

function makePendingImageFlowNode(): FlowAdapterNode {
  const snapshotNode: ImageNodeItem = {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    outputStatus: 'running',
    outputImages: [],
  }
  return {
    id: 'image-1',
    type: 'canvas',
    position: { x: 0, y: 0 },
    data: {
      kind: 'image',
      nodeType: 'image',
      title: 'Image',
      snapshotNode,
    },
  }
}

function createSessionHarness(initialBoardId = 'board-a') {
  const routeBoardId = ref(initialBoardId)
  const fullNodes = ref<unknown[]>([])
  const fullEdges = ref<unknown[]>([])
  const boardBaseSnapshot = ref(createEmptyCanvasBoardSnapshot())
  const isDirty = ref(true)
  const saveStatus = ref<SaveStatus>('idle')
  const localEditVersion = ref(0)
  const isLoadingBoardSnapshot = ref(false)
  const isSavingBoard = ref(false)
  const canSaveBoard = ref(true)
  const canReconcileGraph = ref(true)
  const boardAttachmentEpoch = ref(1)
  const commitGraph = vi.fn((nextGraph: { nodes: unknown[]; edges: unknown[] }) => {
    fullNodes.value = nextGraph.nodes
    fullEdges.value = nextGraph.edges
  })
  const onWarning = vi.fn()
  const onSuccess = vi.fn()
  const onError = vi.fn()
  const clearFlowHistoryBaseline = vi.fn()
  const replaceGraphSnapshot = vi.fn((snapshot: CanvasBoardSnapshot) => {
    boardBaseSnapshot.value = snapshot
  })
  const exportGraphSnapshot = vi.fn(() => boardBaseSnapshot.value)
  const boardSession: CanvasBoardSession = {
    currentBoardId: computed(() => {
      const boardId = routeBoardId.value.trim()
      return boardId || null
    }),
    exists: vi.fn((boardId?: string | null) => !!boardId),
    load: vi.fn(async () => createEmptyCanvasBoardSnapshot()),
    save: vi.fn(async (snapshot: CanvasBoardSnapshot) => ({
      snapshot,
      remoteSync: { ok: true as const },
    })),
    refreshPreview: vi.fn(),
  }
  const session = createBoardPersistenceSession({
    fullNodes,
    fullEdges,
    boardBaseSnapshot,
    isDirty,
    saveStatus,
    graphRevision: localEditVersion,
    isLoadingBoardSnapshot,
    isSavingBoard,
    boardSession,
    readBoardAttachmentEpoch: () => boardAttachmentEpoch.value,
    canSaveBoard: () => canSaveBoard.value,
    onWarning,
    onSuccess,
    onError,
    clearFlowHistoryBaseline,
    canReconcileGraph: () => canReconcileGraph.value,
    replaceGraphSnapshot,
    exportGraphSnapshot,
    commitGraph,
  })

  return {
    session,
    routeBoardId,
    fullNodes,
    fullEdges,
    boardBaseSnapshot,
    isDirty,
    saveStatus,
    localEditVersion,
    isLoadingBoardSnapshot,
    isSavingBoard,
    canSaveBoard,
    canReconcileGraph,
    commitGraph,
    onWarning,
    onSuccess,
    onError,
    clearFlowHistoryBaseline,
    replaceGraphSnapshot,
    exportGraphSnapshot,
    boardSession,
    boardAttachmentEpoch,
  }
}

describe('boardPersistenceSession', () => {
  it('loads a board snapshot, applies the graph, and clears dirty state', async () => {
    const harness = createSessionHarness()
    const snapshot = createEmptyCanvasBoardSnapshot()
    snapshot.promptNodes = [makePromptSnapshot()]
    vi.mocked(harness.boardSession.load).mockResolvedValueOnce(snapshot)

    await harness.session.loadInitialOrRouteBoard()
    await Promise.resolve()

    expect(harness.boardSession.load).toHaveBeenCalledWith('board-a')
    expect(harness.commitGraph).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: expect.any(Array),
        edges: expect.any(Array),
      }),
      { source: 'hydrate-replace' },
    )
    expect(harness.boardBaseSnapshot.value).toMatchObject({
      promptNodes: snapshot.promptNodes,
    })
    expect(harness.isDirty.value).toBe(false)
    expect(harness.saveStatus.value).toBe('saved')
    expect(harness.clearFlowHistoryBaseline).toHaveBeenCalledOnce()
    expect(harness.isLoadingBoardSnapshot.value).toBe(false)
  })

  it('loads an empty snapshot without calling the repository when no board is selected', async () => {
    const harness = createSessionHarness('')
    harness.canSaveBoard.value = false

    await harness.session.loadInitialOrRouteBoard()

    expect(harness.boardSession.load).not.toHaveBeenCalled()
    expect(harness.commitGraph).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: expect.any(Array),
        edges: expect.any(Array),
      }),
      { source: 'hydrate-replace' },
    )
    expect(harness.isDirty.value).toBe(false)
    expect(harness.saveStatus.value).toBe('idle')
  })

  it('keeps the current graph when a selected board snapshot fails validation during load', async () => {
    const harness = createSessionHarness()
    harness.fullNodes.value = [{ id: 'existing-node' }]
    harness.fullEdges.value = [{ id: 'existing-edge' }]
    harness.saveStatus.value = 'saved'
    harness.isDirty.value = true
    vi.mocked(harness.boardSession.load).mockRejectedValueOnce(
      new InvalidBoardStateError({
        code: 'INVALID_BOARD_STATE',
        boundary: 'local-state-read',
        recoverable: true,
        issues: [{ path: 'imageNodes.0.sourceImageRatio', message: 'Invalid ratio' }],
      }),
    )

    await harness.session.loadInitialOrRouteBoard()

    expect(harness.commitGraph).not.toHaveBeenCalled()
    expect(harness.fullNodes.value).toEqual([{ id: 'existing-node' }])
    expect(harness.fullEdges.value).toEqual([{ id: 'existing-edge' }])
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('saved')
    expect(harness.onError).toHaveBeenCalledWith('画布数据校验失败，已保留当前画布，请稍后重试')
    expect(harness.isLoadingBoardSnapshot.value).toBe(false)
  })

  it('delegates runtime overlay preservation to save reconciliation', async () => {
    const harness = createSessionHarness()
    harness.fullNodes.value = [makePendingImageFlowNode()]

    const saved = await harness.session.saveCurrentBoardSnapshot()

    expect(saved).toBe(true)
    expect(harness.boardSession.save).toHaveBeenCalledOnce()
    expect(harness.commitGraph).toHaveBeenCalledWith(
      expect.objectContaining({ nodes: expect.any(Array), edges: expect.any(Array) }),
      { source: 'save-reconcile' },
    )
    expect(harness.clearFlowHistoryBaseline).toHaveBeenCalledOnce()
    expect(harness.isDirty.value).toBe(false)
    expect(harness.saveStatus.value).toBe('saved')
    expect(harness.onSuccess).toHaveBeenCalledWith('已保存')
    expect(harness.isSavingBoard.value).toBe(false)
  })

  it('keeps the board dirty and failed when remote sync fails after local save', async () => {
    const harness = createSessionHarness()
    const savedSnapshot = createEmptyCanvasBoardSnapshot()
    savedSnapshot.promptNodes = [makePromptSnapshot(2)]
    vi.mocked(harness.boardSession.save).mockResolvedValueOnce({
      snapshot: savedSnapshot,
      remoteSync: {
        ok: false,
        reason: 'sync-failed',
        error: new Error('auth unavailable'),
      },
    })

    const saved = await harness.session.saveCurrentBoardSnapshot()

    expect(saved).toBe(false)
    expect(harness.boardBaseSnapshot.value).toEqual(savedSnapshot)
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('retry')
    expect(harness.clearFlowHistoryBaseline).not.toHaveBeenCalled()
    expect(harness.onWarning).toHaveBeenCalledWith('已保存到本地，远端同步失败，将在网络恢复后重试')
    expect(harness.onSuccess).not.toHaveBeenCalled()
    expect(harness.isSavingBoard.value).toBe(false)
  })

  it('warns when a silent autosave cannot sync remotely after the local save', async () => {
    const harness = createSessionHarness()
    vi.mocked(harness.boardSession.save).mockResolvedValueOnce({
      snapshot: createEmptyCanvasBoardSnapshot(),
      remoteSync: {
        ok: false,
        reason: 'sync-failed',
        error: new Error('auth unavailable'),
      },
    })

    const saved = await harness.session.saveCurrentBoardSnapshot({ silent: true })

    expect(saved).toBe(false)
    expect(harness.onWarning).toHaveBeenCalledWith('已保存到本地，远端同步失败，将在网络恢复后重试')
    expect(harness.onSuccess).not.toHaveBeenCalled()
  })

  it('suppresses repeated silent autosave warnings for the same remote sync failure state', async () => {
    const harness = createSessionHarness()
    vi.mocked(harness.boardSession.save).mockResolvedValue({
      snapshot: createEmptyCanvasBoardSnapshot(),
      remoteSync: {
        ok: false,
        reason: 'sync-failed',
        error: new Error('auth unavailable'),
      },
    })

    await harness.session.saveCurrentBoardSnapshot({ silent: true })
    await harness.session.saveCurrentBoardSnapshot({ silent: true })

    expect(harness.saveStatus.value).toBe('retry')
    expect(harness.onWarning).toHaveBeenCalledOnce()
  })

  it('notifies the same silent remote sync failure after switching boards', async () => {
    const harness = createSessionHarness('board-a')
    vi.mocked(harness.boardSession.save).mockResolvedValue({
      snapshot: createEmptyCanvasBoardSnapshot(),
      remoteSync: {
        ok: false,
        reason: 'sync-failed',
        error: new Error('auth unavailable'),
      },
    })

    await harness.session.saveCurrentBoardSnapshot({ silent: true })
    harness.routeBoardId.value = 'board-b'
    await harness.session.saveCurrentBoardSnapshot({ silent: true })

    expect(harness.onWarning).toHaveBeenCalledTimes(2)
  })

  it('notifies repeated non-silent remote sync failures', async () => {
    const harness = createSessionHarness()
    vi.mocked(harness.boardSession.save).mockResolvedValue({
      snapshot: createEmptyCanvasBoardSnapshot(),
      remoteSync: {
        ok: false,
        reason: 'sync-failed',
        error: new Error('auth unavailable'),
      },
    })

    await harness.session.saveCurrentBoardSnapshot({ silent: false })
    await harness.session.saveCurrentBoardSnapshot({ silent: false })

    expect(harness.onWarning).toHaveBeenCalledTimes(2)
  })

  it('notifies when a silent autosave failure changes from retry to conflict', async () => {
    const harness = createSessionHarness()
    vi.mocked(harness.boardSession.save)
      .mockResolvedValueOnce({
        snapshot: createEmptyCanvasBoardSnapshot(),
        remoteSync: {
          ok: false,
          reason: 'sync-failed',
          error: new Error('auth unavailable'),
        },
      })
      .mockResolvedValueOnce({
        snapshot: createEmptyCanvasBoardSnapshot(),
        remoteSync: {
          ok: false,
          reason: 'conflict',
          error: new Error('name exists'),
        },
      })

    await harness.session.saveCurrentBoardSnapshot({ silent: true })
    await harness.session.saveCurrentBoardSnapshot({ silent: true })

    expect(harness.saveStatus.value).toBe('conflict')
    expect(harness.onWarning).toHaveBeenCalledOnce()
    expect(harness.onError).toHaveBeenCalledWith('已保存到本地，但画布名称已存在，请更换后再试')
  })

  it('reports a general silent save failure after a remote sync warning', async () => {
    const harness = createSessionHarness()
    vi.mocked(harness.boardSession.save)
      .mockResolvedValueOnce({
        snapshot: createEmptyCanvasBoardSnapshot(),
        remoteSync: {
          ok: false,
          reason: 'sync-failed',
          error: new Error('auth unavailable'),
        },
      })
      .mockRejectedValueOnce(new Error('保存服务不可用'))

    await harness.session.saveCurrentBoardSnapshot({ silent: true })
    await harness.session.saveCurrentBoardSnapshot({ silent: true })

    expect(harness.onWarning).toHaveBeenCalledOnce()
    expect(harness.onError).toHaveBeenCalledWith('保存服务不可用')
  })

  it('reports a remote sync warning after a general silent save failure', async () => {
    const harness = createSessionHarness()
    vi.mocked(harness.boardSession.save)
      .mockRejectedValueOnce(new Error('保存服务不可用'))
      .mockResolvedValueOnce({
        snapshot: createEmptyCanvasBoardSnapshot(),
        remoteSync: {
          ok: false,
          reason: 'sync-failed',
          error: new Error('auth unavailable'),
        },
      })

    await harness.session.saveCurrentBoardSnapshot({ silent: true })
    await harness.session.saveCurrentBoardSnapshot({ silent: true })

    expect(harness.onError).toHaveBeenCalledOnce()
    expect(harness.onWarning).toHaveBeenCalledWith('已保存到本地，远端同步失败，将在网络恢复后重试')
  })

  it('reports a remote name conflict from a silent autosave as an error', async () => {
    const harness = createSessionHarness()
    vi.mocked(harness.boardSession.save).mockResolvedValueOnce({
      snapshot: createEmptyCanvasBoardSnapshot(),
      remoteSync: {
        ok: false,
        reason: 'conflict',
        error: new Error('name exists'),
      },
    })

    const saved = await harness.session.saveCurrentBoardSnapshot({ silent: true })

    expect(saved).toBe(false)
    expect(harness.onError).toHaveBeenCalledWith('已保存到本地，但画布名称已存在，请更换后再试')
    expect(harness.onSuccess).not.toHaveBeenCalled()
  })

  it('reports an unexpected silent autosave failure as an error', async () => {
    const harness = createSessionHarness()
    vi.mocked(harness.boardSession.save).mockRejectedValueOnce(new Error('保存服务不可用'))

    const saved = await harness.session.saveCurrentBoardSnapshot({ silent: true })

    expect(saved).toBe(false)
    expect(harness.onError).toHaveBeenCalledWith('保存服务不可用')
    expect(harness.onSuccess).not.toHaveBeenCalled()
  })

  it('suppresses repeated silent autosave errors in the retry state', async () => {
    const harness = createSessionHarness()
    vi.mocked(harness.boardSession.save).mockRejectedValue(new Error('保存服务不可用'))

    await harness.session.saveCurrentBoardSnapshot({ silent: true })
    await harness.session.saveCurrentBoardSnapshot({ silent: true })

    expect(harness.saveStatus.value).toBe('retry')
    expect(harness.onError).toHaveBeenCalledOnce()
  })

  it('notifies again when the same silent failure returns after a remote sync succeeds', async () => {
    const harness = createSessionHarness()
    vi.mocked(harness.boardSession.save)
      .mockResolvedValueOnce({
        snapshot: createEmptyCanvasBoardSnapshot(),
        remoteSync: {
          ok: false,
          reason: 'sync-failed',
          error: new Error('auth unavailable'),
        },
      })
      .mockResolvedValueOnce({
        snapshot: createEmptyCanvasBoardSnapshot(),
        remoteSync: { ok: true },
      })
      .mockResolvedValueOnce({
        snapshot: createEmptyCanvasBoardSnapshot(),
        remoteSync: {
          ok: false,
          reason: 'sync-failed',
          error: new Error('auth unavailable'),
        },
      })

    await harness.session.saveCurrentBoardSnapshot({ silent: true })
    await harness.session.saveCurrentBoardSnapshot({ silent: true })
    await harness.session.saveCurrentBoardSnapshot({ silent: true })

    expect(harness.onWarning).toHaveBeenCalledTimes(2)
  })

  it('keeps a successful silent autosave without a success toast', async () => {
    const harness = createSessionHarness()

    const saved = await harness.session.saveCurrentBoardSnapshot({ silent: true })

    expect(saved).toBe(true)
    expect(harness.onSuccess).not.toHaveBeenCalled()
  })

  it('preserves local draft edits when an older save completes', async () => {
    const harness = createSessionHarness()
    harness.boardBaseSnapshot.value = {
      ...harness.boardBaseSnapshot.value,
      promptNodes: [makePromptSnapshot(1)],
    }
    const savedSnapshot = createEmptyCanvasBoardSnapshot()
    savedSnapshot.promptNodes = [makePromptSnapshot(1)]
    let resolveSave: () => void = () => {
      throw new Error('save did not start')
    }
    vi.mocked(harness.boardSession.save).mockImplementationOnce(
      () =>
        new Promise<CanvasBoardSessionSaveResult>((resolve) => {
          resolveSave = () => {
            resolve({
              snapshot: savedSnapshot,
              remoteSync: { ok: true as const },
            })
          }
        }),
    )

    const savePromise = harness.session.saveCurrentBoardSnapshot()
    await Promise.resolve()
    harness.commitGraph.mockClear()
    harness.localEditVersion.value += 1
    harness.boardBaseSnapshot.value = {
      ...harness.boardBaseSnapshot.value,
      promptNodes: [{ ...makePromptSnapshot(2), x: 520 }],
    }
    resolveSave()

    const saved = await savePromise

    expect(saved).toBe(true)
    expect(harness.commitGraph).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: [expect.objectContaining({ id: 'prompt-2' })],
      }),
      { source: 'save-reconcile' },
    )
    expect(harness.fullNodes.value).toEqual([expect.objectContaining({ id: 'prompt-2' })])
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('pending')
  })

  it('rebases stabilized media without rolling back newer base-only group edits', async () => {
    const harness = createSessionHarness()
    const capturedGroup = {
      id: 'group-1' as const,
      nodeIds: ['image-1' as const],
      x: 0,
      y: 0,
      width: 320,
      height: 320,
    }
    const currentGroup = { ...capturedGroup, x: 480 }
    harness.boardBaseSnapshot.value = {
      ...harness.boardBaseSnapshot.value,
      groups: [capturedGroup],
    }
    harness.boardBaseSnapshot.value = {
      ...harness.boardBaseSnapshot.value,
      imageNodes: [
        {
          id: 1,
          type: 'image-node',
          x: 0,
          y: 0,
          width: 280,
          sourceImageUrl: 'blob:captured-source',
          sourceImageRatio: 1,
          outputStatus: 'ready',
          outputImages: [],
        },
      ],
    }
    let resolveSave!: () => void
    vi.mocked(harness.boardSession.save).mockImplementationOnce(
      (snapshot) =>
        new Promise<CanvasBoardSessionSaveResult>((resolve) => {
          const stabilized = JSON.parse(JSON.stringify(snapshot)) as CanvasBoardSnapshot
          stabilized.imageNodes[0]!.sourceImageUrl = '/uploads/canvas/stable-source.png'
          stabilized.imageNodes[0]!.sourceMediaAssetId = 42
          stabilized.mediaRefs = [
            {
              mediaAssetId: 42,
              refType: 'board_node',
              refId: 'image-1',
              role: 'source',
            },
          ]
          resolveSave = () => resolve({ snapshot: stabilized, remoteSync: { ok: true } })
        }),
    )

    const save = harness.session.saveCurrentBoardSnapshot()
    await Promise.resolve()
    harness.boardBaseSnapshot.value = {
      ...harness.boardBaseSnapshot.value,
      groups: [currentGroup],
    }
    harness.localEditVersion.value += 1
    resolveSave()

    expect(await save).toBe(true)
    expect(harness.boardBaseSnapshot.value.groups).toEqual([currentGroup])
    expect(harness.boardBaseSnapshot.value.imageNodes[0]).toMatchObject({
      sourceImageUrl: '/uploads/canvas/stable-source.png',
      sourceMediaAssetId: 42,
    })
    expect(harness.boardBaseSnapshot.value.mediaRefs).toEqual([
      {
        mediaAssetId: 42,
        refType: 'board_node',
        refId: 'image-1',
        role: 'source',
      },
    ])
    const committedNode = harness.fullNodes.value[0] as
      | { data?: { snapshotNode?: ImageNodeItem } }
      | undefined
    expect(committedNode?.data?.snapshotNode).toMatchObject({
      sourceImageUrl: '/uploads/canvas/stable-source.png',
      sourceMediaAssetId: 42,
    })
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('pending')
  })

  it('does not carry deferred media stabilization into a different Board', async () => {
    const harness = createSessionHarness('board-a')
    harness.boardBaseSnapshot.value = {
      ...harness.boardBaseSnapshot.value,
      imageNodes: [
        {
          id: 1,
          type: 'image-node',
          x: 0,
          y: 0,
          width: 280,
          sourceImageUrl: 'blob:board-a',
          sourceImageRatio: 1,
          outputStatus: 'ready',
          outputImages: [],
        },
      ],
    }
    let resolveFirstSave!: () => void
    vi.mocked(harness.boardSession.save).mockImplementationOnce(
      (snapshot) =>
        new Promise<CanvasBoardSessionSaveResult>((resolve) => {
          const stabilized = JSON.parse(JSON.stringify(snapshot)) as CanvasBoardSnapshot
          stabilized.imageNodes[0]!.sourceImageUrl = '/uploads/board-a-stable.png'
          stabilized.imageNodes[0]!.sourceMediaAssetId = 42
          resolveFirstSave = () => resolve({ snapshot: stabilized, remoteSync: { ok: true } })
        }),
    )

    const firstSave = harness.session.saveCurrentBoardSnapshot()
    await Promise.resolve()
    harness.localEditVersion.value += 1
    harness.canReconcileGraph.value = false
    resolveFirstSave()
    await firstSave

    const boardBSnapshot = createEmptyCanvasBoardSnapshot()
    boardBSnapshot.imageNodes = [
      {
        id: 1,
        type: 'image-node',
        x: 0,
        y: 0,
        width: 280,
        sourceImageUrl: 'blob:board-b',
        sourceImageRatio: 1,
        outputStatus: 'ready',
        outputImages: [],
      },
    ]
    harness.routeBoardId.value = 'board-b'
    vi.mocked(harness.boardSession.load).mockResolvedValueOnce(boardBSnapshot)
    await harness.session.loadInitialOrRouteBoard()

    await harness.session.saveCurrentBoardSnapshot()

    expect(harness.boardSession.save).toHaveBeenLastCalledWith(
      expect.objectContaining({
        imageNodes: [expect.objectContaining({ sourceImageUrl: 'blob:board-b' })],
      }),
      expect.anything(),
    )
  })

  it('rejects an older overlapping save after a newer save commits', async () => {
    const harness = createSessionHarness()
    const firstSnapshot = createEmptyCanvasBoardSnapshot()
    firstSnapshot.promptNodes = [makePromptSnapshot(1)]
    const secondSnapshot = createEmptyCanvasBoardSnapshot()
    secondSnapshot.promptNodes = [makePromptSnapshot(2)]
    const saveResolvers: Array<(result: CanvasBoardSessionSaveResult) => void> = []
    vi.mocked(harness.boardSession.save).mockImplementation(
      () =>
        new Promise<CanvasBoardSessionSaveResult>((resolve) => {
          saveResolvers.push(resolve)
        }),
    )

    const firstSave = harness.session.saveCurrentBoardSnapshot()
    await Promise.resolve()
    const secondSave = harness.session.saveCurrentBoardSnapshot()
    await Promise.resolve()

    saveResolvers[1]?.({ snapshot: secondSnapshot, remoteSync: { ok: true } })
    expect(await secondSave).toBe(true)
    saveResolvers[0]?.({ snapshot: firstSnapshot, remoteSync: { ok: true } })
    expect(await firstSave).toBe(true)

    expect(harness.commitGraph).toHaveBeenCalledTimes(1)
    expect(harness.boardBaseSnapshot.value).toStrictEqual(secondSnapshot)
    expect(harness.saveStatus.value).toBe('saved')
  })

  it('rejects a deferred save commit after the same Board opens a new attachment epoch', async () => {
    const harness = createSessionHarness()
    harness.boardBaseSnapshot.value = {
      ...harness.boardBaseSnapshot.value,
      promptNodes: [makePromptSnapshot(1)],
    }
    const savedSnapshot = createEmptyCanvasBoardSnapshot()
    savedSnapshot.promptNodes = [makePromptSnapshot(1)]
    let resolveSave!: () => void
    vi.mocked(harness.boardSession.save).mockImplementationOnce(
      () =>
        new Promise<CanvasBoardSessionSaveResult>((resolve) => {
          resolveSave = () => resolve({ snapshot: savedSnapshot, remoteSync: { ok: true } })
        }),
    )

    const save = harness.session.saveCurrentBoardSnapshot()
    await Promise.resolve()
    harness.boardAttachmentEpoch.value += 1
    resolveSave()

    expect(await save).toBe(true)
    expect(harness.commitGraph).not.toHaveBeenCalled()
    expect(harness.boardBaseSnapshot.value).not.toBe(savedSnapshot)
  })
})
