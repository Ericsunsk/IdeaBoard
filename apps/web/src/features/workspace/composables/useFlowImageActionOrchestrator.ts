import type { Ref } from 'vue'
import {
  IMAGE_NODE_ACTION,
  isImageStrengthAction,
  type ImageDirectTaskAction,
  type ImageNodeAction,
  type ImageStrengthAction,
} from '../types/imageActions'

type DirectActionKey = ImageDirectTaskAction | ImageStrengthAction

type UseFlowImageActionOrchestratorOptions = {
  flowMaskEditorNodeId: Ref<string | null>
  activeImageActionNodeId: Ref<string | null>
  activeImagePromptNodeId: Ref<string | null>
  activeFusionPromptNodeId: Ref<string | null>
  suppressClickAction: Ref<ImageStrengthAction | null>
  clearActiveNodePanels: (options?: { keepFusionPrompt?: boolean }) => void
  closeActiveImagePromptComposer: () => void
  hideStrengthControl: () => void
  selectFlowNodeForContextMenu: (nodeId: string) => void
  cancelFlowExpandMode: () => void
  cancelFlowMaskEditorMode: () => void
  enterFlowExpandMode: (nodeId: string) => boolean
  enterFlowMaskEditorMode: (nodeId: string) => boolean
  focusMaskEditorAndShowChrome: (nodeId: string) => void | Promise<void>
  runImageDirectAction: (
    nodeId: string,
    action: DirectActionKey,
    options?: { strength?: number },
  ) => Promise<void> | void
  runImagePromptGeneration: (
    nodeId: string,
    prompt: string,
    outputCount: number,
  ) => Promise<void> | void
  runImageInpaintAction: (
    nodeId: string,
    options: {
      prompt: string
      outputCount: number
      maskDataUrl: string
      referenceImageUrls?: string[]
    },
  ) => Promise<void> | void
  runFusionPromptGeneration: (
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: string,
  ) => Promise<void> | void
  getFlowMaskEditorCommittedData: (
    nodeId: string,
  ) => { executionMaskDataUrl?: string | null } | null | undefined
  onFlowNodeParamUpdate: (
    nodeId: string,
    patch: { key: 'prompt' | 'outputCount' | 'aspectRatio'; value: string | number },
  ) => void
  onWarning: (message: string) => void
  getActionStrength: (action: ImageStrengthAction) => number
}

export function useFlowImageActionOrchestrator(options: UseFlowImageActionOrchestratorOptions) {
  function onFlowImageContextMenu(nodeId: string, event: MouseEvent) {
    event.preventDefault()
    if (options.flowMaskEditorNodeId.value === nodeId) return
    options.cancelFlowExpandMode()
    options.selectFlowNodeForContextMenu(nodeId)
    options.activeImageActionNodeId.value = nodeId
    options.activeImagePromptNodeId.value = nodeId
    options.activeFusionPromptNodeId.value = null
    options.hideStrengthControl()
  }

  function onFlowFusionContextMenu(nodeId: string, event: MouseEvent) {
    event.preventDefault()
    options.cancelFlowExpandMode()
    options.selectFlowNodeForContextMenu(nodeId)
    options.clearActiveNodePanels({ keepFusionPrompt: true })
    options.activeFusionPromptNodeId.value = nodeId
    options.hideStrengthControl()
  }

  function onFlowImageDirectAction(nodeId: string, action: ImageNodeAction) {
    if (action === IMAGE_NODE_ACTION.imageExtend) {
      options.cancelFlowMaskEditorMode()
      const entered = options.enterFlowExpandMode(nodeId)
      options.clearActiveNodePanels()
      options.hideStrengthControl()
      if (!entered) options.onWarning('当前图片不可扩图')
      return
    }
    if (action === IMAGE_NODE_ACTION.imageInpaint) {
      options.cancelFlowExpandMode()
      options.closeActiveImagePromptComposer()
      const entered = options.enterFlowMaskEditorMode(nodeId)
      options.activeImageActionNodeId.value = null
      options.activeFusionPromptNodeId.value = null
      options.hideStrengthControl()
      if (!entered) options.onWarning('当前图片不可局部重绘')
      else void options.focusMaskEditorAndShowChrome(nodeId)
      return
    }
    if (isImageStrengthAction(action) && options.suppressClickAction.value === action) {
      return
    }
    options.cancelFlowExpandMode()
    options.cancelFlowMaskEditorMode()
    options.clearActiveNodePanels()
    options.hideStrengthControl()
    void options.runImageDirectAction(
      nodeId,
      action,
      isImageStrengthAction(action) ? { strength: options.getActionStrength(action) } : undefined,
    )
  }

  function onFlowImagePromptGenerate(nodeId: string, prompt: string, outputCount: number) {
    options.cancelFlowExpandMode()
    options.cancelFlowMaskEditorMode()
    options.clearActiveNodePanels()
    options.hideStrengthControl()
    const committedMaskData = options.getFlowMaskEditorCommittedData(nodeId)
    if (committedMaskData?.executionMaskDataUrl) {
      void options.runImageInpaintAction(nodeId, {
        prompt,
        outputCount,
        maskDataUrl: committedMaskData.executionMaskDataUrl,
      })
      return
    }
    void options.runImagePromptGeneration(nodeId, prompt, outputCount)
  }

  function onFlowFusionPromptGenerate(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: string,
  ) {
    options.cancelFlowExpandMode()
    options.cancelFlowMaskEditorMode()
    options.clearActiveNodePanels()
    options.hideStrengthControl()
    options.onFlowNodeParamUpdate(nodeId, { key: 'prompt', value: prompt })
    options.onFlowNodeParamUpdate(nodeId, { key: 'outputCount', value: outputCount })
    options.onFlowNodeParamUpdate(nodeId, { key: 'aspectRatio', value: aspectRatio })
    void options.runFusionPromptGeneration(nodeId, prompt, outputCount, aspectRatio)
  }

  return {
    onFlowImageContextMenu,
    onFlowFusionContextMenu,
    onFlowImageDirectAction,
    onFlowImagePromptGenerate,
    onFlowFusionPromptGenerate,
  }
}
