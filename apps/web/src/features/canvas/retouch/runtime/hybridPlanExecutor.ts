import {
  createPlanWorkUnits,
  defaultNow,
  defaultYieldToMainThread,
  executeCooperativeCpuImageAdjustmentPlan,
  isAbortSignalError,
  normalizePixelChunkBytes,
  normalizePositiveInteger,
  normalizePositiveNumber,
} from './cooperativeCpuPlanExecutor'
import { createCpuImageAdjustmentPlanExecutor } from './cpuPlanExecutor'
import {
  reportPlanExecutionEvent,
  type CreateImageAdjustmentPlanExecutorOptions,
  type ImageAdjustmentPlanExecutor,
} from './planExecutorTypes'
import { createWorkerImageAdjustmentPlanExecutor } from './workerPlanExecutor'

export type CreateHybridImageAdjustmentPlanExecutorOptions =
  CreateImageAdjustmentPlanExecutorOptions & {
    cooperativeChunkBytes?: number
    cooperativeFrameBudgetMs?: number
    syncMaxWorkUnits?: number
    now?: () => number
    yieldToMainThread?: () => Promise<void>
  }

const DEFAULT_HYBRID_SYNC_MAX_WORK_UNITS = 24_000_000
const DEFAULT_HYBRID_COOPERATIVE_CHUNK_BYTES = 1024 * 1024
const DEFAULT_HYBRID_COOPERATIVE_FRAME_BUDGET_MS = 10

export function createHybridImageAdjustmentPlanExecutor(
  options: CreateHybridImageAdjustmentPlanExecutorOptions = {},
): ImageAdjustmentPlanExecutor {
  const cpuExecutor = createCpuImageAdjustmentPlanExecutor(options)
  const workerExecutor = createWorkerImageAdjustmentPlanExecutor(options)
  const syncMaxWorkUnits = normalizePositiveInteger(
    options.syncMaxWorkUnits,
    DEFAULT_HYBRID_SYNC_MAX_WORK_UNITS,
  )
  const cooperativeChunkBytes = normalizePixelChunkBytes(
    options.cooperativeChunkBytes,
    DEFAULT_HYBRID_COOPERATIVE_CHUNK_BYTES,
  )
  const cooperativeFrameBudgetMs = normalizePositiveNumber(
    options.cooperativeFrameBudgetMs,
    DEFAULT_HYBRID_COOPERATIVE_FRAME_BUDGET_MS,
  )
  const now = options.now ?? defaultNow
  const yieldToMainThread = options.yieldToMainThread ?? defaultYieldToMainThread

  return {
    async execute(input) {
      const pixelCount = Math.max(0, input.imageData.width * input.imageData.height)
      const workUnits = createPlanWorkUnits(input.imageData, input.plan)
      const executionMode =
        input.plan.length === 0 || workUnits <= syncMaxWorkUnits ? 'sync' : 'cooperative'
      reportPlanExecutionEvent(options.onPlanExecutorEvent, {
        kind: 'hybrid-plan-selected',
        backend: 'cpu-main',
        executionMode,
        reason: input.plan.length === 0 ? 'empty-plan' : 'work-threshold',
        planLength: input.plan.length,
        width: input.imageData.width,
        height: input.imageData.height,
        pixelCount,
        workUnits,
        syncThresholdWorkUnits: syncMaxWorkUnits,
      })
      if (executionMode === 'sync') return cpuExecutor.execute(input)

      try {
        const output = await executeCooperativeCpuImageAdjustmentPlan({
          imageData: input.imageData,
          plan: input.plan,
          signal: input.signal,
          onProgress: input.onProgress,
          chunkBytes: cooperativeChunkBytes,
          frameBudgetMs: cooperativeFrameBudgetMs,
          now,
          yieldToMainThread,
        })
        reportPlanExecutionEvent(options.onPlanExecutorEvent, {
          kind: 'cpu-plan-completed',
          planLength: input.plan.length,
          width: input.imageData.width,
          height: input.imageData.height,
        })
        return output
      } catch (error) {
        if (isAbortSignalError(input.signal, error)) throw error
        return workerExecutor.execute(input)
      }
    },

    dispose() {
      cpuExecutor.dispose?.()
      workerExecutor.dispose?.()
    },
  }
}
