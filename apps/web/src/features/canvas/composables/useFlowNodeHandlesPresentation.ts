import {
  computed,
  onBeforeUnmount,
  ref,
  shallowRef,
  watch,
  type ComponentPublicInstance,
  type CSSProperties,
} from 'vue'
import { Position } from '@vue-flow/core'
import {
  getCanvasNodeVisibleInputPorts,
  getCanvasNodeVisibleOutputPorts,
  type CanvasConnectionNodeType,
  type CanvasConnectionPort,
} from '../../canvas-board/domain'
import { useCanvasHandleProximity } from './useCanvasHandleProximity'
import { resolveCanvasNodeUiScale, useCanvasViewportContext } from './useCanvasViewportContext'

export type FlowNodeHandleDirection = 'input' | 'output'

type FlowNodeHandleElement = Element | ComponentPublicInstance | null

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
  returning: boolean
  disabled: boolean
  style: CSSProperties
  ariaLabel: string
  title: string
  setElement: (value: FlowNodeHandleElement) => void
  onPointerDown: (event: PointerEvent) => void
}

export type FlowNodeHandlesPresentation = {
  anchors: FlowNodeAnchorHandlePresentation[]
  aggregates: FlowNodeAggregateHandlePresentation[]
}

type UseFlowNodeHandlesPresentationOptions = {
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
  ariaDirectionLabel: string
  title: string
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
    ariaDirectionLabel: '输入',
    title: '拖拽创建输入连接',
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
    ariaDirectionLabel: '输出',
    title: '拖拽创建输出连接',
  },
}

const HANDLE_HIT_TARGET_SCREEN_SIZE = 52

function resolveHandleElement(value: FlowNodeHandleElement): HTMLElement | null {
  if (value instanceof HTMLElement) return value
  if (!value || value instanceof Element) return null
  const element = value.$el
  return element instanceof HTMLElement ? element : null
}

function createFollowStyle(
  centeredStyle: CSSProperties,
  point: { x: number; y: number },
  localScale: number,
): CSSProperties {
  const x = point.x * localScale
  const y = point.y * localScale
  return {
    ...createScreenSizedHandleStyle(centeredStyle, localScale),
    '--handle-plus-x': `${x}px`,
    '--handle-plus-y': `${y}px`,
    '--handle-plus-hit-x': `${x}px`,
    '--handle-plus-hit-y': `${y}px`,
  }
}

function createScreenSizedHandleStyle(
  centeredStyle: CSSProperties,
  localScale: number,
): CSSProperties {
  return {
    ...centeredStyle,
    '--handle-hit-size': `${HANDLE_HIT_TARGET_SCREEN_SIZE * localScale}px`,
  }
}

function getNodeKindLabel(kind: CanvasConnectionNodeType) {
  const labels: Partial<Record<CanvasConnectionNodeType, string>> = {
    prompt: '提示词节点',
    image: '图片节点',
    fusion: '融合节点',
  }
  return labels[kind] ?? '节点'
}

function useHandleDirectionPresentation(
  direction: FlowNodeHandleDirection,
  options: UseFlowNodeHandlesPresentationOptions,
  localScale: () => number,
  nodeKindLabel: () => string,
  isConnectionDragging: () => boolean,
  onConnectionPointerDown: (event: PointerEvent) => void,
) {
  const policy = HANDLE_DIRECTION_POLICIES[direction]
  const element = shallowRef<FlowNodeHandleElement>(null)
  const portIds = computed(() =>
    policy.getPorts(options.kind()).map((port) => `${policy.portPrefix}:${port}`),
  )
  const proximity = useCanvasHandleProximity({
    direction,
    getAnchorRect: () => resolveHandleElement(element.value)?.getBoundingClientRect() ?? null,
    enabled: computed(() => portIds.value.length > 0 && !options.disabled()),
  })
  const visible = computed(
    () =>
      !options.disabled() &&
      (options.showHandles() ||
        isConnectionDragging() ||
        proximity.isActive.value ||
        proximity.isReturning.value),
  )
  const setElement = (value: FlowNodeHandleElement) => {
    element.value = value
  }
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
      connectable: visible.value,
      visible: visible.value,
      proximityActive: !isConnectionDragging() && proximity.isActive.value,
      returning: !isConnectionDragging() && proximity.isReturning.value,
      disabled: options.disabled(),
      style: proximity.isActive.value && !isConnectionDragging()
        ? createFollowStyle(policy.centeredStyle, proximity.point.value, localScale())
        : createScreenSizedHandleStyle(policy.centeredStyle, localScale()),
      ariaLabel: `${nodeKindLabel()}${policy.ariaDirectionLabel}连接点，拖拽创建连接`,
      title: policy.title,
      setElement,
      onPointerDown: onConnectionPointerDown,
    }
  })

  return {
    aggregate,
    anchors,
    portIds,
    refreshCoordinator: proximity.refreshCoordinator,
  }
}

export function useFlowNodeHandlesPresentation(options: UseFlowNodeHandlesPresentationOptions) {
  const { zoomLevel } = useCanvasViewportContext()
  const localScale = computed(() => resolveCanvasNodeUiScale(zoomLevel.value))
  const nodeKindLabel = computed(() => getNodeKindLabel(options.kind()))
  const isConnectionDragging = ref(false)
  let refreshCoordinator: () => void = () => undefined

  function finishConnectionDrag(shouldRefresh = true) {
    if (!isConnectionDragging.value) return
    isConnectionDragging.value = false
    if (typeof window !== 'undefined') {
      window.removeEventListener('pointerup', onConnectionPointerEnd, true)
      window.removeEventListener('pointercancel', onConnectionPointerEnd, true)
      window.removeEventListener('blur', onConnectionPointerEnd)
    }
    if (shouldRefresh) refreshCoordinator()
  }

  function onConnectionPointerEnd() {
    finishConnectionDrag()
  }

  function onConnectionPointerDown(event: PointerEvent) {
    if ((event.pointerType === 'mouse' || event.pointerType === 'pen') && event.button !== 0) return
    isConnectionDragging.value = true
    if (typeof window === 'undefined') return
    window.addEventListener('pointerup', onConnectionPointerEnd, true)
    window.addEventListener('pointercancel', onConnectionPointerEnd, true)
    window.addEventListener('blur', onConnectionPointerEnd)
  }

  const input = useHandleDirectionPresentation(
    'input',
    options,
    () => localScale.value,
    () => nodeKindLabel.value,
    () => isConnectionDragging.value,
    onConnectionPointerDown,
  )
  const output = useHandleDirectionPresentation(
    'output',
    options,
    () => localScale.value,
    () => nodeKindLabel.value,
    () => isConnectionDragging.value,
    onConnectionPointerDown,
  )
  refreshCoordinator = input.refreshCoordinator

  onBeforeUnmount(() => finishConnectionDrag(false))

  watch(
    zoomLevel,
    input.refreshCoordinator,
    { flush: 'post' },
  )

  const presentation = computed<FlowNodeHandlesPresentation>(() => ({
    anchors: [...input.anchors.value, ...output.anchors.value],
    aggregates: [input.aggregate.value, output.aggregate.value].filter(
      (aggregate): aggregate is FlowNodeAggregateHandlePresentation => aggregate !== null,
    ),
  }))
  // Vue Flow internals refresh only when the rendered Handle identity set changes.
  const handleLayoutIdentity = computed(
    () => `${options.kind()}|${input.portIds.value.join(',')}|${output.portIds.value.join(',')}`,
  )

  return { handleLayoutIdentity, presentation }
}
