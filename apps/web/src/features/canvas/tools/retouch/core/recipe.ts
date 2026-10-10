import type { ImageRetouchRecipe, ImageRetouchRecipeStep } from '../../../../board/ports/canvas'
import type { RetouchCurvesLuts } from './kinds/curves'
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
        luts: encodeLuts(step.luts),
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
        luts: decodeLuts(step.luts),
      }
    case 'sharpen':
      return { kind: 'sharpen', params: { ...step.params } }
  }
}

function encodeLuts(luts: RetouchCurvesLuts) {
  return {
    red: Array.from(luts.red),
    green: Array.from(luts.green),
    blue: Array.from(luts.blue),
  }
}

function decodeLuts(luts: {
  red: ArrayLike<number>
  green: ArrayLike<number>
  blue: ArrayLike<number>
}) {
  return {
    red: new Uint8ClampedArray(luts.red),
    green: new Uint8ClampedArray(luts.green),
    blue: new Uint8ClampedArray(luts.blue),
  }
}
