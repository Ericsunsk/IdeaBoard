import type { ImageActionBinding, ImageToolStageBinding } from './imageToolBindings'
import type { ImageToolSession } from './imageToolSession'

type CssStyleRecord = Record<string, string> | null

function areCssStyleRecordsEqual(left: CssStyleRecord, right: CssStyleRecord) {
  if (left === right) return true
  if (!left || !right) return false
  const leftKeys = Object.keys(left)
  const rightKeys = Object.keys(right)
  if (leftKeys.length !== rightKeys.length) return false
  return leftKeys.every((key) => left[key] === right[key])
}

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

function areStageBindingValuesEqual(
  key: keyof ImageToolStageBinding,
  leftValue: ImageToolStageBinding[keyof ImageToolStageBinding],
  rightValue: ImageToolStageBinding[keyof ImageToolStageBinding],
) {
  if (key === 'maskOverlayStyle' || key === 'rasterOverlayStyle') {
    return areCssStyleRecordsEqual(leftValue as CssStyleRecord, rightValue as CssStyleRecord)
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
  const { options } = session
  const stageBindingCache = new Map<string, ImageToolStageBinding>()
  const actionBindingCache = new Map<string, ImageActionBinding>()
  const maskCanvasRefCache = new Map<string, ImageToolStageBinding['setMaskCanvasRef']>()
  const rasterCanvasRefCache = new Map<string, ImageToolStageBinding['setRasterCanvasRef']>()

  function readMaskCanvasSize(style: Record<string, string> | null, key: 'width' | 'height') {
    const parsed = style?.[key] ? parseFloat(style[key]) : 0
    return Math.max(1, Math.round(Number.isFinite(parsed) ? parsed : 0))
  }

  function setMaskCanvasRefFor(nodeId: string) {
    const cached = maskCanvasRefCache.get(nodeId)
    if (cached) return cached
    const callback: ImageToolStageBinding['setMaskCanvasRef'] = (element) => {
      session.maskEditor.setMaskCanvasRef(nodeId, element)
    }
    maskCanvasRefCache.set(nodeId, callback)
    return callback
  }

  function setRasterCanvasRefFor(nodeId: string) {
    const cached = rasterCanvasRefCache.get(nodeId)
    if (cached) return cached
    const callback: ImageToolStageBinding['setRasterCanvasRef'] = (element) => {
      session.imageEditorRasterOverlay.setRasterCanvasRef(nodeId, element)
    }
    rasterCanvasRefCache.set(nodeId, callback)
    return callback
  }

  function stageBindingFor(nodeId: string): ImageToolStageBinding {
    const mode = session.imageEditorNodeId.value === nodeId ? session.imageEditorMode.value : null
    const maskOverlayStyle =
      session.maskEditor.maskEditorNodeId.value === nodeId
        ? session.maskEditor.maskEditorOverlayStyle.value
        : null
    const nextBinding: ImageToolStageBinding = {
      mode,
      renderedPreview:
        session.imageEditorNodeId.value === nodeId && session.imageEditorMode.value === 'adjust'
          ? renderedPreview.value
          : null,
      maskOverlayStyle,
      maskCanvasWidth: readMaskCanvasSize(maskOverlayStyle, 'width'),
      maskCanvasHeight: readMaskCanvasSize(maskOverlayStyle, 'height'),
      maskLayerOpacity: session.maskChromeState.maskEditorLayerOpacity.value,
      imageLayerVisible: true,
      maskBrushSize: session.maskEditor.brushSize.value,
      maskBrushShape: session.maskEditor.brushShape.value,
      setMaskCanvasRef: setMaskCanvasRefFor(nodeId),
      onMaskCanvasPointerDown: session.maskEditor.onCanvasPointerDown,
      rasterOverlayStyle:
        session.imageEditorNodeId.value === nodeId && session.imageEditorMode.value === 'adjust'
          ? session.imageEditorRasterOverlay.rasterOverlayStyle.value
          : null,
      rasterCanvasWidth: session.imageEditorRasterOverlay.rasterCanvasWidth.value,
      rasterCanvasHeight: session.imageEditorRasterOverlay.rasterCanvasHeight.value,
      rasterBrushSize: session.imageEditorRasterOverlay.rasterBrushSize.value,
      rasterTool: session.imageEditorRasterOverlay.rasterTool.value,
      rasterToolEnabled: session.imageEditorRasterOverlay.rasterToolEnabled.value,
      rasterOverlayVisible:
        session.imageEditorRasterOverlay.hasRasterEdits.value ||
        session.imageEditorRasterOverlay.rasterTool.value != null,
      setRasterCanvasRef: setRasterCanvasRefFor(nodeId),
      onRasterCanvasPointerDown: session.imageEditorRasterOverlay.onRasterCanvasPointerDown,
    }
    return reuseCachedBinding(stageBindingCache, nodeId, nextBinding, (left, right) =>
      areBindingObjectsEqual(left, right, areStageBindingValuesEqual),
    )
  }

  function actionBindingFor(nodeId: string): ImageActionBinding {
    const hasCommittedMask = !!session.maskEditor.getCommittedMaskData(nodeId)
    const actionMenuVisible = options.isImageActionPanelVisible(nodeId)
    const nextBinding: ImageActionBinding = {
      actionMenuVisible,
      promptComposerVisible: options.isImagePromptComposerVisible(nodeId),
      promptPlaceholder: hasCommittedMask ? '描述如何重绘选区' : '输入提示词改造这张图',
      directGenerating: options.isDirectActionRunning(nodeId),
      showStrengthControl: !!options.activeStrengthAction.value && actionMenuVisible,
      activeStrengthAction: options.activeStrengthAction.value,
      getStrengthValue: options.getActionStrength,
      setStrengthRangeRef: options.setStrengthRangeRef,
      setStrengthTriggerRef: options.setStrengthTriggerRef,
      onStrengthRangePointerDown: options.onStrengthRangePointerDown,
      onStrengthInput: options.onStrengthInput,
      onStrengthChange: options.onStrengthChange,
      onStrengthPointerUp: options.onStrengthPointerUp,
      onStrengthClick: options.onStrengthClick,
    }
    return reuseCachedBinding(actionBindingCache, nodeId, nextBinding, areBindingObjectsEqual)
  }

  return {
    stageBindingFor,
    actionBindingFor,
  }
}
