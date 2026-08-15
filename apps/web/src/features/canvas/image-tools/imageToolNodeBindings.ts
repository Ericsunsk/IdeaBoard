import type {
  ImageActionBinding,
  ImageActionStrengthControl,
  ImageToolStageBinding,
} from './imageToolBindings'
import type { ImageToolSession } from './imageToolSession'

function areBindingObjectsEqual<T extends object>(
  left: T,
  right: T,
  isCustomValueEqual?: (key: keyof T, leftValue: T[keyof T], rightValue: T[keyof T]) => boolean,
) {
  const leftKeys = Object.keys(left) as Array<keyof T>
  const rightKeys = Object.keys(right) as Array<keyof T>
  if (leftKeys.length !== rightKeys.length) return false
  return leftKeys.every((key) => {
    if (!Object.prototype.hasOwnProperty.call(right, key)) return false
    const leftValue = left[key]
    const rightValue = right[key]
    return isCustomValueEqual?.(key, leftValue, rightValue) ?? Object.is(leftValue, rightValue)
  })
}

function areStrengthControlsEqual(
  left: ImageActionStrengthControl,
  right: ImageActionStrengthControl,
) {
  return areBindingObjectsEqual(left, right)
}

function areActionBindingValuesEqual(
  key: keyof ImageActionBinding,
  leftValue: ImageActionBinding[keyof ImageActionBinding],
  rightValue: ImageActionBinding[keyof ImageActionBinding],
) {
  if (key === 'strengthControl') {
    return areStrengthControlsEqual(
      leftValue as ImageActionStrengthControl,
      rightValue as ImageActionStrengthControl,
    )
  }
  return Object.is(leftValue, rightValue)
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
  renderedPreview: { readonly value: ImageToolStageBinding['renderedPreview'] },
) {
  const { execution, ui } = session.capabilities
  const stageBindingCache = new Map<string, ImageToolStageBinding>()
  const actionBindingCache = new Map<string, ImageActionBinding>()
  function stageBindingFor(nodeId: string): ImageToolStageBinding {
    const mode = session.imageEditorNodeId.value === nodeId ? session.imageEditorMode.value : null
    const nextBinding: ImageToolStageBinding = {
      nodeId,
      active: mode != null,
      mode,
      renderedPreview:
        session.imageEditorNodeId.value === nodeId && session.imageEditorMode.value === 'adjust'
          ? renderedPreview.value
          : null,
      session,
    }
    return reuseCachedBinding(stageBindingCache, nodeId, nextBinding, (left, right) =>
      areBindingObjectsEqual(left, right),
    )
  }

  function actionBindingFor(nodeId: string): ImageActionBinding {
    const hasCommittedMask = !!session.maskEditor.getCommittedMaskData(nodeId)
    const actionMenuVisible = ui.isImageActionPanelVisible(nodeId)
    const activeStrengthAction = ui.strengthControl.activeAction.value
    const directGenerating = execution.isNodeExecutionActive(nodeId)
    const nextBinding: ImageActionBinding = {
      actionMenuVisible,
      promptComposerVisible: ui.isImagePromptComposerVisible(nodeId),
      promptPlaceholder: hasCommittedMask ? '描述如何重绘选区' : '输入辅助要求（可选）',
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
    return reuseCachedBinding(actionBindingCache, nodeId, nextBinding, (left, right) =>
      areBindingObjectsEqual(left, right, areActionBindingValuesEqual),
    )
  }

  return {
    stageBindingFor,
    actionBindingFor,
  }
}
