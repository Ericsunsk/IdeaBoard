import { describe, expect, it, vi } from 'vitest'
import type { SubmitTaskData } from '../../../../shared/api/task'
import {
  createCanvasActivationId,
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

  it('reuses an id for transport retries within one activation and rotates it for another', () => {
    const createId = vi
      .fn()
      .mockReturnValueOnce('canvas:action-1')
      .mockReturnValueOnce('canvas:action-2')
      .mockReturnValueOnce('canvas:action-3')
      .mockReturnValueOnce('canvas:action-4')
    const firstScope = createPendingTaskClientActionIds(createId)
    const nextScope = createPendingTaskClientActionIds(createId)

    expect(firstScope.getForResultNode('canvas:activation-1', 'image-1')).toBe('canvas:action-1')
    expect(firstScope.getForResultNode('canvas:activation-1', 'image-1')).toBe('canvas:action-1')
    expect(firstScope.getForResultNode('canvas:activation-1', 'image-2')).toBe('canvas:action-2')
    expect(firstScope.getForResultNode('canvas:activation-2', 'image-1')).toBe('canvas:action-3')
    expect(nextScope.getForResultNode('canvas:activation-1', 'image-1')).toBe('canvas:action-4')
  })

  it('creates a Canvas activation identity independently from task idempotency ids', () => {
    expect(createCanvasActivationId()).toMatch(/^canvas:/)
  })
})
