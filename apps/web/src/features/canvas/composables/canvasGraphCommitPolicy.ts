import type { CanvasVueFlowGraphCommandOptions } from './useCanvasVueFlowGraphAdapter'
import type { CanvasGraphTransactionSummary } from '../graph-runtime'
import { createFlowGraphCommandOptionsFromTransactionSummary } from './flowGraphAdapterCommandSideEffects'

export type CanvasGraphCommitPolicy = Readonly<{
  summary: CanvasGraphTransactionSummary
  historyBefore?: unknown
}>

type CanvasGraphProjectionCommand<TResult> = (
  commandOptions: CanvasVueFlowGraphCommandOptions,
) => TResult

export function applyCanvasGraphCommitPolicy<TResult>(
  policy: CanvasGraphCommitPolicy,
  command: CanvasGraphProjectionCommand<TResult>,
): TResult {
  return command(
    createFlowGraphCommandOptionsFromTransactionSummary(policy.summary, {
      historyBefore: policy.historyBefore,
    }),
  )
}
