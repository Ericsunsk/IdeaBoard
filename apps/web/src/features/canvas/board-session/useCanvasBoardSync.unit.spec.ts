import { computed, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { createEmptyCanvasBoardSnapshot } from '../../canvas-board/domain'
import type { CanvasBoardSession } from './useCanvasBoardSession'
import { useCanvasBoardSync } from './useCanvasBoardSync'

function createHarness() {
  const boardBaseSnapshot = ref(createEmptyCanvasBoardSnapshot())
  const routeBoardId = ref('board-a')
  const isDirty = ref(true)
  const saveStatus = ref<'idle' | 'saved' | 'pending' | 'retry' | 'conflict' | 'recover'>('idle')
  const isLoadingBoardSnapshot = ref(false)
  const isSavingBoard = ref(false)
  const boardSession: CanvasBoardSession = {
    currentBoardId: computed(() => routeBoardId.value),
    exists: vi.fn(() => true),
    load: vi.fn(async () => createEmptyCanvasBoardSnapshot()),
    save: vi.fn(async (snapshot) => ({ snapshot, remoteSync: { ok: true as const } })),
    refreshPreview: vi.fn(),
  }
  const persistence = useCanvasBoardSync({
    buildCurrentSnapshot: () => boardBaseSnapshot.value,
    isDirty,
    saveStatus,
    graphRevision: ref(0),
    isLoadingBoardSnapshot,
    isSavingBoard,
    boardSession,
    readBoardAttachmentEpoch: () => 1,
    canApplyReconciliation: () => true,
    onWarning: vi.fn(),
    onSuccess: vi.fn(),
    onError: vi.fn(),
  })
  return { persistence, boardSession, routeBoardId }
}

describe('useCanvasBoardSync', () => {
  it('exposes board identity and coordinator results without renderer callbacks', async () => {
    const harness = createHarness()
    expect(harness.persistence.currentBoardId.value).toBe('board-a')
    const result = await harness.persistence.saveCurrentBoardSnapshot()
    expect(result.saved).toBe(true)
    expect(result.reconciliation?.snapshot).toBeDefined()
  })

  it('returns a load result for the workspace graph runtime to apply', async () => {
    const harness = createHarness()
    const result = await harness.persistence.loadInitialOrRouteBoard()
    expect(result.status).toBe('loaded')
    expect(harness.boardSession.load).toHaveBeenCalledWith('board-a')
  })
})
