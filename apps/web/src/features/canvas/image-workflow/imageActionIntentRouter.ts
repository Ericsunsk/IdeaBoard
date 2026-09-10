import type { Ref } from 'vue'
import {
  IMAGE_ACTION_MODE,
  IMAGE_ACTION_REGISTRY,
  type ImageActionMode,
  type ImageNodeAction,
} from '../config/imageActionRegistry'
import {
  isImageTaskAction,
  isImageStrengthAction,
  type ImageStrengthAction,
  type ImageTaskAction,
} from '../execution'
import type { PromptAspectRatio } from '../../board/domain'
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
  state: {
    flowRetouchNodeId: Ref<string | null>
  }
  strength: {
    consumeSuppressedClick: (action: ImageStrengthAction) => boolean
    getValue: (action: ImageStrengthAction) => number
  }
  ui: {
    openPanel: (panel: ActiveNodePanel) => void
    clearPanel: () => void
    hideStrengthControl: () => void
    warn: (message: string) => void
  }
  selection: {
    selectNode: (nodeId: string) => void
  }
  modes: {
    cancelOutpaint: () => void
    cancelInpaint: () => void
    enterOutpaint: (nodeId: string) => boolean
    enterRetouch: (nodeId: string, mode: RetouchMode) => boolean
    focusRetouch: (nodeId: string) => void | Promise<void>
  }
  execution: {
    runImageDirectAction: (
      nodeId: string,
      action: ImageTaskAction,
      options?: { prompt?: string; outputCount?: number; strength?: number },
    ) => Promise<void> | void
    runFusionGeneration: (
      nodeId: string,
      prompt: string,
      outputCount: number,
      aspectRatio: PromptAspectRatio,
    ) => Promise<void> | void
  }
  params: {
    updateNodeParam: (nodeId: string, patch: FusionPromptParamPatch) => void
  }
}

export function createImageActionIntentRouter(options: ImageActionIntentRouterOptions) {
  function onFlowImageContextMenu(nodeId: string, event: MouseEvent) {
    event.preventDefault()
    if (options.state.flowRetouchNodeId.value === nodeId) return
    options.modes.cancelOutpaint()
    options.selection.selectNode(nodeId)
    options.ui.openPanel({ kind: ACTIVE_NODE_PANEL_KIND.imageTools, nodeId })
    options.ui.hideStrengthControl()
  }

  function onFlowFusionContextMenu(nodeId: string, event: MouseEvent) {
    event.preventDefault()
    options.modes.cancelOutpaint()
    options.selection.selectNode(nodeId)
    options.ui.openPanel({ kind: ACTIVE_NODE_PANEL_KIND.fusionPrompt, nodeId })
    options.ui.hideStrengthControl()
  }

  function enterOutpaintMode(nodeId: string): boolean {
    options.modes.cancelInpaint()
    const entered = options.modes.enterOutpaint(nodeId)
    options.ui.clearPanel()
    options.ui.hideStrengthControl()
    if (!entered) options.ui.warn(i18n.global.t('canvas.tools.cannotOutpaint'))
    return entered
  }

  function enterRetouchMode(nodeId: string, actionMode: ImageActionMode): boolean {
    options.modes.cancelOutpaint()
    const editorMode = actionMode === IMAGE_ACTION_MODE.retouch ? 'adjust' : 'inpaint'
    const entered = options.modes.enterRetouch(nodeId, editorMode)
    options.ui.clearPanel()
    options.ui.hideStrengthControl()
    if (!entered) {
      const warningKey =
        actionMode === IMAGE_ACTION_MODE.retouch
          ? 'canvas.tools.imageNotEditable'
          : 'canvas.tools.cannotInpaint'
      options.ui.warn(i18n.global.t(warningKey))
    } else {
      void options.modes.focusRetouch(nodeId)
    }
    return entered
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
      enterOutpaintMode(nodeId)
      return
    }
    if (actionMode === IMAGE_ACTION_MODE.inpaint || actionMode === IMAGE_ACTION_MODE.retouch) {
      enterRetouchMode(nodeId, actionMode)
      return
    }
    if (!isImageTaskAction(action)) {
      options.ui.warn(i18n.global.t('canvas.tools.actionUnavailable'))
      return
    }
    if (
      strength === undefined &&
      isImageStrengthAction(action) &&
      options.strength.consumeSuppressedClick(action)
    ) {
      return
    }
    options.modes.cancelOutpaint()
    options.modes.cancelInpaint()
    options.ui.clearPanel()
    options.ui.hideStrengthControl()
    void options.execution.runImageDirectAction(nodeId, action, {
      prompt,
      outputCount,
      ...(isImageStrengthAction(action)
        ? { strength: strength ?? options.strength.getValue(action) }
        : {}),
    })
  }

  function onFlowFusionPromptGenerate(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) {
    options.modes.cancelOutpaint()
    options.modes.cancelInpaint()
    options.ui.clearPanel()
    options.ui.hideStrengthControl()
    options.params.updateNodeParam(nodeId, { key: 'prompt', value: prompt })
    options.params.updateNodeParam(nodeId, { key: 'outputCount', value: outputCount })
    options.params.updateNodeParam(nodeId, { key: 'aspectRatio', value: aspectRatio })
    void options.execution.runFusionGeneration(nodeId, prompt, outputCount, aspectRatio)
  }

  return {
    onFlowImageContextMenu,
    onFlowFusionContextMenu,
    onFlowImageDirectAction,
    onFlowFusionPromptGenerate,
  }
}
