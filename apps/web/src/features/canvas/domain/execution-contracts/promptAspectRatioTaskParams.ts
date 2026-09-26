import type { SubmitTaskParams } from './canvasTaskRequest'
import type { PromptAspectRatio } from '../../../board'

export function buildPromptAspectRatioTaskParams(
  ratio: PromptAspectRatio,
): Pick<SubmitTaskParams, 'ratio'> {
  if (ratio === 'auto') return {}
  return { ratio }
}
