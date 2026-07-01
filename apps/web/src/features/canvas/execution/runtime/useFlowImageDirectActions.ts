import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { PromptAspectRatio } from '../../../canvas-board/domain'
import type { FlowTransactions } from '../../composables/useFlowTransactions'
import type { CanvasExtendRect } from '../../utils/canvasExtendGeometry'
import {
  createImageExecutionPlanner,
  isImageExecutionPlanError,
  type ImageExecutionInput,
  type ImageResultAnchor,
} from '../planning/ImageExecutionPlan'
import type { ImageTaskAction } from '../contracts/imageActionExecution'
import { useFlowImageWorkflowExecution } from './flowImageWorkflowExecution'

type RunImageDirectActionOptions = {
  strength?: number
}

type RunImageInpaintActionOptions = {
  prompt: string
  maskDataUrl: string
  outputCount: number
  referenceImageUrls?: string[]
}

type UseFlowImageDirectActionsOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  commitGraphMutation: FlowTransactions['commitGraphMutation']
  commitNodeMutation: FlowTransactions['commitNodeMutation']
  commitEdgeMutation: FlowTransactions['commitEdgeMutation']
  commitDirty: FlowTransactions['commitDirty']
  refreshNodeInternals?: (nodeIds: string[]) => void
  flushAutosave?: (options?: { silent?: boolean }) => Promise<void>
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  getCurrentBoardId?: () => string
}

type CanvasExtendRectIntent = {
  frameRect: CanvasExtendRect
  originalImageRect: CanvasExtendRect
  prompt?: string
  outputCount?: number
}

function warnPlanningError(error: unknown, onWarning: (message: string) => void): boolean {
  if (!isImageExecutionPlanError(error)) return false
  onWarning(error.message)
  return true
}

export function useFlowImageDirectActions(options: UseFlowImageDirectActionsOptions) {
  const planner = createImageExecutionPlanner(() => ({
    nodes: options.nodes.value,
    edges: options.edges.value,
  }))
  const { executeImageWorkflow, isNodeDirectActionRunning, reattachPendingImageResults } =
    useFlowImageWorkflowExecution({
      nodes: options.nodes,
      edges: options.edges,
      commitGraphMutation: options.commitGraphMutation,
      commitNodeMutation: options.commitNodeMutation,
      commitEdgeMutation: options.commitEdgeMutation,
      refreshNodeInternals: options.refreshNodeInternals,
      flushAutosave: options.flushAutosave,
      getCurrentBoardId: options.getCurrentBoardId,
      onSuccess: options.onSuccess,
      onWarning: options.onWarning,
    })

  async function runImageDirectAction(
    nodeId: string,
    action: ImageTaskAction,
    runOptions?: RunImageDirectActionOptions,
  ) {
    try {
      const plan = planner.planImageDirectAction(nodeId, action, runOptions)
      await executeImageWorkflow(plan)
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  async function runCanvasExtendAction(nodeId: string, intent: CanvasExtendRectIntent) {
    try {
      const plan = planner.planCanvasExtendAction(nodeId, intent)
      await executeImageWorkflow(plan)
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  async function runImageInpaintAction(nodeId: string, inpaint: RunImageInpaintActionOptions) {
    try {
      const plan = planner.planImageInpaintAction(nodeId, inpaint)
      await executeImageWorkflow(plan)
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  async function runImagePromptGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio = 'auto',
    optionsOverride: { outputAnchorNodeId?: string } = {},
  ) {
    try {
      const plan = planner.planImagePromptGeneration(
        nodeId,
        prompt,
        outputCount,
        aspectRatio,
        optionsOverride,
      )
      await runImagePromptGenerationPlan(plan)
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  async function runImagePromptGenerationFromSource(
    source: ImageExecutionInput,
    sourceNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
    optionsOverride: { outputAnchorNodeId?: string; anchor?: ImageResultAnchor } = {},
  ) {
    try {
      const plan = planner.planImagePromptGenerationFromSource(
        source,
        sourceNodeId,
        prompt,
        outputCount,
        aspectRatio,
        optionsOverride,
      )
      await runImagePromptGenerationPlan(plan)
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  async function runImagePromptGenerationPlan(
    plan: ReturnType<typeof planner.planImagePromptGenerationFromSource>,
  ) {
    await executeImageWorkflow(plan)
  }

  async function runTextPromptGeneration(
    anchorNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) {
    try {
      const plan = planner.planTextPromptGeneration(anchorNodeId, prompt, outputCount, aspectRatio)
      await executeImageWorkflow(plan)
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  async function runFusionPromptGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) {
    try {
      const plan = planner.planFusionPromptGeneration(nodeId, prompt, outputCount, aspectRatio)
      await executeImageWorkflow(plan)
    } catch (error) {
      if (!warnPlanningError(error, options.onWarning)) throw error
    }
  }

  return {
    isNodeDirectActionRunning,
    reattachPendingImageResults,
    runImageDirectAction,
    runCanvasExtendAction,
    runImagePromptGeneration,
    runImagePromptGenerationFromSource,
    runImageInpaintAction,
    runFusionPromptGeneration,
    runTextPromptGeneration,
  }
}
