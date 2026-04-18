import type {
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  ColorNodeItem,
  EditNodeItem,
  EnhanceNodeItem,
  FaceSwapNodeItem,
  FusionNodeItem,
  GenerateNodeItem,
  ImageNodeItem,
  MannequinNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PaletteNodeItem,
  PromptNodeItem,
  TryOnNodeItem,
  ViewpointNodeItem,
} from '../types/canvasNodes'

export type CanvasConnectionKind = 'image' | 'mask' | 'prompt'
export type CanvasConnectionDirection = 'input' | 'output'

export type CanvasConnectionContractNode =
  | PromptNodeItem
  | ImageNodeItem
  | MaskNodeItem
  | ColorNodeItem
  | ModelNodeItem
  | MannequinNodeItem
  | ViewpointNodeItem
  | EditNodeItem
  | FusionNodeItem
  | EnhanceNodeItem
  | TryOnNodeItem
  | FaceSwapNodeItem
  | PaletteNodeItem
  | GenerateNodeItem

export interface CanvasNodePortContract {
  port: CanvasConnectionPort
  kind: CanvasConnectionKind
  direction: CanvasConnectionDirection
  required: boolean
  visible: boolean
}

export function enhanceModeSupportsPrompt(mode: EnhanceNodeItem['mode']) {
  return mode === 'deblur' || mode === 'canvas-extend'
}

export function paletteModeUsesReference(mode: PaletteNodeItem['mode']) {
  return mode === 'reference'
}

export function generateModeRequiresImage(mode: GenerateNodeItem['mode']) {
  return mode !== 'pattern-gen'
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
      return [output('prompt', 'prompt')]
    case 'image':
      return [input('image', 'image'), output('image', 'image')]
    case 'mask':
      return [input('image', 'image', true), output('mask', 'mask')]
    case 'color':
      return [
        input('image', 'image', true),
        input('mask', 'mask', true),
        input('prompt', 'prompt'),
        output('image', 'image'),
      ]
    case 'model':
    case 'mannequin':
    case 'viewpoint':
      return [input('image', 'image', true), input('prompt', 'prompt'), output('image', 'image')]
    case 'edit':
      return [
        input('image', 'image', true),
        input('mask', 'mask', true),
        input('prompt', 'prompt'),
        output('image', 'image'),
      ]
    case 'fusion':
      return [
        input('imageA', 'image', true),
        input('imageB', 'image', true),
        input('prompt', 'prompt'),
        output('image', 'image'),
      ]
    case 'enhance': {
      const enhanceNode = node as EnhanceNodeItem
      const promptVisible = enhanceModeSupportsPrompt(enhanceNode.mode)
      return [
        input('image', 'image', true),
        input('prompt', 'prompt', false, promptVisible),
        output('image', 'image'),
      ]
    }
    case 'tryon':
      return [input('imageA', 'image', true), input('imageB', 'image', true), output('image', 'image')]
    case 'faceswap':
      return [
        input('imageA', 'image', true),
        input('imageB', 'image', true),
        input('mask', 'mask'),
        output('image', 'image'),
      ]
    case 'palette': {
      const paletteNode = node as PaletteNodeItem
      const referenceVisible = paletteModeUsesReference(paletteNode.mode)
      return [
        input('image', 'image', true),
        input('reference', 'image', referenceVisible, referenceVisible),
        output('image', 'image'),
      ]
    }
    case 'generate': {
      const generateNode = node as GenerateNodeItem
      return [
        input('image', 'image', generateModeRequiresImage(generateNode.mode)),
        input('prompt', 'prompt', true),
        output('image', 'image'),
      ]
    }
    default:
      return []
  }
}

export function getCanvasNodeVisibleInputPorts(
  nodeType: CanvasConnectionNodeType,
  node: CanvasConnectionContractNode | { id: number },
): CanvasNodePortContract[] {
  return getCanvasNodePortContracts(nodeType, node).filter(port => port.direction === 'input' && port.visible)
}

export function getCanvasNodeVisibleOutputPorts(
  nodeType: CanvasConnectionNodeType,
  node: CanvasConnectionContractNode | { id: number },
): CanvasNodePortContract[] {
  return getCanvasNodePortContracts(nodeType, node).filter(port => port.direction === 'output' && port.visible)
}

export function getCanvasNodeRequiredInputPorts(
  nodeType: CanvasConnectionNodeType,
  node: CanvasConnectionContractNode | { id: number },
): CanvasNodePortContract[] {
  return getCanvasNodeVisibleInputPorts(nodeType, node).filter(port => port.required)
}
