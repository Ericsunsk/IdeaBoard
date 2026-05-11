import { computed, ref } from 'vue'
import { resolveFlowEdgeToneFromHandles } from '../utils/flowEdgeVisual'

const TONE_COLOR_MAP: Record<string, string> = {
  image: '#5b8def',
  mask: '#34c38f',
  prompt: '#f59e0b',
}

export function useFlowConnectionVisuals() {
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
  }

  function onFlowConnectEnd() {
    connectingSourceHandle.value = null
  }

  function onFlowEdgeUpdateEnd() {
    connectingSourceHandle.value = null
  }

  return {
    connectingSourceHandle,
    flowConnectionLineStyle,
    onFlowConnectStart,
    onFlowConnectEnd,
    onFlowEdgeUpdateEnd,
  }
}
