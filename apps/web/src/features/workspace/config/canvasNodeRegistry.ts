import {
  DEFAULT_FUSION_NODE_OUTPUT_COUNT,
  DEFAULT_IMAGE_NODE_OUTPUT_COUNT,
  DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
  type CanvasBoardSnapshot,
  type CanvasConnectionNodeType,
  type CanvasConnectionPort,
  type CanvasExecutionOutputNodeType,
  type CanvasNodeItemByKind,
  type CanvasNodeItem,
  type CanvasPoint,
  type FusionNodeItem,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../types/canvasNodes'
import { DEFAULT_PROMPT_ASPECT_RATIO } from './promptAspectRatio'
import { DEFAULT_FUSION_BLEND_POINT } from '../utils/fusionBlend'

export type CanvasSnapshotNodeCollectionKey = keyof Pick<
  CanvasBoardSnapshot,
  'imageNodes' | 'promptNodes' | 'fusionNodes'
>

export type CanvasCreatableNodeKind = CanvasConnectionNodeType

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

export interface CanvasNodeMenuItem {
  key: CanvasCreatableNodeKind
  label: string
  group?: string
  order?: number
}

type ExecutionActionLabels = { ready: string; running: string }

type CanvasNodeRegistryItem = {
  kind: CanvasConnectionNodeType
  snapshotType: CanvasNodeItem['type']
  snapshotCollection: CanvasSnapshotNodeCollectionKey
  defaultTitle: string
  defaultWidth: number
  ariaLabel: string
  menu: CanvasNodeMenuItem
  ports: readonly CanvasNodePortContract[]
  primaryOutputPort: CanvasConnectionPort
  imageInputSource: boolean
  executionOutput: boolean
  primaryAction?: ExecutionActionLabels
  createSnapshot: (point: CanvasPoint, id: number) => CanvasNodeItem
}

const CANVAS_NODE_DEFAULTS = {
  image: { title: 'Image', width: 280 },
  prompt: { title: 'Prompt', width: 280 },
  fusion: { title: 'Fusion', width: 286 },
} as const

export const CANVAS_MEDIA_INPUT_IMAGE_LIMIT = 4

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
    title: CANVAS_NODE_DEFAULTS.prompt.title,
    text: '',
    outputCount: DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
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
    title: CANVAS_NODE_DEFAULTS.image.title,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    outputCount: DEFAULT_IMAGE_NODE_OUTPUT_COUNT,
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
    title: CANVAS_NODE_DEFAULTS.fusion.title,
    prompt: '',
    outputCount: DEFAULT_FUSION_NODE_OUTPUT_COUNT,
    aspectRatio: DEFAULT_PROMPT_ASPECT_RATIO,
    blendPoint: { ...DEFAULT_FUSION_BLEND_POINT },
    inputImages: [],
    outputStatus: 'idle',
    outputImages: [],
  }
}

export const CANVAS_NODE_REGISTRY = {
  image: {
    kind: 'image',
    snapshotType: 'image-node',
    snapshotCollection: 'imageNodes',
    defaultTitle: CANVAS_NODE_DEFAULTS.image.title,
    defaultWidth: CANVAS_NODE_DEFAULTS.image.width,
    ariaLabel: '图片节点',
    menu: {
      key: 'image',
      label: '+ Image 节点',
      group: 'source',
      order: 10,
    },
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
    defaultTitle: CANVAS_NODE_DEFAULTS.prompt.title,
    defaultWidth: CANVAS_NODE_DEFAULTS.prompt.width,
    ariaLabel: '提示词节点',
    menu: {
      key: 'prompt',
      label: '+ Prompt 节点',
      group: 'source',
      order: 20,
    },
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
    defaultTitle: CANVAS_NODE_DEFAULTS.fusion.title,
    defaultWidth: CANVAS_NODE_DEFAULTS.fusion.width,
    ariaLabel: '融合节点',
    menu: {
      key: 'fusion',
      label: '+ Fusion 节点',
      group: 'process',
      order: 60,
    },
    ports: [
      input('image', 'image', { required: true, maxConnections: CANVAS_MEDIA_INPUT_IMAGE_LIMIT }),
      input('prompt', 'prompt'),
      output('image', 'image'),
    ],
    primaryOutputPort: 'image',
    imageInputSource: true,
    executionOutput: true,
    primaryAction: { ready: '融合', running: '融合中...' },
    createSnapshot: createFusionSnapshot,
  },
} as const satisfies Record<CanvasConnectionNodeType, CanvasNodeRegistryItem>

export const CANVAS_NODE_REGISTRY_LIST = Object.values(CANVAS_NODE_REGISTRY)

export const CANVAS_CONNECTION_NODE_TYPES = CANVAS_NODE_REGISTRY_LIST.map(
  (item) => item.kind,
) as readonly CanvasConnectionNodeType[]

export const CANVAS_EXECUTION_OUTPUT_NODE_TYPES = CANVAS_NODE_REGISTRY_LIST.filter(
  (item) => item.executionOutput,
).map((item) => item.kind) as readonly CanvasExecutionOutputNodeType[]

export const CANVAS_IMAGE_INPUT_SOURCE_NODE_TYPES = CANVAS_NODE_REGISTRY_LIST.filter(
  (item) => item.imageInputSource,
).map((item) => item.kind) as readonly CanvasConnectionNodeType[]

const CANVAS_CONNECTION_NODE_TYPE_SET = new Set<CanvasConnectionNodeType>(
  CANVAS_CONNECTION_NODE_TYPES,
)
const CANVAS_EXECUTION_OUTPUT_NODE_TYPE_SET = new Set<CanvasExecutionOutputNodeType>(
  CANVAS_EXECUTION_OUTPUT_NODE_TYPES,
)
const CANVAS_IMAGE_INPUT_SOURCE_NODE_TYPE_SET = new Set<CanvasConnectionNodeType>(
  CANVAS_IMAGE_INPUT_SOURCE_NODE_TYPES,
)

export function isCanvasConnectionNodeType(value: unknown): value is CanvasConnectionNodeType {
  return (
    typeof value === 'string' &&
    CANVAS_CONNECTION_NODE_TYPE_SET.has(value as CanvasConnectionNodeType)
  )
}

export function isCanvasExecutionOutputNodeType(
  value: unknown,
): value is CanvasExecutionOutputNodeType {
  return (
    typeof value === 'string' &&
    CANVAS_EXECUTION_OUTPUT_NODE_TYPE_SET.has(value as CanvasExecutionOutputNodeType)
  )
}

export function isCanvasImageInputSourceNodeType(
  value: unknown,
): value is CanvasConnectionNodeType {
  return (
    typeof value === 'string' &&
    CANVAS_IMAGE_INPUT_SOURCE_NODE_TYPE_SET.has(value as CanvasConnectionNodeType)
  )
}

export function getCanvasNodeRegistryItem(
  nodeType: CanvasConnectionNodeType,
): CanvasNodeRegistryItem {
  return CANVAS_NODE_REGISTRY[nodeType]
}

export function getDefaultCanvasNodeTitle(nodeType: CanvasConnectionNodeType): string {
  return getCanvasNodeRegistryItem(nodeType).defaultTitle
}

export function getCanvasNodeAriaLabel(nodeType: CanvasConnectionNodeType): string {
  return getCanvasNodeRegistryItem(nodeType).ariaLabel
}

export function getCanvasNodeSnapshotCollectionKey(
  nodeType: CanvasConnectionNodeType,
): CanvasSnapshotNodeCollectionKey {
  return getCanvasNodeRegistryItem(nodeType).snapshotCollection
}

export function createCanvasNodeSnapshot(
  nodeType: CanvasConnectionNodeType,
  point: CanvasPoint,
  id: number,
): CanvasNodeItem {
  return getCanvasNodeRegistryItem(nodeType).createSnapshot(point, id)
}

export function createTypedCanvasNodeSnapshot<T extends CanvasConnectionNodeType>(
  nodeType: T,
  point: CanvasPoint,
  id: number,
): CanvasNodeItemByKind[T] {
  return getCanvasNodeRegistryItem(nodeType).createSnapshot(point, id) as CanvasNodeItemByKind[T]
}

export function getCanvasNodePortContracts(
  nodeType: CanvasConnectionNodeType,
): readonly CanvasNodePortContract[] {
  return getCanvasNodeRegistryItem(nodeType).ports
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

export function getCanvasNodePrimaryOutputPort(
  nodeType: CanvasConnectionNodeType,
): CanvasConnectionPort {
  return getCanvasNodeRegistryItem(nodeType).primaryOutputPort
}

export function getCanvasNodePortInputLimit(
  nodeType: CanvasConnectionNodeType,
  port: CanvasConnectionPort,
): number {
  const portContract = getCanvasNodeVisibleInputPorts(nodeType).find((item) => item.port === port)
  return portContract?.maxConnections ?? 1
}

export function getCanvasNodeImageInputLimit(nodeType: CanvasConnectionNodeType): number {
  return getCanvasNodePortInputLimit(nodeType, 'image')
}

export function getCanvasNodeExecutionActionLabels(
  nodeType: CanvasExecutionOutputNodeType,
): ExecutionActionLabels {
  const labels = getCanvasNodeRegistryItem(nodeType).primaryAction
  if (!labels) throw new Error(`Canvas node ${nodeType} has no execution action labels`)
  return labels
}

export function formatCanvasNodePortLabel(port: CanvasConnectionPort): string {
  if (port === 'image') return 'Image'
  if (port === 'prompt') return 'Prompt'
  return port
}

export const CANVAS_NODE_MENU_ITEMS = CANVAS_NODE_REGISTRY_LIST.map(
  (item) => item.menu,
) as readonly CanvasNodeMenuItem[]
