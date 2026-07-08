import { describe, expect, it, vi } from 'vitest'
import type { SubmitTaskData } from '../../../../shared/api/task'
import {
  createPendingTaskClientActionIds,
  withTaskClientActionId,
} from './taskClientActionIds'

describe('taskClientActionIds', () => {
  it('adds a normalized client action id to a submit request', () => {
    const request: SubmitTaskData = {
      prompt: 'generate',
      feature: 'pattern-gen',
    }

    expect(withTaskClientActionId(request, ' canvas:action-1 ')).toEqual({
      ...request,
      client_action_id: 'canvas:action-1',
    })
    expect(withTaskClientActionId(request, '')).toBe(request)
  })

  it('reuses ids within one pending action scope and rotates them for a new scope', () => {
    const createId = vi
      .fn()
      .mockReturnValueOnce('canvas:action-1')
      .mockReturnValueOnce('canvas:action-2')
      .mockReturnValueOnce('canvas:action-3')
    const firstScope = createPendingTaskClientActionIds(createId)
    const nextScope = createPendingTaskClientActionIds(createId)

    expect(firstScope.getForResultNode('image-1')).toBe('canvas:action-1')
    expect(firstScope.getForResultNode('image-1')).toBe('canvas:action-1')
    expect(firstScope.getForResultNode('image-2')).toBe('canvas:action-2')
    expect(nextScope.getForResultNode('image-1')).toBe('canvas:action-3')
  })
})
