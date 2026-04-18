import type { Ref } from 'vue'
import {
  getCanvasNodeRequiredInputPorts,
  getCanvasNodeVisibleInputPorts,
  getCanvasNodeVisibleOutputPorts,
  type CanvasConnectionContractNode,
} from '../config/canvasNodeConnectionContract'
import type {
  CanvasConnectionPort,
  CanvasConnectionNodeType,
  CanvasNodeItem,
  ImageNodeItem,
  ImageNodeOutputItem,
  MaskNodeItem,
  PromptNodeItem,
} from '../types/canvasNodes'

type ConnectedImageInput = { url: string; ratio: number }
type PlacedImageInput = ConnectedImageInput & { placementX: number; placementY: number }
type PlacedMaskInput = { dataUrl: string; placementX: number; placementY: number }

type ConnectionStatusMap = Partial<Record<CanvasConnectionPort, boolean>>

export interface NodeConnectionStatus {
  hasAnyConnection: boolean
  incoming: ConnectionStatusMap
  pendingIncoming: ConnectionStatusMap
  outgoing: ConnectionStatusMap
}

interface UseCanvasNodeConnectionsOptions {
  imageNodes: Ref<ImageNodeItem[]>
  promptNodes: Ref<PromptNodeItem[]>
  maskNodes: Ref<MaskNodeItem[]>
  getIncomingConnections: (nodeType: CanvasConnectionNodeType, nodeId: number) => unknown[]
  hasIncomingPortConnection: (
    nodeType: CanvasConnectionNodeType,
    nodeId: number,
    port: CanvasConnectionPort,
  ) => boolean
  hasOutgoingPortConnection: (
    nodeType: CanvasConnectionNodeType,
    nodeId: number,
    port: CanvasConnectionPort,
  ) => boolean
  findIncomingConnection: (
    nodeType: CanvasConnectionNodeType,
    nodeId: number,
    port: CanvasConnectionPort,
  ) => {
    fromNodeType: CanvasConnectionNodeType
    fromNodeId: number
  } | null
  getOutgoingConnections: (
    nodeType: CanvasConnectionNodeType,
    nodeId: number,
    port: CanvasConnectionPort,
  ) => {
    toNodeType: CanvasConnectionNodeType
    toNodeId: number
  }[]
  getImageNodePreferredImage: (node: ImageNodeItem | null) => ConnectedImageInput | null
  lookupNodeByType: (nodeType: CanvasConnectionNodeType, nodeId: number) => CanvasNodeItem | null
  refineMaskDataUrl: (maskDataUrl: string, refineAmount: number, invertMask: boolean) => Promise<string>
}

export function useCanvasNodeConnections(options: UseCanvasNodeConnectionsOptions) {
  function getNodeConnectionStatus(
    nodeType: CanvasConnectionNodeType,
    node: CanvasConnectionContractNode | { id: number },
  ): NodeConnectionStatus {
    const incoming: ConnectionStatusMap = {}
    const pendingIncoming: ConnectionStatusMap = {}
    const outgoing: ConnectionStatusMap = {}

    getCanvasNodeVisibleInputPorts(nodeType, node).forEach((port) => {
      const connected = options.hasIncomingPortConnection(nodeType, node.id, port.port)
      incoming[port.port] = connected
      pendingIncoming[port.port] = connected ? isPendingInputConnection(nodeType, node.id, port.port) : false
    })

    getCanvasNodeVisibleOutputPorts(nodeType, node).forEach((port) => {
      outgoing[port.port] = options.hasOutgoingPortConnection(nodeType, node.id, port.port)
    })

    const hasAnyConnection =
      options.getIncomingConnections(nodeType, node.id).length > 0 ||
      Object.values(outgoing).some(Boolean)

    return {
      hasAnyConnection,
      incoming,
      pendingIncoming,
      outgoing,
    }
  }

  function getConnectedImageInput(
    toNodeType: CanvasConnectionNodeType,
    toNodeId: number,
    toPort: 'image' | 'imageA' | 'imageB' | 'reference',
  ): ConnectedImageInput | null {
    const connection = options.findIncomingConnection(toNodeType, toNodeId, toPort)
    if (!connection) return null
    return resolveConnectedImageSource(connection.fromNodeType, connection.fromNodeId)?.image ?? null
  }

  function getConnectedPromptInput(
    toNodeType: CanvasConnectionNodeType,
    toNodeId: number,
    toPort: 'prompt' = 'prompt',
  ): string {
    const connection = options.findIncomingConnection(toNodeType, toNodeId, toPort)
    if (!connection || connection.fromNodeType !== 'prompt') return ''
    const sourceNode = options.promptNodes.value.find(item => item.id === connection.fromNodeId) ?? null
    return sourceNode?.text.trim() || ''
  }

  function getConnectedOutputImageTarget(
    fromNodeType: CanvasConnectionNodeType,
    fromNodeId: number,
    fromPort: 'image' = 'image',
  ): ImageNodeItem | null {
    const connection = options.getOutgoingConnections(fromNodeType, fromNodeId, fromPort)
      .find(item => item.toNodeType === 'image') ?? null
    if (!connection || connection.toNodeType !== 'image') return null
    return options.imageNodes.value.find(item => item.id === connection.toNodeId) ?? null
  }

  function resolveConnectedPlacedImageInput(
    toNodeType: CanvasConnectionNodeType,
    toNodeId: number,
    toPort: 'image' | 'imageA' | 'imageB' | 'reference',
  ): PlacedImageInput | null {
    const connection = options.findIncomingConnection(toNodeType, toNodeId, toPort)
    if (!connection) return null
    const resolvedSource = resolveConnectedImageSource(connection.fromNodeType, connection.fromNodeId)
    if (!resolvedSource?.image.url) return null
    return {
      ...resolvedSource.image,
      placementX: resolvedSource.placementX,
      placementY: resolvedSource.placementY,
    }
  }

  async function resolveConnectedPlacedMaskInput(
    toNodeType: CanvasConnectionNodeType,
    toNodeId: number,
    toPort: 'mask',
  ): Promise<PlacedMaskInput | null> {
    const connection = options.findIncomingConnection(toNodeType, toNodeId, toPort)
    if (!connection || connection.fromNodeType !== 'mask') return null
    const sourceNode = options.maskNodes.value.find(item => item.id === connection.fromNodeId) ?? null
    if (!sourceNode?.maskDataUrl) return null
    const dataUrl = await options.refineMaskDataUrl(
      sourceNode.maskDataUrl,
      sourceNode.refineAmount ?? 0,
      !!sourceNode.invertMask,
    )
    if (!dataUrl) return null
    return {
      dataUrl,
      placementX: sourceNode.x + 340,
      placementY: sourceNode.y + 18,
    }
  }

  function resolveConnectedImageSource(
    fromNodeType: CanvasConnectionNodeType,
    fromNodeId: number,
  ): { image: ConnectedImageInput; placementX: number; placementY: number } | null {
    if (fromNodeType === 'image') {
      const sourceNode = options.imageNodes.value.find(item => item.id === fromNodeId) ?? null
      const preferredImage = options.getImageNodePreferredImage(sourceNode)
      if (!preferredImage?.url || !sourceNode) return null
      return {
        image: preferredImage,
        placementX: sourceNode.x - 340,
        placementY: sourceNode.y + 18,
      }
    }

    const sourceNode = options.lookupNodeByType(fromNodeType, fromNodeId)
    if (!hasNodeOutputImages(sourceNode)) return null
    if (!hasFreshNodeOutput(sourceNode)) return null
    const preferredImage = getLatestNodeOutputImage(sourceNode.outputImages)
    if (!preferredImage?.url) return null
    return {
      image: preferredImage,
      placementX: sourceNode.x - 340,
      placementY: sourceNode.y + 18,
    }
  }

  function isPendingInputConnection(
    toNodeType: CanvasConnectionNodeType,
    toNodeId: number,
    toPort: CanvasConnectionPort,
  ): boolean {
    const connection = options.findIncomingConnection(toNodeType, toNodeId, toPort)
    if (!connection) return false

    if (toPort === 'image' || toPort === 'imageA' || toPort === 'imageB' || toPort === 'reference') {
      return !resolveConnectedImageSource(connection.fromNodeType, connection.fromNodeId)
    }

    if (toPort === 'mask') {
      const sourceNode = connection.fromNodeType === 'mask'
        ? options.maskNodes.value.find(item => item.id === connection.fromNodeId) ?? null
        : null
      return !sourceNode?.maskDataUrl
    }

    return false
  }

  function getMissingRequiredNodeInputs(
    nodeType: CanvasConnectionNodeType,
    node: CanvasConnectionContractNode | { id: number },
    resolvedInputs: Partial<Record<CanvasConnectionPort, unknown>>,
  ): CanvasConnectionPort[] {
    return getCanvasNodeRequiredInputPorts(nodeType, node)
      .filter(port => !hasResolvedInputValue(resolvedInputs[port.port]))
      .map(port => port.port)
  }

  return {
    getNodeConnectionStatus,
    getConnectedImageInput,
    getConnectedPromptInput,
    getConnectedOutputImageTarget,
    resolveConnectedPlacedImageInput,
    resolveConnectedPlacedMaskInput,
    getMissingRequiredNodeInputs,
  }
}

function hasResolvedInputValue(value: unknown): boolean {
  if (typeof value === 'string') return value.trim().length > 0
  if (Array.isArray(value)) return value.length > 0
  return value != null && value !== false
}

function hasNodeOutputImages(node: CanvasNodeItem | null): node is CanvasNodeItem & { outputImages: ImageNodeOutputItem[] } {
  return !!node && 'outputImages' in node && Array.isArray(node.outputImages)
}

function hasFreshNodeOutput(node: CanvasNodeItem & { outputImages: ImageNodeOutputItem[] }): boolean {
  if (!('outputStatus' in node)) return node.outputImages.length > 0
  if (node.outputStatus === 'stale') return false
  return node.outputImages.length > 0
}

function getLatestNodeOutputImage(outputImages: ImageNodeOutputItem[]): ConnectedImageInput | null {
  if (!outputImages.length) return null
  const latestOutput = outputImages[outputImages.length - 1]
  return {
    url: latestOutput.url,
    ratio: latestOutput.ratio > 0 ? latestOutput.ratio : 1,
  }
}
