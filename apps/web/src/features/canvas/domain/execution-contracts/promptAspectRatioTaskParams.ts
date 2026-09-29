import type { CanvasTaskParams } from './canvasTaskRequest'
import type { PromptAspectRatio } from '../../../board'

export function buildPromptAspectRatioTaskParams(
  ratio: PromptAspectRatio,
): Pick<CanvasTaskParams, 'ratio'> {
  if (ratio === 'auto') return {}
  return { ratio }
}
