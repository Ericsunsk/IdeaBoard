import {
  WORKFLOW_FEATURE,
  type WorkflowFeature,
} from '../../../../../shared/domain/workflowFeatures'
import type {
  SubmitTaskData,
  SubmitTaskReadinessData,
} from '../../../domain/execution-contracts/canvasTaskRequest'
import {
  buildFusionTaskRequest,
  buildImageActionReadinessRequest,
  buildInpaintReadinessRequest,
  buildGenerationTaskRequest,
} from '../commands/imageTaskRequests'
import type { OutputResolution } from '../../../../../shared/domain/generationContract'
import {
  createCanvasTaskClientActionId,
  withTaskClientActionId,
} from '../task-runner/taskClientActionIds'
import type { SubmitTaskReadinessMatrixRequest } from '../../../domain/execution-contracts/canvasTaskRequest'
import {
  fetchCanvasWorkspaceTaskReadiness,
  fetchCanvasWorkspaceTaskReadinessMatrix,
} from '../workspace/canvasWorkspaceTaskAdapter'

function buildCanvasBillingReadinessRequest(feature: WorkflowFeature): SubmitTaskData {
  switch (feature) {
    case WORKFLOW_FEATURE.inpaint:
      return buildInpaintReadinessRequest()
    case WORKFLOW_FEATURE.generation:
      return buildGenerationTaskRequest({
        prompt: '',
        aspectRatio: 'auto',
        outputCount: 1,
      })
    case WORKFLOW_FEATURE.fusion:
      throw new Error('Fusion readiness requires current image inputs')
    case WORKFLOW_FEATURE.cutout:
    case WORKFLOW_FEATURE.variations:
    case WORKFLOW_FEATURE.render:
    case WORKFLOW_FEATURE.upscale:
    case WORKFLOW_FEATURE.outpaint:
      return buildImageActionReadinessRequest(feature)
  }
}

export async function fetchCanvasBillingReadiness(
  feature: WorkflowFeature,
  request?: SubmitTaskData,
): Promise<SubmitTaskReadinessData> {
  return fetchCanvasWorkspaceTaskReadiness(
    withTaskClientActionId(
      request ?? buildCanvasBillingReadinessRequest(feature),
      createCanvasTaskClientActionId(),
    ),
  )
}

export async function fetchCanvasBillingReadinessMatrix(
  requests: NonNullable<SubmitTaskReadinessMatrixRequest['requests']>,
): Promise<SubmitTaskReadinessData[]> {
  if (requests.length === 0) return []
  return fetchCanvasWorkspaceTaskReadinessMatrix(requests)
}

export function buildFusionBillingReadinessRequest(input: {
  imageUrls: string[]
  aspectRatio: Parameters<typeof buildFusionTaskRequest>[0]['aspectRatio']
  outputCount?: Parameters<typeof buildFusionTaskRequest>[0]['outputCount']
  outputResolution?: OutputResolution | undefined
}): SubmitTaskData | null {
  const imageUrls = input.imageUrls
    .map((url) => url.trim())
    .filter(Boolean)
    .slice(0, 4)
  if (imageUrls.length < 2) return null
  return buildFusionTaskRequest({
    prompt: '[fusion] submit readiness',
    imageUrls,
    inputWeights: imageUrls.map(() => 1 / imageUrls.length),
    outputCount: input.outputCount,
    sizeSource: null,
    targetSize: null,
    aspectRatio: input.aspectRatio,
    outputResolution: input.outputResolution,
  })
}
