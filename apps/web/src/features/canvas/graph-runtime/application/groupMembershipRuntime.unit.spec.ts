import { computed, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { createEmptyBoardSnapshot } from '../../../board/domain'
import { CanvasGraphSession, type CanvasGraphGroupId } from '..'
import { createGroupMembershipRuntime } from './groupMembershipRuntime'
import type { CanvasGroupFrame } from '../../utils/canvasGroupGeometry'

describe('groupMembershipRuntime', () => {
  function createHarness() {
    const snapshot = createEmptyBoardSnapshot()
    snapshot.groups = [
      {
        id: 'group-1',
        nodeIds: [],
        x: 0,
        y: 0,
        width: 200,
        height: 200,
      },
    ]
    snapshot.promptNodes = [
      {
        id: 1,
        type: 'prompt',
        x: 20,
        y: 20,
        width: 100,
        text: '',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    ]
    const session = CanvasGraphSession.hydrate('board-test', snapshot)
    const selectedGroupId = ref<CanvasGraphGroupId | null>('group-1')
    const groupFrames = computed<CanvasGroupFrame[]>(() => [
      {
        id: 'group-1',
        nodeIds: [],
        bounds: { left: 0, top: 0, right: 200, bottom: 200, width: 200, height: 200 },
        selected: true,
        order: 0,
      },
    ])

    const onWarning = vi.fn()
    const selectOnlyFlowNodesByGraphIds = vi.fn(() => true)
    const clearSelectedGroup = vi.fn(() => true)

    const runtime = createGroupMembershipRuntime({
      readCanvasGraphStore: () => session.store,
      isGraphSessionAttached: () => true,
      selectedGroupId,
      groupFrames,
      readGroup: (id) => (id ? (snapshot.groups.find((g) => g.id === id) ?? null) : null),
      resolveGraphNodeIdForFlowNode: (node) => (node.id === 'prompt-1' ? 'prompt-1' : null),
      resolveGraphNodeIdForFlowNodeId: (id) => (id === 'prompt-1' ? 'prompt-1' : null),
      selectOnlyFlowNodesByGraphIds,
      clearSelectedGroup,
      onWarning,
      updateFailedMessage: 'Update failed',
    })

    return {
      runtime,
      session,
      selectedGroupId,
      selectOnlyFlowNodesByGraphIds,
      clearSelectedGroup,
    }
  }

  it('detects external nodes covered by the selected group frame', () => {
    const { runtime } = createHarness()
    expect(runtime.selectedGroupCoveredExternalNodeIds.value).toEqual(['prompt-1'])
  })

  it('captures pending group join and joins node when clicked', () => {
    const { runtime, session, selectOnlyFlowNodesByGraphIds } = createHarness()

    const captured = runtime.capturePendingGroupJoinFromSelectedGroup()
    expect(captured).toBe(true)
    expect(runtime.pendingGroupJoin.value).toEqual({
      groupId: 'group-1',
      nodeIds: ['prompt-1'],
    })

    const joined = runtime.joinPendingGroupNode({ id: 'prompt-1', data: undefined })
    expect(joined).toBe(true)
    expect(runtime.pendingGroupJoin.value).toBe(null)

    expect(session.store.getGroupIdForNode('prompt-1')).toBe('group-1')
    expect(selectOnlyFlowNodesByGraphIds).toHaveBeenCalledWith(['prompt-1'])
  })

  it('resolves removed flow node changes into graph node ids', () => {
    const { runtime } = createHarness()
    const resolved = runtime.resolveRemovedFlowNodeChanges([
      { id: 'prompt-1', type: 'remove' },
      { id: 'prompt-unknown', type: 'remove' },
    ])
    expect(resolved).toBe(null)

    const validResolved = runtime.resolveRemovedFlowNodeChanges([
      { id: 'prompt-1', type: 'remove' },
    ])
    expect(validResolved).toEqual({
      flowNodeIds: ['prompt-1'],
      graphNodeIds: ['prompt-1'],
    })
  })
})
