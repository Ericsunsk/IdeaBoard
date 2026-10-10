import type { CanvasTaskRequestData } from '../../../domain/execution-contracts/canvasTaskRequest'
import { createUniqueId } from '../../../../../core/utils/ids'

export type ClientActionIdFactory = () => string

const CANVAS_CLIENT_ACTION_ID_PREFIX = 'canvas:'

function createCanvasId(): string {
  return `${CANVAS_CLIENT_ACTION_ID_PREFIX}${createUniqueId()}`
}

export function createCanvasTaskClientActionId(): string {
  return createCanvasId()
}

export function createCanvasActivationId(): string {
  return createCanvasId()
}

export function withTaskClientActionId(
  request: CanvasTaskRequestData,
  clientActionId: string | undefined,
): CanvasTaskRequestData {
  const normalizedClientActionId = `${clientActionId ?? ''}`.trim()
  if (!normalizedClientActionId) return request
  return {
    ...request,
    client_action_id: normalizedClientActionId,
  }
}
