import type { CanvasVueFlowGraphCommandOptions } from './useCanvasVueFlowGraphAdapter'

const FLOW_GRAPH_COMMAND_SIDE_EFFECTS_METADATA_KEY = 'flowGraphCommandSideEffects'

export type FlowGraphCommandSideEffectOptions = {
  recordHistory?: boolean
  historyBefore?: unknown
  markDirty?: boolean
  transactionSummary?: unknown
}

export function createFlowGraphCommandOptions(
  sideEffects: FlowGraphCommandSideEffectOptions = {},
): CanvasVueFlowGraphCommandOptions {
  return {
    metadata: {
      [FLOW_GRAPH_COMMAND_SIDE_EFFECTS_METADATA_KEY]: sideEffects,
    },
  }
}

export function readFlowGraphCommandSideEffects(
  metadata: Record<string, unknown> | undefined,
): FlowGraphCommandSideEffectOptions | null {
  const value = metadata?.[FLOW_GRAPH_COMMAND_SIDE_EFFECTS_METADATA_KEY]
  if (!value || typeof value !== 'object') return null
  return value as FlowGraphCommandSideEffectOptions
}
