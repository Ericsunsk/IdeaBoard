import type { ImageRetouchRecipe, ImageRetouchRecipeStep } from '../../../canvas-board/domain'
import type { ImageRetouchPlan, ImageRetouchPlanStep } from './engine'

export function serializeImageRetouchPlan(
  sourceAssetId: string,
  plan: ImageRetouchPlan,
  sourceMimeType?: string,
): ImageRetouchRecipe {
  return {
    version: 1,
    sourceAssetId,
    ...(sourceMimeType ? { sourceMimeType } : {}),
    plan: plan.map(serializeStep),
  }
}

export function deserializeImageRetouchPlan(recipe: ImageRetouchRecipe): ImageRetouchPlan {
  return recipe.plan.map(deserializeStep)
}

function serializeStep(step: ImageRetouchPlanStep): ImageRetouchRecipeStep {
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

function deserializeStep(step: ImageRetouchRecipeStep): ImageRetouchPlanStep {
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
