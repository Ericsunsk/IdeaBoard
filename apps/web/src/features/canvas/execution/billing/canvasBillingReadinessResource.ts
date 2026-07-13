import { WORKFLOW_FEATURE, type WorkflowFeature } from '../../../../shared/domain/workflowFeatures'
import {
  taskApi,
  type SubmitTaskData,
  type SubmitTaskReadinessData,
} from '../../../../shared/api/task'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'
import {
  buildFusionTaskRequest,
  buildImageActionReadinessRequest,
  buildPromptImageReadinessRequest,
  buildTextPromptTaskRequest,
} from '../commands/flowImageTaskRequests'
import {
  createCanvasTaskClientActionId,
  withTaskClientActionId,
} from '../task-runner/taskClientActionIds'

function buildCanvasBillingReadinessRequest(feature: WorkflowFeature): SubmitTaskData {
  switch (feature) {
    case WORKFLOW_FEATURE.partialEdit:
      return buildPromptImageReadinessRequest()
    case WORKFLOW_FEATURE.patternGen:
      return buildTextPromptTaskRequest({ prompt: 'submit readiness', aspectRatio: 'auto' })
    case WORKFLOW_FEATURE.styleFusion:
      throw new Error('Fusion readiness requires current image inputs')
    case WORKFLOW_FEATURE.removeBg:
    case WORKFLOW_FEATURE.styleVariation:
    case WORKFLOW_FEATURE.sketchToImage:
    case WORKFLOW_FEATURE.hdUpscale:
    case WORKFLOW_FEATURE.imageExtend:
      return buildImageActionReadinessRequest(feature)
  }
}

export async function fetchCanvasBillingReadiness(
  feature: WorkflowFeature,
  request?: SubmitTaskData,
): Promise<SubmitTaskReadinessData> {
  const response = await taskApi.checkSubmitReadiness(
    withTaskClientActionId(
      request ?? buildCanvasBillingReadinessRequest(feature),
      createCanvasTaskClientActionId(),
    ),
  )
  if (response.code !== 0 || !response.data) {
    throw new Error(sanitizeApiErrorMessage(response.msg, 'Idea计费信息暂不可用'))
  }
  return response.data
}

export function buildFusionBillingReadinessRequest(input: {
  imageUrls: string[]
  aspectRatio: Parameters<typeof buildFusionTaskRequest>[0]['aspectRatio']
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
    sizeSource: null,
    targetSize: null,
    aspectRatio: input.aspectRatio,
  })
}
