import {
  normalizeTaskImageResults,
  type TaskImageResult,
} from '../../../../../core/query/taskResult'
import type { WaitWorkspaceTaskOptions } from '../../../../../api/workspaceTaskWaiter'
import {
  resolveFirstTaskImageResultDisplay,
  resolveTaskImageResultAtIndexDisplay,
  type ImageSizeLoader,
  type ReceiptDerivativeWaiter,
  type ResolvedTaskImageResult,
} from '../result-nodes/resultDisplay'
import { waitForReceiptDerivativeReady } from '../result-nodes/receiptDerivativeEvent'

export type TaskImageOutcome = {
  kind: 'completed' | 'cancelled'
  results: TaskImageResult[]
}

type TaskImageWaitOptions = { deadlineAt: number; signal: AbortSignal }

export type TaskImageOutcomeWaiter = (
  taskId: string,
  options?: WaitWorkspaceTaskOptions,
) => Promise<TaskImageOutcome>

export type ResolvedTaskImageOutcome =
  | { kind: 'completed'; result: ResolvedTaskImageResult }
  | { kind: 'cancelled'; result: ResolvedTaskImageResult | null }

/** One terminal Task fact, shared by the current execution or recovery batch. */
export type TaskImageObservation = {
  resolve: (
    resultIndex: number | undefined,
    fallbackRatio: number,
    options: TaskImageWaitOptions,
  ) => Promise<ResolvedTaskImageOutcome>
}

export type TaskImageObserver = (
  taskId: string,
  options: TaskImageWaitOptions,
) => Promise<TaskImageObservation>

export function createTaskImageObserver(
  waitForOutcome: TaskImageOutcomeWaiter,
  loadImageSize: ImageSizeLoader,
  waitForReceiptDerivative: ReceiptDerivativeWaiter = waitForReceiptDerivativeReady,
): TaskImageObserver {
  return async (taskId, options) => {
    const observed = await waitForOutcome(taskId, options)
    const outcome = { kind: observed.kind, results: normalizeTaskImageResults(observed.results) }
    return {
      resolve: (resultIndex, fallbackRatio, { signal }) =>
        resolveTaskImageOutcome(
          outcome,
          resultIndex,
          fallbackRatio,
          loadImageSize,
          signal,
          waitForReceiptDerivative,
        ),
    }
  }
}

/** Normal execution and recovery agree on missing outputs after cancellation. */
async function resolveTaskImageOutcome(
  outcome: TaskImageOutcome,
  resultIndex: number | undefined,
  fallbackRatio: number,
  loadImageSize: ImageSizeLoader,
  signal?: AbortSignal,
  waitForReceiptDerivative?: ReceiptDerivativeWaiter,
): Promise<ResolvedTaskImageOutcome> {
  const { results } = outcome
  const result =
    resultIndex === undefined
      ? results[0]
      : results.find((candidate) => candidate.resultIndex === resultIndex)
  if (outcome.kind === 'cancelled' && !result) return { kind: 'cancelled', result: null }
  const resolved =
    resultIndex === undefined
      ? await resolveFirstTaskImageResultDisplay(
          results,
          fallbackRatio,
          loadImageSize,
          signal,
          waitForReceiptDerivative,
        )
      : await resolveTaskImageResultAtIndexDisplay(
          results,
          resultIndex,
          fallbackRatio,
          loadImageSize,
          signal,
          waitForReceiptDerivative,
        )
  return { kind: outcome.kind, result: resolved }
}
