import type { PromptAspectRatio } from '../../../../board/ports/canvas'
import type { ImageTaskAction } from '../../../core/execution/imageActionExecution.ts'
import {
  isImageExecutionPlanError,
  type ActivatedImageExecutionPlan,
  type ImageExecutionInput,
  type ImageExecutionPlan,
} from '../planning/imageExecutionPlan'
import type { OutpaintRect } from '../../../tools/outpaint/core'
import { createCanvasActivationId } from '../task-runner/taskClientActionIds'
import { i18n } from '../../../../../core/i18n'
import {
  isCanvasWorkspaceTaskUnmappableError,
  type CanvasWorkspaceTaskUnmappableError,
} from '../workspace/canvasTaskContractMapping'

type RunImageDirectActionOptions = {
  prompt?: string
  outputCount?: number
  strength?: number
}

type RunImageInpaintActionOptions = {
  prompt: string
  maskDataUrl: string
}

type OutpaintRectIntent = {
  frameRect: OutpaintRect
  originalImageRect: OutpaintRect
  prompt?: string
}

export type ImageWorkflowPlanner = {
  planImageDirectAction: (
    nodeId: string,
    action: ImageTaskAction,
    options?: RunImageDirectActionOptions,
  ) => ImageExecutionPlan
  planOutpaintAction: (nodeId: string, intent: OutpaintRectIntent) => ImageExecutionPlan
  planImageInpaintAction: (
    nodeId: string,
    inpaint: RunImageInpaintActionOptions,
  ) => ImageExecutionPlan
  planGeneration: (
    anchorNodeId: string,
    referenceImages: readonly ImageExecutionInput[],
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) => ImageExecutionPlan
  planFusionGeneration: (
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
  runOutpaintAction: (nodeId: string, intent: OutpaintRectIntent) => Promise<void>
  runImageInpaintAction: (nodeId: string, inpaint: RunImageInpaintActionOptions) => Promise<void>
  runFusionGeneration: (
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) => Promise<void>
  runGeneration: (
    anchorNodeId: string,
    referenceImages: readonly ImageExecutionInput[],
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) => Promise<void>
}

function unmappableWarningCopy(error: CanvasWorkspaceTaskUnmappableError): string {
  // Reference images that point at plain URLs or in-flight uploads have no
  // durable asset identity: tell the user what to do instead of echoing the
  // wire-shaped English message. Other reasons keep the fail-closed message
  // verbatim (same contract as the execution runner's warning copy).
  if (error.reason === 'media-ids' || error.reason === 'media-urls') {
    return i18n.global.t('canvas.execution.referenceImageMissingAsset')
  }
  return error.message
}

function warnPlanningError(error: unknown, onWarning: (message: string) => void): boolean {
  if (isCanvasWorkspaceTaskUnmappableError(error)) {
    onWarning(unmappableWarningCopy(error))
    return true
  }
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
    options.onWarning(
      options.generationUnavailableMessage?.() ??
        i18n.global.t('canvas.execution.generationUnavailable'),
    )
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

  async function runOutpaintAction(nodeId: string, intent: OutpaintRectIntent) {
    await executePlannedAction(() => options.planner.planOutpaintAction(nodeId, intent))
  }

  async function runImageInpaintAction(nodeId: string, inpaint: RunImageInpaintActionOptions) {
    await executePlannedAction(() => options.planner.planImageInpaintAction(nodeId, inpaint))
  }

  async function runGeneration(
    anchorNodeId: string,
    referenceImages: readonly ImageExecutionInput[],
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) {
    await executePlannedAction(() =>
      options.planner.planGeneration(
        anchorNodeId,
        referenceImages,
        prompt,
        outputCount,
        aspectRatio,
      ),
    )
  }

  async function runFusionGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) {
    await executePlannedAction(() =>
      options.planner.planFusionGeneration(nodeId, prompt, outputCount, aspectRatio),
    )
  }

  return {
    isGenerationAvailable,
    isNodeExecutionActive: options.workflow.isNodeExecutionActive,
    reattachPendingImageResults: options.workflow.reattachPendingImageResults,
    runImageDirectAction,
    runOutpaintAction,
    runImageInpaintAction,
    runFusionGeneration,
    runGeneration,
  }
}
