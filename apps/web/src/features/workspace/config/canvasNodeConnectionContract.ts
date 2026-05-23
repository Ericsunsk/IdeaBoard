import type {
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  EditNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  ModelNodeItem,
  PromptNodeItem,
  ViewpointNodeItem,
} from '../types/canvasNodes'
import { EDIT_NODE_MODE } from '../types/editNodeModes'

export type CanvasConnectionKind = 'image' | 'prompt'
export type CanvasConnectionDirection = 'input' | 'output'

export type CanvasConnectionContractNode =
  | PromptNodeItem
  | ImageNodeItem
  | ModelNodeItem
  | ViewpointNodeItem
  | EditNodeItem
  | FusionNodeItem

export interface CanvasNodePortContract {
  port: CanvasConnectionPort
  kind: CanvasConnectionKind
  direction: CanvasConnectionDirection
  required: boolean
  visible: boolean
}

export function editModeSupportsPrompt(mode: EditNodeItem['mode']) {
  return mode !== EDIT_NODE_MODE.faceSwap
}

export function editModeSupportsSourceImage(mode: EditNodeItem['mode']) {
  return mode === EDIT_NODE_MODE.faceSwap
}

export function getEditNodePortLabel(
  mode: EditNodeItem['mode'],
  port: Extract<CanvasConnectionPort, 'image' | 'imageB' | 'prompt'>,
) {
  switch (port) {
    case 'image':
      return mode === EDIT_NODE_MODE.faceSwap ? 'Target' : 'Image'
    case 'imageB':
      return 'Face'
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
