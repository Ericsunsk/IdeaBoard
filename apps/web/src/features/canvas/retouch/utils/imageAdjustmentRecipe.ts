import type { ImageAdjustmentRecipe, ImageAdjustmentRecipeStep } from '../../../canvas-board/domain'
import type { ImageAdjustmentPlan, ImageAdjustmentPlanStep } from './imageAdjustmentEngine'

export function serializeImageAdjustmentPlan(
  sourceMediaAssetId: number,
  plan: ImageAdjustmentPlan,
  sourceMimeType?: string,
): ImageAdjustmentRecipe {
  return {
    version: 1,
    sourceMediaAssetId,
    ...(sourceMimeType ? { sourceMimeType } : {}),
    plan: plan.map(serializeStep),
  }
}

export function deserializeImageAdjustmentPlan(recipe: ImageAdjustmentRecipe): ImageAdjustmentPlan {
  return recipe.plan.map(deserializeStep)
}

function serializeStep(step: ImageAdjustmentPlanStep): ImageAdjustmentRecipeStep {
  switch (step.kind) {
    case 'hsl':
      return { kind: 'hsl', params: { ...step.params } }
    case 'threshold':
      return { kind: 'threshold', threshold: step.threshold, invert: step.invert }
    case 'channel-lut':
      return {
        kind: 'channel-lut',
        luts: {
          red: Array.from(step.luts.red),
          green: Array.from(step.luts.green),
          blue: Array.from(step.luts.blue),
        },
      }
    case 'sharpen':
      return { kind: 'sharpen', params: { ...step.params } }
  }
}

function deserializeStep(step: ImageAdjustmentRecipeStep): ImageAdjustmentPlanStep {
  switch (step.kind) {
    case 'hsl':
      return { kind: 'hsl', params: { ...step.params } }
    case 'threshold':
      return { kind: 'threshold', threshold: step.threshold, invert: step.invert }
    case 'channel-lut':
      return {
        kind: 'channel-lut',
        luts: {
          red: new Uint8ClampedArray(step.luts.red),
          green: new Uint8ClampedArray(step.luts.green),
          blue: new Uint8ClampedArray(step.luts.blue),
        },
      }
    case 'sharpen':
      return { kind: 'sharpen', params: { ...step.params } }
  }
}
