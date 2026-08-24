import type { Ref } from 'vue'
import {
  IMAGE_ACTION_MODE,
  IMAGE_ACTION_REGISTRY,
  type ImageNodeAction,
} from '../config/imageActionRegistry'
import {
  isImageTaskAction,
  isImageStrengthAction,
  type ImageStrengthAction,
  type ImageTaskAction,
} from '../execution'
import type { PromptAspectRatio } from '../../canvas-board/domain'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'
import {
  ACTIVE_NODE_PANEL_KIND,
  type ActiveNodePanel,
} from '../screen-presentation/useFlowActiveNodePanels'
import type { ImageEditorMode } from '../image-editor'

type FusionPromptParamPatch = Extract<
  FlowSnapshotParamPatch,
  { key: 'prompt' | 'outputCount' | 'aspectRatio' }
>

export type ImageActionIntentRouterOptions = {
  flowImageEditorNodeId: Ref<string | null>
  consumeSuppressedStrengthClick: (action: ImageStrengthAction) => boolean
  openActiveNodePanel: (panel: ActiveNodePanel) => void
  clearActiveNodePanel: () => void
  hideStrengthControl: () => void
  selectFlowNodeForContextMenu: (nodeId: string) => void
  cancelFlowExpandMode: () => void
  cancelFlowMaskEditorMode: () => void
  enterFlowExpandMode: (nodeId: string) => boolean
  enterFlowImageEditorMode: (nodeId: string, mode: ImageEditorMode) => boolean
  focusImageEditorAndShowChrome: (nodeId: string) => void | Promise<void>
  runImageDirectAction: (
    nodeId: string,
    action: ImageTaskAction,
    options?: { prompt?: string; outputCount?: number; strength?: number },
  ) => Promise<void> | void
  runFusionPromptGeneration: (
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) => Promise<void> | void
  onFlowNodeParamUpdate: (nodeId: string, patch: FusionPromptParamPatch) => void
  onWarning: (message: string) => void
  getActionStrength: (action: ImageStrengthAction) => number
}

export function createImageActionIntentRouter(options: ImageActionIntentRouterOptions) {
  function onFlowImageContextMenu(nodeId: string, event: MouseEvent) {
    event.preventDefault()
    if (options.flowImageEditorNodeId.value === nodeId) return
    options.cancelFlowExpandMode()
    options.selectFlowNodeForContextMenu(nodeId)
    options.openActiveNodePanel({ kind: ACTIVE_NODE_PANEL_KIND.imageTools, nodeId })
    options.hideStrengthControl()
  }

  function onFlowFusionContextMenu(nodeId: string, event: MouseEvent) {
    event.preventDefault()
    options.cancelFlowExpandMode()
    options.selectFlowNodeForContextMenu(nodeId)
    options.openActiveNodePanel({ kind: ACTIVE_NODE_PANEL_KIND.fusionPrompt, nodeId })
    options.hideStrengthControl()
  }

  function onFlowImageDirectAction(
    nodeId: string,
    action: ImageNodeAction,
    prompt = '',
    outputCount = 1,
    strength?: number,
  ) {
    const actionMode = IMAGE_ACTION_REGISTRY[action].mode
    if (actionMode === IMAGE_ACTION_MODE.canvasExtend) {
      options.cancelFlowMaskEditorMode()
      const entered = options.enterFlowExpandMode(nodeId)
      options.clearActiveNodePanel()
      options.hideStrengthControl()
      if (!entered) options.onWarning('当前图片不可扩图')
      return
    }
    if (
      actionMode === IMAGE_ACTION_MODE.partialEdit ||
      actionMode === IMAGE_ACTION_MODE.localImageEditor
    ) {
      options.cancelFlowExpandMode()
      const editorMode = actionMode === IMAGE_ACTION_MODE.localImageEditor ? 'adjust' : 'inpaint'
      const entered = options.enterFlowImageEditorMode(nodeId, editorMode)
      options.clearActiveNodePanel()
      options.hideStrengthControl()
      if (!entered) {
        options.onWarning(
          actionMode === IMAGE_ACTION_MODE.localImageEditor
            ? '当前图片不可编辑'
            : '当前图片不可局部重绘',
        )
      } else {
        void options.focusImageEditorAndShowChrome(nodeId)
      }
      return
    }
    if (!isImageTaskAction(action)) {
      options.onWarning('当前图片工具不可执行')
      return
    }
    if (
      strength === undefined &&
      isImageStrengthAction(action) &&
      options.consumeSuppressedStrengthClick(action)
    ) {
      return
    }
    options.cancelFlowExpandMode()
    options.cancelFlowMaskEditorMode()
    options.clearActiveNodePanel()
    options.hideStrengthControl()
    void options.runImageDirectAction(
      nodeId,
      action,
      {
        prompt,
        outputCount,
        ...(isImageStrengthAction(action)
          ? { strength: strength ?? options.getActionStrength(action) }
          : {}),
      },
    )
  }

  function onFlowFusionPromptGenerate(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) {
    options.cancelFlowExpandMode()
    options.cancelFlowMaskEditorMode()
    options.clearActiveNodePanel()
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
    onFlowFusionPromptGenerate,
  }
}
