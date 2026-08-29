import { cloneImageData } from '../utils/imageDataUtils'
import { isProcessingCancelled, processingCancelledError } from './processingCancelled'
import {
  createImageAdjustmentPlanStepExecutionState,
  estimateImageAdjustmentPlanStepRangeWorkUnits,
  estimateImageAdjustmentPlanWorkUnits,
  executeImageAdjustmentPlanStepRange,
  type ImageAdjustmentPlan,
} from '../utils/imageAdjustmentEngine'
import type { ImageProcessingBackendTaskProgressReporter } from './planExecutorTypes'

export type CooperativeCpuPlanExecutionInput = {
  imageData: ImageData
  plan: ImageAdjustmentPlan
  signal: AbortSignal
  onProgress?: ImageProcessingBackendTaskProgressReporter
  chunkBytes: number
  frameBudgetMs: number
  now: () => number
  yieldToMainThread: () => Promise<void>
}

export async function executeCooperativeCpuImageAdjustmentPlan(
  input: CooperativeCpuPlanExecutionInput,
): Promise<ImageData> {
  throwIfAborted(input.signal)

  const output = cloneImageData(input.imageData)
  const total = createPlanWorkUnits(output, input.plan)
  let loaded = 0
  let frameStartedAt = input.now()
  input.onProgress?.({ phase: 'render', loaded, total })

  if (total === 0) return output

  for (const step of input.plan) {
    const state = createImageAdjustmentPlanStepExecutionState(output, step)
    for (let startByte = 0; startByte < output.data.length; startByte += input.chunkBytes) {
      throwIfAborted(input.signal)

      const endByte = Math.min(startByte + input.chunkBytes, output.data.length)
      executeImageAdjustmentPlanStepRange(output.data, step, startByte, endByte, state)
      loaded += estimateImageAdjustmentPlanStepRangeWorkUnits(output.data, step, startByte, endByte)
      input.onProgress?.({ phase: 'render', loaded, total })
      throwIfAborted(input.signal)

      if (input.now() - frameStartedAt >= input.frameBudgetMs) {
        await input.yieldToMainThread()
        throwIfAborted(input.signal)
        frameStartedAt = input.now()
      }
    }
  }

  return output
}

export function createPlanWorkUnits(imageData: ImageData, plan: ImageAdjustmentPlan) {
  return Math.max(0, estimateImageAdjustmentPlanWorkUnits(imageData, plan))
}

export function throwIfAborted(signal: AbortSignal) {
  if (!signal.aborted) return
  throw processingCancelledError()
}

export function isAbortSignalError(signal: AbortSignal, error: unknown) {
  if (signal.aborted) return true
  return isProcessingCancelled(error)
}

export function normalizePixelChunkBytes(value: number | undefined, fallback: number) {
  const normalizedFallback = Math.max(4, Math.floor(fallback / 4) * 4)
  if (value === undefined) return normalizedFallback
  if (!Number.isFinite(value) || value < 4) return normalizedFallback
  return Math.max(4, Math.floor(value / 4) * 4)
}

export function normalizePositiveInteger(value: number | undefined, fallback: number) {
  if (value === undefined) return fallback
  if (!Number.isFinite(value) || value < 0) return fallback
  return Math.floor(value)
}

export function normalizePositiveNumber(value: number | undefined, fallback: number) {
  if (value === undefined) return fallback
  if (!Number.isFinite(value) || value < 0) return fallback
  return value
}

export function defaultNow() {
  return globalThis.performance?.now?.() ?? Date.now()
}

export function defaultYieldToMainThread() {
  const scheduler = (
    globalThis as {
      scheduler?: {
        yield?: () => Promise<void>
      }
    }
  ).scheduler
  if (typeof scheduler?.yield === 'function') return scheduler.yield.call(scheduler)

  return new Promise<void>((resolve) => {
    setTimeout(resolve, 0)
  })
}
