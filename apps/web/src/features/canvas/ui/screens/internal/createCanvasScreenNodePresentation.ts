import { computed, type Ref } from 'vue'
import type { NodeProps } from '@vue-flow/core'
import { ACTIVE_NODE_PANEL_KIND, type useFlowActiveNodePanels } from '../useFlowActiveNodePanels'
import type { useImageStageRegistry } from '../../../session/flow-adapter'
import {
  createFusionNodePresentationResolver,
  createImageNodePresentationResolver,
} from '../../nodes'
import type { CanvasWorkspaceRuntime } from '../../../session/workspace'
import type { FlowNodeData } from '../../../core/nodes/flowTypes.ts'
import { createCanvasNodePresentationIntentRouter } from '../createCanvasNodePresentationIntentRouter'

type ActivePanels = ReturnType<typeof useFlowActiveNodePanels>
type ImageStages = ReturnType<typeof useImageStageRegistry>

export function createCanvasScreenNodePresentation(options: {
  workspace: CanvasWorkspaceRuntime
  activePanels: ActivePanels
  imageStages: ImageStages
  isDuplicateModifierPressed: Ref<boolean>
  nodeInteractions: ReturnType<
    CanvasWorkspaceRuntime['commands']['graphEditing']['createNodeInteractions']
  >
}) {
  const { viewModel, commands } = options.workspace
  const imageTools = {
    viewModel: viewModel.imageWorkflow.tools,
    commands: commands.imageWorkflow.tools,
  }
  const imagePresentationFor = createImageNodePresentationResolver({
    actionBindingFor: imageTools.viewModel.nodeBindings.actionFor,
    stageBindingFor: imageTools.viewModel.nodeBindings.stageFor,
  }).forNode
  const fusionPresentationFor = createFusionNodePresentationResolver({
    promptComposerVisibleFor: (nodeId) =>
      options.activePanels.isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.fusionPrompt, nodeId),
  }).forNode
  const isGenerationAvailable = computed(() => commands.imageWorkflow.isGenerationAvailable())
  const isCanvasShortcutSuppressed = computed(
    () =>
      !!imageTools.viewModel.active.nodeId.value || !!imageTools.viewModel.active.maskNodeId.value,
  )

  const interactions = commands.imageWorkflow.interactions
  const nodePresentationIntentRouter = createCanvasNodePresentationIntentRouter({
    prompt: {
      updateText: options.nodeInteractions.onFlowPromptTextUpdate,
      updateParameters: options.nodeInteractions.onFlowNodeParamUpdate,
      generate: interactions.onFlowPromptGenerate,
    },
    node: {
      delete: options.nodeInteractions.onFlowDeleteNode,
      retryImageImport: (nodeId) => {
        void commands.graphEditing.imageImport.retry(nodeId)
      },
    },
    mediaInput: {
      drop: commands.graphEditing.onFlowMediaInputImageDrop,
      open: commands.graphEditing.onFlowMediaInputImageOpen,
      preview: commands.graphEditing.onFlowMediaInputImagePreview,
      remove: commands.graphEditing.onFlowMediaInputImageRemove,
    },
    image: {
      dropStage: options.nodeInteractions.onFlowImageStageDrop,
      openFile: options.nodeInteractions.onFlowImageOpenFile,
      preview: commands.graphEditing.onFlowImageOpenPreview,
      appendToCanvas: commands.graphEditing.onFlowImageAppendToCanvas,
      reset: options.nodeInteractions.onFlowImageReset,
      openContextMenu: interactions.onFlowImageContextMenu,
      runAction: interactions.onFlowImageDirectAction,
    },
    outpaint: {
      submit: interactions.onFlowImageOutpaintSubmit,
      beginHandle: interactions.onFlowOutpaintHandlePointerDown,
      moveHandle: interactions.onFlowOutpaintHandlePointerMove,
      endHandle: interactions.onFlowOutpaintHandlePointerUp,
      beginFrame: interactions.onFlowOutpaintFramePointerDown,
    },
    fusion: {
      openContextMenu: interactions.onFlowFusionContextMenu,
      generate: interactions.onFlowFusionPromptGenerate,
    },
  })

  function nodePropsFor(node: NodeProps<FlowNodeData>) {
    const outpaint = viewModel.imageWorkflow.outpaint
    const isOutpaintModeActive = outpaint.nodeId.value === node.id

    // uiState is intentionally NOT resolved here: reading the shared ui-state
    // cache in the parent render effect would subscribe EVERY node slot to a
    // single computed covering all nodes/edges, so one node's change
    // re-renders all subtrees. Each FlowCanvasNode resolves its own uiState
    // from the injected canvas node-ui-state context instead.
    return {
      nodeId: node.id,
      data: node.data,
      isSelected: node.selected,
      isDragging: node.dragging,
      isCopyMode: options.isDuplicateModifierPressed.value,
      imagePresentation: imagePresentationFor(node.id),
      fusionPresentation: fusionPresentationFor(node.id),
      isGenerationAvailable: isGenerationAvailable.value,
      setImageStageRef: options.imageStages.setImageStageRef,
      clearImageStageRef: options.imageStages.clearImageStageRef,
      isOutpaintModeActive: isOutpaintModeActive,
      outpaintOverlayStyle: isOutpaintModeActive ? outpaint.overlayStyle.value : null,
      outpaintHandleRadius: outpaint.handleRadius,
    }
  }

  return {
    nodePresentationIntentRouter,
    nodePropsFor,
    isCanvasShortcutSuppressed,
  }
}
