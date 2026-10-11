import {
  DEFAULT_FUSION_NODE_OUTPUT_COUNT,
  DEFAULT_IMAGE_NODE_OUTPUT_COUNT,
  DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
  type BoardSnapshot,
  type CanvasConnectionNodeKind,
  type CanvasConnectionPort,
  type CanvasExecutionOutputNodeKind,
  type CanvasNodeItem,
  type CanvasNodeItemByKind,
  type CanvasPoint,
  type FusionNodeItem,
  type ImageNodeItem,
  type PromptNodeItem,
} from './nodes'
import { DEFAULT_FUSION_BLEND_POINT } from './fusionBlend'
import { DEFAULT_PROMPT_ASPECT_RATIO } from './promptAspectRatio'

export type CanvasSnapshotNodeCollectionKey = keyof Pick<
  BoardSnapshot,
  'imageNodes' | 'promptNodes' | 'fusionNodes'
>

export type CanvasCreatableNodeKind = CanvasConnectionNodeKind
export type CanvasConnectionKind = 'image' | 'prompt'
export type CanvasConnectionDirection = 'input' | 'output'
export type CanvasConnectionContractNode = PromptNodeItem | ImageNodeItem | FusionNodeItem

export interface CanvasNodePortContract {
  port: CanvasConnectionPort
  kind: CanvasConnectionKind
  direction: CanvasConnectionDirection
  required: boolean
  visible: boolean
  maxConnections?: number
}

type CanvasNodeRegistryItem = {
  kind: CanvasConnectionNodeKind
  snapshotType: CanvasNodeItem['type']
  snapshotCollection: CanvasSnapshotNodeCollectionKey
  defaultWidth: number
  ports: readonly CanvasNodePortContract[]
  primaryOutputPort: CanvasConnectionPort
  imageInputSource: boolean
  executionOutput: boolean
  createSnapshot: (point: CanvasPoint, id: number) => CanvasNodeItem
}

const CANVAS_NODE_DEFAULTS = {
  image: { width: 280 },
  prompt: { width: 280 },
  fusion: { width: 286 },
} as const

const CANVAS_MEDIA_INPUT_IMAGE_LIMIT = 4

function input(
  port: CanvasConnectionPort,
  kind: CanvasConnectionKind,
  options: { required?: boolean; visible?: boolean; maxConnections?: number } = {},
): CanvasNodePortContract {
  return {
    port,
    kind,
    direction: 'input',
    required: options.required ?? false,
    visible: options.visible ?? true,
    ...(options.maxConnections ? { maxConnections: options.maxConnections } : {}),
  }
}

function output(
  port: CanvasConnectionPort,
  kind: CanvasConnectionKind,
  visible = true,
): CanvasNodePortContract {
  return { port, kind, direction: 'output', required: false, visible }
}

function createPromptSnapshot(point: CanvasPoint, id: number): PromptNodeItem {
  return {
    id,
    type: 'prompt',
    x: point.x,
    y: point.y,
    width: CANVAS_NODE_DEFAULTS.prompt.width,
    text: '',
    outputCount: DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
    outputResolution: '1k',
    aspectRatio: DEFAULT_PROMPT_ASPECT_RATIO,
    inputImages: [],
  }
}

function createImageSnapshot(point: CanvasPoint, id: number): ImageNodeItem {
  return {
    id,
    type: 'image-node',
    x: point.x,
    y: point.y,
    width: CANVAS_NODE_DEFAULTS.image.width,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    outputCount: DEFAULT_IMAGE_NODE_OUTPUT_COUNT,
    outputResolution: '1k',
    outputStatus: 'idle',
    outputImages: [],
  }
}

function createFusionSnapshot(point: CanvasPoint, id: number): FusionNodeItem {
  return {
    id,
    type: 'fusion',
    x: point.x,
    y: point.y,
    width: CANVAS_NODE_DEFAULTS.fusion.width,
    prompt: '',
    outputCount: DEFAULT_FUSION_NODE_OUTPUT_COUNT,
    outputResolution: '1k',
    aspectRatio: DEFAULT_PROMPT_ASPECT_RATIO,
    blendPoint: { ...DEFAULT_FUSION_BLEND_POINT },
    inputImages: [],
    outputStatus: 'idle',
    outputImages: [],
  }
}

const CANVAS_NODE_REGISTRY = {
  image: {
    kind: 'image',
    snapshotType: 'image-node',
    snapshotCollection: 'imageNodes',
    defaultWidth: CANVAS_NODE_DEFAULTS.image.width,
    ports: [input('image', 'image'), output('image', 'image')],
    primaryOutputPort: 'image',
    imageInputSource: true,
    executionOutput: false,
    createSnapshot: createImageSnapshot,
  },
  prompt: {
    kind: 'prompt',
    snapshotType: 'prompt',
    snapshotCollection: 'promptNodes',
    defaultWidth: CANVAS_NODE_DEFAULTS.prompt.width,
    ports: [
      input('image', 'image', { maxConnections: CANVAS_MEDIA_INPUT_IMAGE_LIMIT }),
      output('image', 'image'),
      output('prompt', 'prompt'),
    ],
    primaryOutputPort: 'prompt',
    imageInputSource: false,
    executionOutput: false,
    createSnapshot: createPromptSnapshot,
  },
  fusion: {
    kind: 'fusion',
    snapshotType: 'fusion',
    snapshotCollection: 'fusionNodes',
    defaultWidth: CANVAS_NODE_DEFAULTS.fusion.width,
    ports: [
      input('image', 'image', { required: true, maxConnections: CANVAS_MEDIA_INPUT_IMAGE_LIMIT }),
      input('prompt', 'prompt'),
      output('image', 'image'),
    ],
    primaryOutputPort: 'image',
    imageInputSource: true,
    executionOutput: true,
    createSnapshot: createFusionSnapshot,
  },
} as const satisfies Record<CanvasConnectionNodeKind, CanvasNodeRegistryItem>

export const CANVAS_NODE_REGISTRY_LIST = Object.values(CANVAS_NODE_REGISTRY)

const CANVAS_CONNECTION_NODE_TYPES = CANVAS_NODE_REGISTRY_LIST.map(
  (item) => item.kind,
) as readonly CanvasConnectionNodeKind[]

const CANVAS_EXECUTION_OUTPUT_NODE_TYPES = CANVAS_NODE_REGISTRY_LIST.filter(
  (item) => item.executionOutput,
).map((item) => item.kind) as readonly CanvasExecutionOutputNodeKind[]

const CANVAS_IMAGE_INPUT_SOURCE_NODE_TYPES = CANVAS_NODE_REGISTRY_LIST.filter(
  (item) => item.imageInputSource,
).map((item) => item.kind) as readonly CanvasConnectionNodeKind[]

const CANVAS_CONNECTION_NODE_TYPE_SET = new Set<CanvasConnectionNodeKind>(
  CANVAS_CONNECTION_NODE_TYPES,
)
const CANVAS_EXECUTION_OUTPUT_NODE_TYPE_SET = new Set<CanvasExecutionOutputNodeKind>(
  CANVAS_EXECUTION_OUTPUT_NODE_TYPES,
)
const CANVAS_IMAGE_INPUT_SOURCE_NODE_TYPE_SET = new Set<CanvasConnectionNodeKind>(
  CANVAS_IMAGE_INPUT_SOURCE_NODE_TYPES,
)

export function isCanvasConnectionNodeType(value: unknown): value is CanvasConnectionNodeKind {
  return (
    typeof value === 'string' &&
    CANVAS_CONNECTION_NODE_TYPE_SET.has(value as CanvasConnectionNodeKind)
  )
}

export function isCanvasExecutionOutputNodeType(
  value: unknown,
): value is CanvasExecutionOutputNodeKind {
  return (
    typeof value === 'string' &&
    CANVAS_EXECUTION_OUTPUT_NODE_TYPE_SET.has(value as CanvasExecutionOutputNodeKind)
  )
}

export function isCanvasImageInputSourceNodeType(
  value: unknown,
): value is CanvasConnectionNodeKind {
  return (
    typeof value === 'string' &&
    CANVAS_IMAGE_INPUT_SOURCE_NODE_TYPE_SET.has(value as CanvasConnectionNodeKind)
  )
}

function getCanvasNodeRegistryItem(nodeType: CanvasConnectionNodeKind): CanvasNodeRegistryItem {
  return CANVAS_NODE_REGISTRY[nodeType]
}

export function createTypedCanvasNodeSnapshot<T extends CanvasConnectionNodeKind>(
  nodeType: T,
  point: CanvasPoint,
  id: number,
): CanvasNodeItemByKind[T] {
  return getCanvasNodeRegistryItem(nodeType).createSnapshot(point, id) as CanvasNodeItemByKind[T]
}

function getCanvasNodePortContracts(
  nodeType: CanvasConnectionNodeKind,
): readonly CanvasNodePortContract[] {
  return getCanvasNodeRegistryItem(nodeType).ports
}

export function getCanvasNodeVisibleInputPorts(
  nodeType: CanvasConnectionNodeKind,
): CanvasNodePortContract[] {
  return getCanvasNodePortContracts(nodeType).filter(
    (port) => port.direction === 'input' && port.visible,
  )
}

export function getCanvasNodeVisibleOutputPorts(
  nodeType: CanvasConnectionNodeKind,
): CanvasNodePortContract[] {
  return getCanvasNodePortContracts(nodeType).filter(
    (port) => port.direction === 'output' && port.visible,
  )
}

export function getCanvasNodeRequiredInputPorts(
  nodeType: CanvasConnectionNodeKind,
): CanvasNodePortContract[] {
  return getCanvasNodeVisibleInputPorts(nodeType).filter((port) => port.required)
}

export function getCanvasNodePrimaryOutputPort(
  nodeType: CanvasConnectionNodeKind,
): CanvasConnectionPort {
  return getCanvasNodeRegistryItem(nodeType).primaryOutputPort
}

export function getCanvasNodePortInputLimit(
  nodeType: CanvasConnectionNodeKind,
  port: CanvasConnectionPort,
): number {
  const portContract = getCanvasNodeVisibleInputPorts(nodeType).find((item) => item.port === port)
  return portContract?.maxConnections ?? 1
}

export function getCanvasNodeImageInputLimit(nodeType: CanvasConnectionNodeKind): number {
  return getCanvasNodePortInputLimit(nodeType, 'image')
}
