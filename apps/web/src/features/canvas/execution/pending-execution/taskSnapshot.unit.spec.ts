import { describe, expect, it } from 'vitest'
import { createEmptyBoardSnapshot } from '../../../board/domain'
import { collectPendingImageTasks } from './taskSnapshot'

describe('collectPendingImageTasks', () => {
  it('reads reattach facts from Canvas domain snapshots and preserves client action identity', () => {
    const snapshot = createEmptyBoardSnapshot()
    snapshot.imageNodes.push(
      {
        id: 1,
        type: 'image-node',
        x: 0,
        y: 0,
        width: 280,
        sourceImageUrl: '',
        sourceImageRatio: 0.5,
        pendingTaskId: 'task-1',
        pendingResultIndex: 2,
        pendingClientActionId: 'action-1',
        outputStatus: 'running',
        outputImages: [],
      },
      {
        id: 2,
        type: 'image-node',
        x: 0,
        y: 0,
        width: 280,
        sourceImageUrl: '',
        sourceImageRatio: 1,
        pendingTaskId: 'task-2',
        outputStatus: 'ready',
        outputImages: [],
      },
      {
        id: 3,
        type: 'image-node',
        x: 0,
        y: 0,
        width: 280,
        sourceImageUrl: '',
        sourceImageRatio: 1,
        pendingTaskId: 'task-without-action',
        outputStatus: 'running',
        outputImages: [],
      },
    )
    expect(collectPendingImageTasks(snapshot)).toEqual([
      {
        resultNodeId: 'image-1',
        taskId: 'task-1',
        resultIndex: 2,
        fallbackRatio: 0.5,
        clientActionId: 'action-1',
      },
    ])
  })
})
