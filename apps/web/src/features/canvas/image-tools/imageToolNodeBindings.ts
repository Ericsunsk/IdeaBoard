import type { ImageActionBinding, ImageToolStageBinding } from './imageToolBindings'
import type { ImageToolSession } from './imageToolSession'

export function createImageToolNodeBindings(
  session: ImageToolSession,
  renderedPreview: { readonly value: ImageToolStageBinding['renderedPreview'] },
) {
  const { options } = session

  function readMaskCanvasSize(style: Record<string, string> | null, key: 'width' | 'height') {
    const parsed = style?.[key] ? parseFloat(style[key]) : 0
    return Math.max(1, Math.round(Number.isFinite(parsed) ? parsed : 0))
  }

  function stageBindingFor(nodeId: string): ImageToolStageBinding {
    const mode = session.imageEditorNodeId.value === nodeId ? session.imageEditorMode.value : null
    const maskOverlayStyle =
      session.maskEditor.maskEditorNodeId.value === nodeId
        ? session.maskEditor.maskEditorOverlayStyle.value
        : null
    return {
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
      setMaskCanvasRef: (element) => session.maskEditor.setMaskCanvasRef(nodeId, element),
      onMaskCanvasPointerDown: session.maskEditor.onCanvasPointerDown,
    }
  }

  function actionBindingFor(nodeId: string): ImageActionBinding {
    const hasCommittedMask = !!session.maskEditor.getCommittedMaskData(nodeId)
    return {
      actionMenuVisible: options.isImageActionPanelVisible(nodeId),
      promptComposerVisible: options.isImagePromptComposerVisible(nodeId),
      promptPlaceholder: hasCommittedMask ? '描述如何重绘选区' : '输入提示词改造这张图',
      directGenerating: options.isDirectActionRunning(nodeId),
      showStrengthControl:
        !!options.activeStrengthAction.value && options.isImageActionPanelVisible(nodeId),
      strengthPct: options.strengthPct.value,
      setStrengthTrackRef: options.getStrengthTrackRefSetter(),
    }
  }

  return {
    stageBindingFor,
    actionBindingFor,
  }
}
