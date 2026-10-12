import type { ImageActionBinding, ImageToolStageBinding } from './bindings'
import type { ImageToolSession } from './imageToolSession'
import type { ZeroCopyStageHandle } from '../../retouch'
import { i18n } from '../../../../../core/i18n'

// Stage identity covers the 4 reactive fields; session is a stable closure
// reference and intentionally excluded from the equality check.
function areStageBindingsEqual(left: ImageToolStageBinding, right: ImageToolStageBinding): boolean {
  return (
    left.nodeId === right.nodeId &&
    left.active === right.active &&
    left.mode === right.mode &&
    Object.is(left.retouchStage, right.retouchStage)
  )
}

// Action identity covers the 5 top-level fields; strengthControl is a fresh
// object per call, so its fields are compared inline instead of by reference.
function areActionBindingsEqual(left: ImageActionBinding, right: ImageActionBinding): boolean {
  const leftControl = left.strengthControl
  const rightControl = right.strengthControl
  return (
    left.actionMenuVisible === right.actionMenuVisible &&
    left.promptComposerVisible === right.promptComposerVisible &&
    left.promptPlaceholder === right.promptPlaceholder &&
    left.directGenerating === right.directGenerating &&
    leftControl.visible === rightControl.visible &&
    Object.is(leftControl.activeAction, rightControl.activeAction) &&
    leftControl.getValue === rightControl.getValue &&
    leftControl.setRangeRef === rightControl.setRangeRef &&
    leftControl.setTriggerRef === rightControl.setTriggerRef &&
    leftControl.onRangePointerDown === rightControl.onRangePointerDown &&
    leftControl.onInput === rightControl.onInput &&
    leftControl.onChange === rightControl.onChange &&
    leftControl.onPointerUp === rightControl.onPointerUp &&
    leftControl.onClick === rightControl.onClick
  )
}

function reuseCachedBinding<T>(
  cache: Map<string, T>,
  nodeId: string,
  nextBinding: T,
  isEqual: (left: T, right: T) => boolean,
) {
  const previousBinding = cache.get(nodeId)
  if (previousBinding && isEqual(previousBinding, nextBinding)) return previousBinding
  cache.set(nodeId, nextBinding)
  return nextBinding
}

export function createImageToolNodeBindings(
  session: ImageToolSession,
  retouchStage?: Pick<
    ZeroCopyStageHandle,
    'canvasRef' | 'setCanvasRef' | 'isGpuActive' | 'stageStatus'
  > | null,
) {
  const { execution, ui } = session.capabilities
  const stageBindingCache = new Map<string, ImageToolStageBinding>()
  const actionBindingCache = new Map<string, ImageActionBinding>()
  function stageBindingFor(nodeId: string): ImageToolStageBinding {
    const mode = session.retouchNodeId.value === nodeId ? session.retouchMode.value : null
    const nextBinding: ImageToolStageBinding = {
      nodeId,
      active: mode != null,
      mode,
      retouchStage:
        session.retouchNodeId.value === nodeId && session.retouchMode.value === 'adjust'
          ? (retouchStage ?? null)
          : null,
      session,
    }
    return reuseCachedBinding(stageBindingCache, nodeId, nextBinding, areStageBindingsEqual)
  }

  function actionBindingFor(nodeId: string): ImageActionBinding {
    const hasCommittedMask = !!session.inpaint.getCommittedMaskData(nodeId)
    const actionMenuVisible = ui.isImageActionPanelVisible(nodeId)
    const activeStrengthAction = ui.strengthControl.activeAction.value
    const directGenerating =
      execution.isNodeExecutionActive(nodeId) || session.applyScope.pendingNodeId.value === nodeId
    const nextBinding: ImageActionBinding = {
      actionMenuVisible,
      promptComposerVisible: ui.isImagePromptComposerVisible(nodeId),
      promptPlaceholder: hasCommittedMask
        ? i18n.global.t('canvas.tools.maskPromptPlaceholder')
        : i18n.global.t('canvas.tools.optionalPromptPlaceholder'),
      directGenerating,
      strengthControl: {
        visible: !!activeStrengthAction && actionMenuVisible,
        activeAction: activeStrengthAction,
        getValue: ui.strengthControl.getValue,
        setRangeRef: ui.strengthControl.setRangeRef,
        setTriggerRef: ui.strengthControl.setTriggerRef,
        onRangePointerDown: ui.strengthControl.onRangePointerDown,
        onInput: ui.strengthControl.onInput,
        onChange: ui.strengthControl.onChange,
        onPointerUp: ui.strengthControl.onPointerUp,
        onClick: ui.strengthControl.onClick,
      },
    }
    return reuseCachedBinding(actionBindingCache, nodeId, nextBinding, areActionBindingsEqual)
  }

  return {
    stageBindingFor,
    actionBindingFor,
  }
}
