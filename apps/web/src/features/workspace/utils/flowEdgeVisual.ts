import { parseHandlePort } from '../services/flowTaskExecutorUtils'

export type FlowEdgeTone = 'image' | 'mask' | 'prompt'

export function resolveFlowEdgeToneFromHandles(
  sourceHandle: string | null | undefined,
  targetHandle: string | null | undefined,
): FlowEdgeTone {
  const sourcePort = parseHandlePort(sourceHandle)
  const targetPort = parseHandlePort(targetHandle)
  if (sourcePort === 'mask' || targetPort === 'mask') return 'mask'
  if (sourcePort === 'prompt' || targetPort === 'prompt') return 'prompt'
  return 'image'
}

export function buildFlowEdgeClassName(tone: FlowEdgeTone, selected = false): string {
  return selected ? `tone-${tone} is-selected` : `tone-${tone}`
}
