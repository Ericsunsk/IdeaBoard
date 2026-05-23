import type {
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  FusionNodeItem,
  ImageNodeItem,
  ModelNodeItem,
  PromptNodeItem,
  ViewpointNodeItem,
} from '../types/canvasNodes'

export type CanvasConnectionKind = 'image' | 'prompt'
export type CanvasConnectionDirection = 'input' | 'output'

export type CanvasConnectionContractNode =
  | PromptNodeItem
  | ImageNodeItem
  | ModelNodeItem
  | ViewpointNodeItem
  | FusionNodeItem

export interface CanvasNodePortContract {
  port: CanvasConnectionPort
  kind: CanvasConnectionKind
  direction: CanvasConnectionDirection
  required: boolean
  visible: boolean
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
): CanvasNodePortContract[] {
  return getCanvasNodePortContracts(nodeType).filter(
    (port) => port.direction === 'input' && port.visible,
  )
}

export function getCanvasNodeVisibleOutputPorts(
  nodeType: CanvasConnectionNodeType,
): CanvasNodePortContract[] {
  return getCanvasNodePortContracts(nodeType).filter(
    (port) => port.direction === 'output' && port.visible,
  )
}

export function getCanvasNodeRequiredInputPorts(
  nodeType: CanvasConnectionNodeType,
): CanvasNodePortContract[] {
  return getCanvasNodeVisibleInputPorts(nodeType).filter((port) => port.required)
}
