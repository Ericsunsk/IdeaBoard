import { computed, type CSSProperties } from 'vue'
import { Position } from '@vue-flow/core'
import { i18n } from '../../../shared/i18n'
import {
  getCanvasNodeVisibleInputPorts,
  getCanvasNodeVisibleOutputPorts,
  type CanvasConnectionNodeType,
  type CanvasConnectionPort,
} from '../../board/domain'
import { useCanvasHandleProximity } from './useCanvasHandleProximity'
import { resolveCanvasNodeUiScale, useCanvasViewportContext } from './useCanvasViewportContext'

export type FlowNodeHandleDirection = 'input' | 'output'

type FlowNodeAnchorHandlePresentation = {
  id: string
  direction: FlowNodeHandleDirection
  handleType: 'source' | 'target'
  position: Position
  style: CSSProperties
}

type FlowNodeAggregateHandlePresentation = {
  id: string
  direction: FlowNodeHandleDirection
  handleType: 'source' | 'target'
  position: Position
  connectable: boolean
  visible: boolean
  proximityActive: boolean
  style: CSSProperties
  ariaLabel: string
  title: string
}

type FlowNodeHandlesPresentation = {
  anchors: FlowNodeAnchorHandlePresentation[]
  aggregates: FlowNodeAggregateHandlePresentation[]
}

type UseFlowNodeHandlesPresentationOptions = {
  nodeId: () => string
  kind: () => CanvasConnectionNodeType
  showHandles: () => boolean
  disabled: () => boolean
}

type HandleDirectionPolicy = {
  centeredStyle: CSSProperties
  handleType: 'source' | 'target'
  position: Position
  getPorts: (kind: CanvasConnectionNodeType) => CanvasConnectionPort[]
  getAggregateId: (ids: string[]) => string | null
  getAnchorIds: (ids: string[]) => string[]
  portPrefix: 'in' | 'out'
  directionKey: 'input' | 'output'
}

const HANDLE_DIRECTION_POLICIES: Record<FlowNodeHandleDirection, HandleDirectionPolicy> = {
  input: {
    centeredStyle: {
      left: '0',
      top: '50%',
      transform: 'translateY(-50%)',
    },
    getPorts: (kind) => getCanvasNodeVisibleInputPorts(kind).map(({ port }) => port),
    getAggregateId: (ids) => (ids.length > 0 ? 'in:__auto' : null),
    getAnchorIds: (ids) => ids,
    handleType: 'target',
    position: Position.Left,
    portPrefix: 'in',
    directionKey: 'input',
  },
  output: {
    centeredStyle: {
      right: '0',
      top: '50%',
      transform: 'translateY(-50%)',
    },
    getPorts: (kind) => getCanvasNodeVisibleOutputPorts(kind).map(({ port }) => port),
    getAggregateId: (ids) => ids[0] ?? null,
    getAnchorIds: (ids) => ids.slice(1),
    handleType: 'source',
    position: Position.Right,
    portPrefix: 'out',
    directionKey: 'output',
  },
}

function createFollowStyle(
  centeredStyle: CSSProperties,
  point: { x: number; y: number },
  localScale: number,
): CSSProperties {
  const x = point.x * localScale
  const y = point.y * localScale
  return {
    ...centeredStyle,
    '--handle-plus-x': `${x}px`,
    '--handle-plus-y': `${y}px`,
    '--handle-plus-hit-x': `${x}px`,
    '--handle-plus-hit-y': `${y}px`,
  }
}

function getNodeKindLabel(kind: CanvasConnectionNodeType) {
  const { t } = i18n.global
  const labelKeys: Partial<Record<CanvasConnectionNodeType, string>> = {
    prompt: 'canvas.handles.promptNodeLabel',
    image: 'canvas.handles.imageNodeLabel',
    fusion: 'canvas.handles.fusionNodeLabel',
  }
  const labelKey = labelKeys[kind]
  return labelKey ? t(labelKey) : t('canvas.handles.fallbackNodeLabel')
}

function useHandleDirectionPresentation(
  direction: FlowNodeHandleDirection,
  options: UseFlowNodeHandlesPresentationOptions,
  localScale: () => number,
  nodeKindLabel: () => string,
) {
  const policy = HANDLE_DIRECTION_POLICIES[direction]
  const portIds = computed(() =>
    policy.getPorts(options.kind()).map((port) => `${policy.portPrefix}:${port}`),
  )
  const proximity = useCanvasHandleProximity({
    nodeId: options.nodeId(),
    direction,
    enabled: computed(() => portIds.value.length > 0 && !options.disabled()),
  })
  const visible = computed(
    () => !options.disabled() && (options.showHandles() || proximity.isActive.value),
  )
  const anchors = computed<FlowNodeAnchorHandlePresentation[]>(() =>
    policy.getAnchorIds(portIds.value).map((id) => ({
      id,
      direction,
      handleType: policy.handleType,
      position: policy.position,
      style: policy.centeredStyle,
    })),
  )
  const aggregate = computed<FlowNodeAggregateHandlePresentation | null>(() => {
    const id = policy.getAggregateId(portIds.value)
    if (!id) return null

    return {
      id,
      direction,
      handleType: policy.handleType,
      position: policy.position,
      connectable: !options.disabled(),
      visible: visible.value,
      proximityActive: proximity.isActive.value,
      style: proximity.isActive.value
        ? createFollowStyle(policy.centeredStyle, proximity.point.value, localScale())
        : policy.centeredStyle,
      ariaLabel: i18n.global.t('canvas.handles.connectHandleAria', {
        kind: nodeKindLabel(),
        direction: i18n.global.t(`canvas.handles.${policy.directionKey}DirectionLabel`),
      }),
      title: i18n.global.t(`canvas.handles.${policy.directionKey}Title`),
    }
  })

  return {
    aggregate,
    anchors,
  }
}

export function useFlowNodeHandlesPresentation(options: UseFlowNodeHandlesPresentationOptions) {
  const { zoomLevel } = useCanvasViewportContext()
  const localScale = computed(() => resolveCanvasNodeUiScale(zoomLevel.value))
  const nodeKindLabel = computed(() => getNodeKindLabel(options.kind()))

  const input = useHandleDirectionPresentation(
    'input',
    options,
    () => localScale.value,
    () => nodeKindLabel.value,
  )
  const output = useHandleDirectionPresentation(
    'output',
    options,
    () => localScale.value,
    () => nodeKindLabel.value,
  )

  return computed<FlowNodeHandlesPresentation>(() => ({
    anchors: [...input.anchors.value, ...output.anchors.value],
    aggregates: [input.aggregate.value, output.aggregate.value].filter(
      (aggregate): aggregate is FlowNodeAggregateHandlePresentation => aggregate !== null,
    ),
  }))
}
