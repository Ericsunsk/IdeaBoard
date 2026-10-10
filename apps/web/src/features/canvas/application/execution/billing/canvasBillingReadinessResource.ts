import { SERVICE, type Service } from '../../../../../core/contracts/imageServices'
import type {
  CanvasTaskRequestData,
  SubmitTaskReadinessData,
} from '../../../domain/execution-contracts/canvasTaskRequest'
import {
  buildFusionTaskRequest,
  buildImageActionReadinessRequest,
  buildInpaintReadinessRequest,
  buildGenerationTaskRequest,
} from '../commands/imageTaskRequests'
import type { OutputResolution } from '../../../../../core/contracts/generationContract'
import {
  createCanvasTaskClientActionId,
  withTaskClientActionId,
} from '../task-runner/taskClientActionIds'
import type { SubmitTaskReadinessMatrixRequest } from '../../../domain/execution-contracts/canvasTaskRequest'
import {
  fetchCanvasWorkspaceTaskReadiness,
  fetchCanvasWorkspaceTaskReadinessMatrix,
} from '../workspace/canvasWorkspaceTaskAdapter'

function buildCanvasBillingReadinessRequest(service: Service): CanvasTaskRequestData {
  switch (service) {
    case SERVICE.inpaint:
      return buildInpaintReadinessRequest()
    case SERVICE.generation:
      return buildGenerationTaskRequest({
        prompt: '',
        aspectRatio: 'auto',
        outputCount: 1,
      })
    case SERVICE.fusion:
      throw new Error('Fusion readiness requires current image inputs')
    case SERVICE.cutout:
    case SERVICE.variations:
    case SERVICE.render:
    case SERVICE.upscale:
    case SERVICE.outpaint:
      return buildImageActionReadinessRequest(service)
  }
}

export async function fetchCanvasBillingReadiness(
  service: Service,
  request?: CanvasTaskRequestData,
): Promise<SubmitTaskReadinessData> {
  return fetchCanvasWorkspaceTaskReadiness(
    withTaskClientActionId(
      request ?? buildCanvasBillingReadinessRequest(service),
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
}): CanvasTaskRequestData | null {
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
