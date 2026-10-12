import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { createCanvasScreenNodePresentation } from './createCanvasScreenNodePresentation'

function createPresentation(getFlowNodeUiStateCached: ReturnType<typeof vi.fn>) {
  const noop = () => undefined
  const workspace = {
    viewModel: {
      imageWorkflow: {
        tools: {
          nodeBindings: {
            actionFor: () => ({ isDirectGenerating: false }),
            stageFor: () => null,
          },
          active: { nodeId: ref(null), maskNodeId: ref(null) },
        },
        outpaint: { nodeId: ref(null), overlayStyle: ref(null), handleRadius: 4 },
      },
    },
    commands: {
      imageWorkflow: {
        isGenerationAvailable: () => true,
        interactions: {
          onFlowPromptGenerate: noop,
          onFlowImageContextMenu: noop,
          onFlowImageDirectAction: noop,
          onFlowImageOutpaintSubmit: noop,
          onFlowOutpaintHandlePointerDown: noop,
          onFlowOutpaintFramePointerDown: noop,
          onFlowOutpaintHandlePointerMove: noop,
          onFlowOutpaintHandlePointerUp: noop,
          onFlowFusionContextMenu: noop,
          onFlowFusionPromptGenerate: noop,
        },
        tools: {},
      },
      graphEditing: {
        getFlowNodeUiStateCached,
        imageImport: { retry: async () => false },
        onFlowMediaInputImageDrop: noop,
        onFlowMediaInputImageOpen: noop,
        onFlowMediaInputImagePreview: noop,
        onFlowMediaInputImageRemove: noop,
        onFlowImageOpenPreview: noop,
        onFlowImageAppendToCanvas: noop,
      },
    },
  }
  return createCanvasScreenNodePresentation({
    workspace: workspace as never,
    activePanels: { isActiveNodePanel: () => false } as never,
    imageStages: { setImageStageRef: noop, clearImageStageRef: noop } as never,
    isDuplicateModifierPressed: ref(false),
    nodeInteractions: {
      onFlowPromptTextUpdate: noop,
      onFlowNodeParamUpdate: noop,
      onFlowDeleteNode: noop,
      onFlowImageStageDrop: noop,
      onFlowImageOpenFile: noop,
      onFlowImageReset: noop,
    } as never,
  })
}

describe('createCanvasScreenNodePresentation node slot scope', () => {
  it('does not resolve node uiState in the parent render effect', () => {
    const getFlowNodeUiStateCached = vi.fn(() => ({
      connection: { hasAnyConnection: false, connected: {} },
    }))
    const presentation = createPresentation(getFlowNodeUiStateCached)

    const props = presentation.nodePropsFor({
      id: 'prompt-1',
      data: { kind: 'prompt', snapshotNode: null },
      isSelected: false,
      isDragging: false,
    } as never)

    // The shared ui-state cache covers all nodes/edges: touching it here
    // would subscribe the CanvasScreen slot scope (all subtrees) to every
    // node's change. Each FlowCanvasNode resolves its own entry instead.
    expect(getFlowNodeUiStateCached).not.toHaveBeenCalled()
    expect(props).not.toHaveProperty('uiState')
  })
})
