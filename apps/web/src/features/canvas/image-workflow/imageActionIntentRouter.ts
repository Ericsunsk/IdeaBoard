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
import { i18n } from '../../../shared/i18n'
import {
  ACTIVE_NODE_PANEL_KIND,
  type ActiveNodePanel,
} from '../workspace-ui/useFlowActiveNodePanels'
import type { RetouchMode } from '../retouch'

type FusionPromptParamPatch = Extract<
  FlowSnapshotParamPatch,
  { key: 'prompt' | 'outputCount' | 'aspectRatio' }
>

export type ImageActionIntentRouterOptions = {
  flowRetouchNodeId: Ref<string | null>
  consumeSuppressedStrengthClick: (action: ImageStrengthAction) => boolean
  openActiveNodePanel: (panel: ActiveNodePanel) => void
  clearActiveNodePanel: () => void
  hideStrengthControl: () => void
  selectFlowNodeForContextMenu: (nodeId: string) => void
  cancelFlowOutpaintMode: () => void
  cancelFlowInpaintMode: () => void
  enterFlowOutpaintMode: (nodeId: string) => boolean
  enterFlowRetouchMode: (nodeId: string, mode: RetouchMode) => boolean
  focusRetouchAndShowChrome: (nodeId: string) => void | Promise<void>
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
    if (options.flowRetouchNodeId.value === nodeId) return
    options.cancelFlowOutpaintMode()
    options.selectFlowNodeForContextMenu(nodeId)
    options.openActiveNodePanel({ kind: ACTIVE_NODE_PANEL_KIND.imageTools, nodeId })
    options.hideStrengthControl()
  }

  function onFlowFusionContextMenu(nodeId: string, event: MouseEvent) {
    event.preventDefault()
    options.cancelFlowOutpaintMode()
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
    if (actionMode === IMAGE_ACTION_MODE.outpaint) {
      options.cancelFlowInpaintMode()
      const entered = options.enterFlowOutpaintMode(nodeId)
      options.clearActiveNodePanel()
      options.hideStrengthControl()
      if (!entered) options.onWarning(i18n.global.t('canvas.tools.cannotOutpaint'))
      return
    }
    if (
      actionMode === IMAGE_ACTION_MODE.inpaint ||
      actionMode === IMAGE_ACTION_MODE.retouch
    ) {
      options.cancelFlowOutpaintMode()
      const editorMode = actionMode === IMAGE_ACTION_MODE.retouch ? 'adjust' : 'inpaint'
      const entered = options.enterFlowRetouchMode(nodeId, editorMode)
      options.clearActiveNodePanel()
      options.hideStrengthControl()
      if (!entered) {
        const warningKey =
          actionMode === IMAGE_ACTION_MODE.retouch
            ? 'canvas.tools.imageNotEditable'
            : 'canvas.tools.cannotInpaint'
        options.onWarning(i18n.global.t(warningKey))
      } else {
        void options.focusRetouchAndShowChrome(nodeId)
      }
      return
    }
    if (!isImageTaskAction(action)) {
      options.onWarning(i18n.global.t('canvas.tools.actionUnavailable'))
      return
    }
    if (
      strength === undefined &&
      isImageStrengthAction(action) &&
      options.consumeSuppressedStrengthClick(action)
    ) {
      return
    }
    options.cancelFlowOutpaintMode()
    options.cancelFlowInpaintMode()
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
    options.cancelFlowOutpaintMode()
    options.cancelFlowInpaintMode()
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
