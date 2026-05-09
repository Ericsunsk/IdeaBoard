import { computed, ref, watch, type Ref } from 'vue'
import type { Edge } from '@vue-flow/core'
import { resolveFlowEdgeToneFromHandles, buildFlowEdgeClassName } from '../utils/flowEdgeVisual'
import type { CanvasInteractionKey } from './useFlowCanvasInteractionState'

type UseFlowConnectionVisualsOptions = {
  edges: Ref<Edge[]>
  startFlowInteraction: (key: CanvasInteractionKey) => void
  stopFlowInteraction: (key: CanvasInteractionKey) => void
}

const TONE_COLOR_MAP: Record<string, string> = {
  image: '#5b8def',
  mask: '#34c38f',
  prompt: '#f59e0b',
}

export function useFlowConnectionVisuals(options: UseFlowConnectionVisualsOptions) {
  const connectingSourceHandle = ref<string | null>(null)

  const flowConnectionLineStyle = computed(() => {
    const tone = resolveFlowEdgeToneFromHandles(connectingSourceHandle.value, null)
    return {
      stroke: TONE_COLOR_MAP[tone] || '#9278e6',
      strokeWidth: '2',
      opacity: '0.9',
    }
  })

  function onFlowConnectStart(event: { handleId?: string | null }) {
    connectingSourceHandle.value = event.handleId ?? null
    options.startFlowInteraction('connect')
  }

  function onFlowConnectEnd() {
    connectingSourceHandle.value = null
    options.stopFlowInteraction('connect')
  }

  function onFlowEdgeUpdateStart() {
    options.startFlowInteraction('edge-update')
  }

  function onFlowEdgeUpdateEnd() {
    connectingSourceHandle.value = null
    options.stopFlowInteraction('edge-update')
  }

  function buildFlowEdgeClass(edge: { selected?: boolean; sourceHandle?: string | null; targetHandle?: string | null }): string {
    const tone = resolveFlowEdgeToneFromHandles(edge.sourceHandle, edge.targetHandle)
    return buildFlowEdgeClassName(tone, !!edge.selected)
  }

  function ensureFlowEdgeVisuals() {
    const source = options.edges.value as Array<Edge & { data?: { tone?: string } }>
    const withVisuals = source.map((edge): Edge => {
      const nextClass = buildFlowEdgeClass(edge)
      const tone = resolveFlowEdgeToneFromHandles(edge.sourceHandle, edge.targetHandle)
      return {
        ...edge,
        animated: false,
        class: nextClass,
        data: { ...(edge.data ?? {}), tone },
      }
    })
    const same = source
      .map(edge => `${edge.id}|${edge.class ?? ''}|${edge.animated ? 1 : 0}|${edge.data?.tone ?? ''}`)
      .join(';')
      === withVisuals
        .map(edge => `${edge.id}|${edge.class ?? ''}|${edge.animated ? 1 : 0}|${edge.data?.tone ?? ''}`)
        .join(';')
    if (!same) {
      options.edges.value = withVisuals
    }
  }

  // Keep edge class/tone in sync when edges change
  watch(
    () => (options.edges.value as Array<Edge & { selected?: boolean }>).map(edge => `${edge.id}:${edge.selected ? 1 : 0}:${edge.sourceHandle ?? ''}:${edge.targetHandle ?? ''}`).join('|'),
    () => {
      ensureFlowEdgeVisuals()
    },
  )

  return {
    flowConnectionLineStyle,
    onFlowConnectStart,
    onFlowConnectEnd,
    onFlowEdgeUpdateStart,
    onFlowEdgeUpdateEnd,
    buildFlowEdgeClass,
    ensureFlowEdgeVisuals,
  }
}
