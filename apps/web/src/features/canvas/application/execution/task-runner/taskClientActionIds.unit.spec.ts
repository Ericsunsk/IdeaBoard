import { describe, expect, it } from 'vitest'
import type { CanvasTaskRequestData } from '../../../domain/execution-contracts/canvasTaskRequest'
import { createCanvasActivationId, withTaskClientActionId } from './taskClientActionIds'

describe('taskClientActionIds', () => {
  it('adds a normalized client action id to a submit request', () => {
    const request: CanvasTaskRequestData = {
      prompt: 'generate',
      service: 'generation',
    }

    expect(withTaskClientActionId(request, ' canvas:action-1 ')).toEqual({
      ...request,
      client_action_id: 'canvas:action-1',
    })
    expect(withTaskClientActionId(request, '')).toBe(request)
  })

  it('creates a Canvas activation identity independently from task idempotency ids', () => {
    expect(createCanvasActivationId()).toMatch(/^canvas:/)
  })
})
