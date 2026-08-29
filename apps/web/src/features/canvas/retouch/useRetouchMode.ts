import { useEventListener } from '@vueuse/core'
import { computed, ref, watch, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { RetouchMode } from './types'
import { isEditableEventTarget } from '../utils/domEventTargets'

const windowTarget = () => (typeof window === 'undefined' ? null : window)

type ImageDisplayPayload = {
  url: string
  ratio: number
}

export type RetouchModeState = {
  nodeId: string
  mode: RetouchMode
}

type UseRetouchModeOptions = {
  nodes: Ref<Node[]>
  activeTool: Ref<string>
  getImageNodeDisplayPayload: (nodeId: string) => ImageDisplayPayload | null
  onBeforePointerCancel?: () => void
  onBeforeExit?: (state: RetouchModeState) => boolean | void
}

export function useRetouchMode(options: UseRetouchModeOptions) {
  const state = ref<RetouchModeState | null>(null)

  const retouchMode = computed(() => state.value?.mode ?? null)
  const retouchNodeId = computed(() => state.value?.nodeId ?? null)
  function getNodeKind(nodeId: string) {
    return options.nodes.value.find((item) => item.id === nodeId)?.data?.kind ?? null
  }
  function isEditableImageNode(nodeId: string) {
    return getNodeKind(nodeId) === 'image'
  }
  const activeEditorNodeKind = computed(() => {
    const nodeId = state.value?.nodeId
    if (!nodeId) return null
    return getNodeKind(nodeId)
  })
  function exitRetouchMode(exitOptions: { runBeforeExit?: boolean } = {}) {
    const current = state.value
    if (!current) return true
    if (exitOptions.runBeforeExit !== false) {
      const shouldExit = options.onBeforeExit?.(current)
      if (shouldExit === false) return false
    }
    state.value = null
    return true
  }

  function enterRetouchMode(nodeId: string, mode: RetouchMode): boolean {
    if (!isEditableImageNode(nodeId)) return false
    const display = options.getImageNodeDisplayPayload(nodeId)
    if (!display?.url) return false

    if (!exitRetouchMode()) return false
    state.value = {
      nodeId,
      mode,
    }
    return true
  }

  function onGlobalPointerDown(event: PointerEvent) {
    if (!state.value || event.button !== 0) return
    const target = event.target as HTMLElement | null
    if (!target) return
    const inEditor = !!target.closest('[data-inpaint], [data-retouch]')
    if (inEditor) return
    options.onBeforePointerCancel?.()
    exitRetouchMode()
  }

  function onGlobalKeydown(event: KeyboardEvent) {
    if (!state.value) return
    if (isEditableEventTarget(event.target)) return
    if (event.key !== 'Escape') return
    const target = event.target as HTMLElement | null
    if (target?.closest('[data-retouch-tool-rail], [data-retouch-popover]')) return
    if (document.documentElement.dataset.retouchPopoverOpen === 'true') return
    event.preventDefault()
    exitRetouchMode()
  }

  watch(
    () => options.activeTool.value,
    () => {
      if (options.activeTool.value !== 'select' && state.value) {
        exitRetouchMode()
      }
    },
  )

  watch(activeEditorNodeKind, (kind) => {
    if (!state.value) return
    if (kind !== 'image') exitRetouchMode()
  })

  useEventListener<PointerEvent>(windowTarget, 'pointerdown', onGlobalPointerDown, true)
  useEventListener<KeyboardEvent>(windowTarget, 'keydown', onGlobalKeydown, true)

  return {
    retouchState: state,
    retouchMode,
    retouchNodeId,
    enterRetouchMode,
    exitRetouchMode,
  }
}
