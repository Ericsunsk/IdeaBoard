import { computed, type Ref } from 'vue'
import type { NodeProps } from '@vue-flow/core'
import {
  ACTIVE_NODE_PANEL_KIND,
  type useFlowActiveNodePanels,
} from '../useFlowActiveNodePanels'
import type { useImageStageRegistry } from '../../vue-flow-adapter/useImageStageRegistry'
import {
  createFusionNodePresentationResolver,
  createImageNodePresentationResolver,
} from '../../node-presentation'
import type { CanvasWorkspaceRuntime } from '../../workspace-runtime'
import type { FlowNodeData } from '../../types/flowTypes'
import { createCanvasNodePresentationIntentRouter } from '../createCanvasNodePresentationIntentRouter'

type ActivePanels = ReturnType<typeof useFlowActiveNodePanels>
type ImageStages = ReturnType<typeof useImageStageRegistry>

export function createCanvasScreenNodePresentation(options: {
  workspace: CanvasWorkspaceRuntime
  activePanels: ActivePanels
  imageStages: ImageStages
  duplicateModifierPressed: Ref<boolean>
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

    return {
      nodeId: node.id,
      data: node.data,
      selected: node.selected,
      dragging: node.dragging,
      copyMode: options.duplicateModifierPressed.value,
      uiState: commands.graphEditing.getFlowNodeUiStateCached(node.id),
      imagePresentation: imagePresentationFor(node.id),
      fusionPresentation: fusionPresentationFor(node.id),
      generationAvailable: isGenerationAvailable.value,
      setImageStageRef: options.imageStages.setImageStageRef,
      clearImageStageRef: options.imageStages.clearImageStageRef,
      outpaintModeActive: isOutpaintModeActive,
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
