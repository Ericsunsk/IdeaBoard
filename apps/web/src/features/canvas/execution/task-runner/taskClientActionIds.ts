import type { SubmitTaskData } from '../../../../shared/api/task'

export type ClientActionIdFactory = () => string

export type PendingTaskClientActionIds = {
  getForResultNode: (activationId: string, resultNodeId: string) => string
}

const CANVAS_CLIENT_ACTION_ID_PREFIX = 'canvas:'

function createCanvasId(): string {
  const randomUUID = globalThis.crypto?.randomUUID
  if (typeof randomUUID === 'function') {
    return `${CANVAS_CLIENT_ACTION_ID_PREFIX}${randomUUID.call(globalThis.crypto)}`
  }
  return `${CANVAS_CLIENT_ACTION_ID_PREFIX}${Date.now().toString(36)}-${Math.random()
    .toString(36)
    .slice(2, 12)}`
}

export function createCanvasTaskClientActionId(): string {
  return createCanvasId()
}

export function createCanvasActivationId(): string {
  return createCanvasId()
}

export function createPendingTaskClientActionIds(
  createId: ClientActionIdFactory = createCanvasTaskClientActionId,
): PendingTaskClientActionIds {
  const idsByResultNodeId = new Map<string, string>()

  return {
    getForResultNode(activationId, resultNodeId) {
      const key = `${activationId}:${resultNodeId}`
      const existing = idsByResultNodeId.get(key)
      if (existing) return existing
      const next = createId()
      idsByResultNodeId.set(key, next)
      return next
    },
  }
}

export function withTaskClientActionId(
  request: SubmitTaskData,
  clientActionId: string | undefined,
): SubmitTaskData {
  const normalizedClientActionId = `${clientActionId ?? ''}`.trim()
  if (!normalizedClientActionId) return request
  return {
    ...request,
    client_action_id: normalizedClientActionId,
  }
}
