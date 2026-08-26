import { WORKFLOW_FEATURE, type WorkflowFeature } from '../../../../shared/domain/workflowFeatures'
import {
  taskApi,
  type SubmitTaskData,
  type SubmitTaskReadinessData,
} from '../../../../shared/api/task'
import {
  buildFusionTaskRequest,
  buildImageActionReadinessRequest,
  buildInpaintReadinessRequest,
  buildPromptGenerationTaskRequest,
} from '../commands/flowImageTaskRequests'
import type { OutputResolution } from '../../../../shared/domain/generationContract'
import {
  createCanvasTaskClientActionId,
  withTaskClientActionId,
} from '../task-runner/taskClientActionIds'

function buildCanvasBillingReadinessRequest(feature: WorkflowFeature): SubmitTaskData {
  switch (feature) {
    case WORKFLOW_FEATURE.inpaint:
      return buildInpaintReadinessRequest()
    case WORKFLOW_FEATURE.promptGeneration:
      return buildPromptGenerationTaskRequest({
        prompt: '',
        aspectRatio: 'auto',
        outputCount: 1,
      })
    case WORKFLOW_FEATURE.styleFusion:
      throw new Error('Fusion readiness requires current image inputs')
    case WORKFLOW_FEATURE.cutout:
    case WORKFLOW_FEATURE.variations:
    case WORKFLOW_FEATURE.render:
    case WORKFLOW_FEATURE.hdUpscale:
    case WORKFLOW_FEATURE.outpaint:
      return buildImageActionReadinessRequest(feature)
  }
}

export async function fetchCanvasBillingReadiness(
  feature: WorkflowFeature,
  request?: SubmitTaskData,
): Promise<SubmitTaskReadinessData> {
  return taskApi.checkSubmitReadiness({
    ...withTaskClientActionId(
      request ?? buildCanvasBillingReadinessRequest(feature),
      createCanvasTaskClientActionId(),
    ),
  })
}

export function buildFusionBillingReadinessRequest(input: {
  imageUrls: string[]
  aspectRatio: Parameters<typeof buildFusionTaskRequest>[0]['aspectRatio']
  outputCount?: Parameters<typeof buildFusionTaskRequest>[0]['outputCount']
  outputResolution?: OutputResolution
}): SubmitTaskData | null {
  const imageUrls = input.imageUrls
    .map((url) => url.trim())
    .filter(Boolean)
    .slice(0, 4)
  if (imageUrls.length < 2) return null
  return buildFusionTaskRequest({
    prompt: '[style-fusion] submit readiness',
    imageUrls,
    inputWeights: imageUrls.map(() => 1 / imageUrls.length),
    outputCount: input.outputCount,
    sizeSource: null,
    targetSize: null,
    aspectRatio: input.aspectRatio,
    outputResolution: input.outputResolution,
  })
}
