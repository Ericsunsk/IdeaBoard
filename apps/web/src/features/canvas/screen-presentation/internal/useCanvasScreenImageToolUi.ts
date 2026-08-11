import type { Ref } from 'vue'
import { useFlowActiveNodePanels } from '../../composables/useFlowActiveNodePanels'
import { useImageStageRegistry } from '../../composables/useImageStageRegistry'
import type { CanvasImageWorkflowUiAdapter } from '../../image-workflow'

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
