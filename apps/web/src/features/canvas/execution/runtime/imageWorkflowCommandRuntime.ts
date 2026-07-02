import type { PromptAspectRatio } from '../../../canvas-board/domain'
import type { ImageTaskAction } from '../contracts/imageActionExecution'
import {
  isImageExecutionPlanError,
  type ImageExecutionInput,
  type ImageExecutionPlan,
  type ImageResultAnchor,
} from '../planning/ImageExecutionPlan'
import type { CanvasExtendRect } from '../../utils/canvasExtendGeometry'

export type RunImageDirectActionOptions = {
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
  planImagePromptGeneration: (
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
    optionsOverride?: { outputAnchorNodeId?: string },
  ) => ImageExecutionPlan
  planImagePromptGenerationFromSource: (
    source: ImageExecutionInput,
    sourceNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
    optionsOverride?: { outputAnchorNodeId?: string; anchor?: ImageResultAnchor },
  ) => ImageExecutionPlan
  planTextPromptGeneration: (
    anchorNodeId: string,
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
  executeImageWorkflow: (plan: ImageExecutionPlan) => Promise<void>
  isNodeDirectActionRunning: (nodeId: string) => boolean
  reattachPendingImageResults: () => Promise<void> | void
}

export type ImageWorkflowCommands = {
  isNodeDirectActionRunning: ImageWorkflowExecutionPort['isNodeDirectActionRunning']
  reattachPendingImageResults: ImageWorkflowExecutionPort['reattachPendingImageResults']
  runImageDirectAction: (
    nodeId: string,
    action: ImageTaskAction,
    options?: RunImageDirectActionOptions,
  ) => Promise<void>
  runCanvasExtendAction: (nodeId: string, intent: CanvasExtendRectIntent) => Promise<void>
  runImagePromptGeneration: (
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio?: PromptAspectRatio,
    optionsOverride?: { outputAnchorNodeId?: string },
  ) => Promise<void>
  runImagePromptGenerationFromSource: (
    source: ImageExecutionInput,
    sourceNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
    optionsOverride?: { outputAnchorNodeId?: string; anchor?: ImageResultAnchor },
  ) => Promise<void>
  runImageInpaintAction: (
    nodeId: string,
    inpaint: RunImageInpaintActionOptions,
  ) => Promise<void>
  runFusionPromptGeneration: (
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) => Promise<void>
  runTextPromptGeneration: (
    anchorNodeId: string,
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
  onWarning: (message: string) => void
}): ImageWorkflowCommands {
  async function executePlannedAction(planAction: () => ImageExecutionPlan) {
    try {
      await options.workflow.executeImageWorkflow(planAction())
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

  async function runImagePromptGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio = 'auto',
    optionsOverride: { outputAnchorNodeId?: string } = {},
  ) {
    await executePlannedAction(() =>
      options.planner.planImagePromptGeneration(
        nodeId,
        prompt,
        outputCount,
        aspectRatio,
        optionsOverride,
      ),
    )
  }

  async function runImagePromptGenerationFromSource(
    source: ImageExecutionInput,
    sourceNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
    optionsOverride: { outputAnchorNodeId?: string; anchor?: ImageResultAnchor } = {},
  ) {
    await executePlannedAction(() =>
      options.planner.planImagePromptGenerationFromSource(
        source,
        sourceNodeId,
        prompt,
        outputCount,
        aspectRatio,
        optionsOverride,
      ),
    )
  }

  async function runTextPromptGeneration(
    anchorNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) {
    await executePlannedAction(() =>
      options.planner.planTextPromptGeneration(anchorNodeId, prompt, outputCount, aspectRatio),
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
    isNodeDirectActionRunning: options.workflow.isNodeDirectActionRunning,
    reattachPendingImageResults: options.workflow.reattachPendingImageResults,
    runImageDirectAction,
    runCanvasExtendAction,
    runImagePromptGeneration,
    runImagePromptGenerationFromSource,
    runImageInpaintAction,
    runFusionPromptGeneration,
    runTextPromptGeneration,
  }
}
