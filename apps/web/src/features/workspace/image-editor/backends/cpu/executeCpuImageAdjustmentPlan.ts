import type { ImageEditorAdjustmentLayer } from '../../types'
import {
  applyImageAdjustments,
  applyImageAdjustmentsInPlace,
  executeImageAdjustmentPlan,
  type ImageAdjustmentPlan,
} from '../../utils/imageAdjustmentEngine'
import { cloneImageData } from '../../utils/imageDataUtils'

export type CpuImageAdjustmentBackendId = 'cpu-main'

export type CpuImageAdjustmentBackend = {
  readonly id: CpuImageAdjustmentBackendId
  execute(source: ImageData, plan: ImageAdjustmentPlan): ImageData
  executeInPlace(imageData: ImageData, plan: ImageAdjustmentPlan): ImageData
  apply(source: ImageData, layers: readonly ImageEditorAdjustmentLayer[]): ImageData
  applyInPlace(imageData: ImageData, layers: readonly ImageEditorAdjustmentLayer[]): ImageData
}

export function executeCpuImageAdjustmentPlan(
  source: ImageData,
  plan: ImageAdjustmentPlan,
): ImageData {
  const output = cloneImageData(source)
  return executeCpuImageAdjustmentPlanInPlace(output, plan)
}

export function executeCpuImageAdjustmentPlanInPlace(
  imageData: ImageData,
  plan: ImageAdjustmentPlan,
): ImageData {
  return executeImageAdjustmentPlan(imageData, plan)
}

export function applyCpuImageAdjustments(
  source: ImageData,
  layers: readonly ImageEditorAdjustmentLayer[],
): ImageData {
  return applyImageAdjustments(source, layers)
}

export function applyCpuImageAdjustmentsInPlace(
  imageData: ImageData,
  layers: readonly ImageEditorAdjustmentLayer[],
): ImageData {
  return applyImageAdjustmentsInPlace(imageData, layers)
}

export const cpuImageAdjustmentBackend: CpuImageAdjustmentBackend = {
  id: 'cpu-main',
  execute: executeCpuImageAdjustmentPlan,
  executeInPlace: executeCpuImageAdjustmentPlanInPlace,
  apply: applyCpuImageAdjustments,
  applyInPlace: applyCpuImageAdjustmentsInPlace,
}
