import type { Ref } from 'vue'
import { useFlowActiveNodePanels } from '../useFlowActiveNodePanels'
import { useImageStageRegistry } from '../../../infrastructure/vue-flow-adapter/useImageStageRegistry.ts'
import type { CanvasImageWorkflowUiAdapter } from '../../../application/image-workflow'

export function useCanvasScreenImageToolUi(options: {
  activeTool: Ref<'select' | 'hand'>
  canvasWrapRef: Ref<HTMLElement | null>
}) {
  const activePanels = useFlowActiveNodePanels()
  const imageStages = useImageStageRegistry()

  const imageWorkflowUi: CanvasImageWorkflowUiAdapter = {
    activeTool: options.activeTool,
    canvasWrapRef: options.canvasWrapRef,
    getImageStageElement: imageStages.getImageStageElement,
    panels: {
      open: activePanels.openActiveNodePanel,
      clear: activePanels.clearActiveNodePanel,
      isActive: activePanels.isActiveNodePanel,
      getActiveNodeId: activePanels.getActiveNodePanelNodeId,
    },
  }

  return {
    imageWorkflowUi,
    activePanels,
    imageStages,
  }
}
