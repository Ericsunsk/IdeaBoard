import type { SubmitTaskParams } from '../../../../shared/api/task'
import type { PromptAspectRatio } from '../../../canvas-board/domain'

export function buildPromptAspectRatioTaskParams(
  ratio: PromptAspectRatio,
): Pick<SubmitTaskParams, 'ratio'> {
  if (ratio === 'auto') return {}
  return { ratio }
}
