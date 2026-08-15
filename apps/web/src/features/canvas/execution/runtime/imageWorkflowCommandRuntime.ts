import type { PromptAspectRatio } from '../../../canvas-board/domain'
import type { ImageTaskAction } from '../contracts/imageActionExecution'
import {
  isImageExecutionPlanError,
  type ActivatedImageExecutionPlan,
  type ImageExecutionInput,
  type ImageExecutionPlan,
} from '../planning/ImageExecutionPlan'
import type { CanvasExtendRect } from '../../utils/canvasExtendGeometry'
import { createCanvasActivationId } from '../task-runner/taskClientActionIds'

export type RunImageDirectActionOptions = {
  prompt?: string
  outputCount?: number
  strength?: number
}

export type RunImageInpaintActionOptions = {
  prompt: string
  maskDataUrl: string
  outputCount: number
  referenceImageUrls?: string[]
}

export type CanvasExtendRectIntent = {
  frameRect: CanvasExtendRect
  originalImageRect: CanvasExtendRect
  prompt?: string
  outputCount?: number
}

export type ImageWorkflowPlanner = {
  planImageDirectAction: (
    nodeId: string,
    action: ImageTaskAction,
    options?: RunImageDirectActionOptions,
  ) => ImageExecutionPlan
  planCanvasExtendAction: (nodeId: string, intent: CanvasExtendRectIntent) => ImageExecutionPlan
  planImageInpaintAction: (
    nodeId: string,
    inpaint: RunImageInpaintActionOptions,
  ) => ImageExecutionPlan
  planPromptGeneration: (
    anchorNodeId: string,
    referenceImages: readonly ImageExecutionInput[],
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) => ImageExecutionPlan
  planFusionPromptGeneration: (
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) => ImageExecutionPlan
}

export type ImageWorkflowExecutionPort = {
  executeImageWorkflow: (plan: ActivatedImageExecutionPlan) => Promise<void>
  isNodeExecutionActive: (nodeId: string) => boolean
  reattachPendingImageResults: () => Promise<void> | void
}

export type ImageWorkflowCommands = {
  isGenerationAvailable: () => boolean
  isNodeExecutionActive: ImageWorkflowExecutionPort['isNodeExecutionActive']
  reattachPendingImageResults: ImageWorkflowExecutionPort['reattachPendingImageResults']
  runImageDirectAction: (
    nodeId: string,
    action: ImageTaskAction,
    options?: RunImageDirectActionOptions,
  ) => Promise<void>
  runCanvasExtendAction: (nodeId: string, intent: CanvasExtendRectIntent) => Promise<void>
  runImageInpaintAction: (nodeId: string, inpaint: RunImageInpaintActionOptions) => Promise<void>
  runFusionPromptGeneration: (
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) => Promise<void>
  runPromptGeneration: (
    anchorNodeId: string,
    referenceImages: readonly ImageExecutionInput[],
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) => Promise<void>
}

function warnPlanningError(error: unknown, onWarning: (message: string) => void): boolean {
  if (!isImageExecutionPlanError(error)) return false
  onWarning(error.message)
  return true
}

export function createImageWorkflowCommands(options: {
  planner: ImageWorkflowPlanner
  workflow: ImageWorkflowExecutionPort
  isGenerationAvailable?: () => boolean
  generationUnavailableMessage?: () => string
  onWarning: (message: string) => void
}): ImageWorkflowCommands {
  function isGenerationAvailable() {
    return options.isGenerationAvailable?.() ?? true
  }

  function warnGenerationUnavailable() {
    options.onWarning(options.generationUnavailableMessage?.() ?? '生成服务暂不可用，请稍后重试')
  }

  async function executePlannedAction(planAction: () => ImageExecutionPlan) {
    if (!isGenerationAvailable()) {
      warnGenerationUnavailable()
      return
    }
    try {
      const plan = planAction()
      const activatedPlan: ActivatedImageExecutionPlan = {
        ...plan,
        activationId: createCanvasActivationId(),
      }
      await options.workflow.executeImageWorkflow(activatedPlan)
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  async function runImageDirectAction(
    nodeId: string,
    action: ImageTaskAction,
    runOptions?: RunImageDirectActionOptions,
  ) {
    await executePlannedAction(() =>
      options.planner.planImageDirectAction(nodeId, action, runOptions),
    )
  }

  async function runCanvasExtendAction(nodeId: string, intent: CanvasExtendRectIntent) {
    await executePlannedAction(() => options.planner.planCanvasExtendAction(nodeId, intent))
  }

  async function runImageInpaintAction(nodeId: string, inpaint: RunImageInpaintActionOptions) {
    await executePlannedAction(() => options.planner.planImageInpaintAction(nodeId, inpaint))
  }

  async function runPromptGeneration(
    anchorNodeId: string,
    referenceImages: readonly ImageExecutionInput[],
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) {
    await executePlannedAction(() =>
      options.planner.planPromptGeneration(
        anchorNodeId,
        referenceImages,
        prompt,
        outputCount,
        aspectRatio,
      ),
    )
  }

  async function runFusionPromptGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) {
    await executePlannedAction(() =>
      options.planner.planFusionPromptGeneration(nodeId, prompt, outputCount, aspectRatio),
    )
  }

  return {
    isGenerationAvailable,
    isNodeExecutionActive: options.workflow.isNodeExecutionActive,
    reattachPendingImageResults: options.workflow.reattachPendingImageResults,
    runImageDirectAction,
    runCanvasExtendAction,
    runImageInpaintAction,
    runFusionPromptGeneration,
    runPromptGeneration,
  }
}
