import type {
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  EditNodeItem,
  EnhanceNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PromptNodeItem,
  WearNodeItem,
  ViewpointNodeItem,
} from '../types/canvasNodes'

export type CanvasConnectionKind = 'image' | 'mask' | 'prompt'
export type CanvasConnectionDirection = 'input' | 'output'

export type CanvasConnectionContractNode =
  | PromptNodeItem
  | ImageNodeItem
  | MaskNodeItem
  | ModelNodeItem
  | ViewpointNodeItem
  | EditNodeItem
  | FusionNodeItem
  | EnhanceNodeItem
  | WearNodeItem

export interface CanvasNodePortContract {
  port: CanvasConnectionPort
  kind: CanvasConnectionKind
  direction: CanvasConnectionDirection
  required: boolean
  visible: boolean
}

export function editModeSupportsPrompt(mode: EditNodeItem['mode']) {
  return mode !== 'face-swap'
}

export function editModeSupportsSourceImage(mode: EditNodeItem['mode']) {
  return mode === 'face-swap'
}

export function editModeRequiresMask(mode: EditNodeItem['mode']) {
  void mode
  return false
}

export function getEditNodePortLabel(
  mode: EditNodeItem['mode'],
  port: Extract<CanvasConnectionPort, 'image' | 'imageB' | 'mask' | 'prompt'>,
) {
  switch (port) {
    case 'image':
      return mode === 'face-swap' ? 'Target' : 'Image'
    case 'imageB':
      return 'Face'
    case 'mask':
      return mode === 'face-swap' ? 'Face Mask' : 'Mask'
    case 'prompt':
      return 'Prompt'
  }
}

function input(
  port: CanvasConnectionPort,
  kind: CanvasConnectionKind,
  required = false,
  visible = true,
): CanvasNodePortContract {
  return { port, kind, direction: 'input', required, visible }
}

function output(
  port: CanvasConnectionPort,
  kind: CanvasConnectionKind,
  visible = true,
): CanvasNodePortContract {
  return { port, kind, direction: 'output', required: false, visible }
}

export function getCanvasNodePortContracts(
  nodeType: CanvasConnectionNodeType,
  node: CanvasConnectionContractNode | { id: number },
): CanvasNodePortContract[] {
  switch (nodeType) {
    case 'prompt':
      return [input('image', 'image'), output('image', 'image'), output('prompt', 'prompt')]
    case 'image':
      return [input('image', 'image'), output('image', 'image')]
    case 'mask':
      return [input('image', 'image', true), output('mask', 'mask')]
    case 'model':
      return [
        input('image', 'image', true),
        input('imageA', 'image', true),
        input('imageB', 'image'),
        input('prompt', 'prompt'),
        output('image', 'image'),
      ]
    case 'viewpoint':
      return [input('image', 'image', true), input('prompt', 'prompt'), output('image', 'image')]
    case 'edit': {
      const editNode = node as EditNodeItem
      return [
        input('image', 'image', true),
        input(
          'imageB',
          'image',
          editModeSupportsSourceImage(editNode.mode),
          editModeSupportsSourceImage(editNode.mode),
        ),
        input('mask', 'mask', editModeRequiresMask(editNode.mode), true),
        input(
          'prompt',
          'prompt',
          editModeSupportsPrompt(editNode.mode),
          editModeSupportsPrompt(editNode.mode),
        ),
        output('image', 'image'),
      ]
    }
    case 'fusion':
      return [
        input('image', 'image', true),
        input('prompt', 'prompt'),
        output('image', 'image'),
      ]
    case 'enhance': {
      return [
        input('image', 'image', true),
        input('mask', 'mask', false, true),
        input('prompt', 'prompt', false, false),
        output('image', 'image'),
      ]
    }
    case 'wear':
      return [
        input('imageA', 'image', true),
        input('imageB', 'image', true),
        input('mask', 'mask', false),
        output('image', 'image'),
      ]
    default:
      return []
  }
}

export function getCanvasNodeVisibleInputPorts(
  nodeType: CanvasConnectionNodeType,
  node: CanvasConnectionContractNode | { id: number },
): CanvasNodePortContract[] {
  return getCanvasNodePortContracts(nodeType, node).filter(
    (port) => port.direction === 'input' && port.visible,
  )
}

export function getCanvasNodeVisibleOutputPorts(
  nodeType: CanvasConnectionNodeType,
  node: CanvasConnectionContractNode | { id: number },
): CanvasNodePortContract[] {
  return getCanvasNodePortContracts(nodeType, node).filter(
    (port) => port.direction === 'output' && port.visible,
  )
}

export function getCanvasNodeRequiredInputPorts(
  nodeType: CanvasConnectionNodeType,
  node: CanvasConnectionContractNode | { id: number },
): CanvasNodePortContract[] {
  return getCanvasNodeVisibleInputPorts(nodeType, node).filter((port) => port.required)
}
