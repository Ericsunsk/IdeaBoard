import { useEventListener } from '@vueuse/core'
import { computed, ref, watch, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { ImageEditorMode } from './types'
import { isEditableEventTarget } from '../utils/domEventTargets'

const windowTarget = () => (typeof window === 'undefined' ? null : window)

type ImageDisplayPayload = {
  url: string
  ratio: number
}

export type ImageEditorModeState = {
  nodeId: string
  mode: ImageEditorMode
}

type UseImageEditorModeOptions = {
  nodes: Ref<Node[]>
  activeTool: Ref<string>
  getImageNodeDisplayPayload: (nodeId: string) => ImageDisplayPayload | null
  onBeforePointerCancel?: () => void
  onBeforeExit?: (state: ImageEditorModeState) => boolean | void
}

export function useImageEditorMode(options: UseImageEditorModeOptions) {
  const state = ref<ImageEditorModeState | null>(null)

  const imageEditorMode = computed(() => state.value?.mode ?? null)
  const imageEditorNodeId = computed(() => state.value?.nodeId ?? null)
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
  function exitImageEditorMode(exitOptions: { runBeforeExit?: boolean } = {}) {
    const current = state.value
    if (!current) return true
    if (exitOptions.runBeforeExit !== false) {
      const shouldExit = options.onBeforeExit?.(current)
      if (shouldExit === false) return false
    }
    state.value = null
    return true
  }

  function enterImageEditorMode(nodeId: string, mode: ImageEditorMode): boolean {
    if (!isEditableImageNode(nodeId)) return false
    const display = options.getImageNodeDisplayPayload(nodeId)
    if (!display?.url) return false

    if (!exitImageEditorMode()) return false
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
    const inEditor = !!target.closest('[data-image-mask-editor], [data-image-editor]')
    if (inEditor) return
    options.onBeforePointerCancel?.()
    exitImageEditorMode()
  }

  function onGlobalKeydown(event: KeyboardEvent) {
    if (!state.value) return
    if (isEditableEventTarget(event.target)) return
    if (event.key !== 'Escape') return
    const target = event.target as HTMLElement | null
    if (target?.closest('[data-image-editor-tool-rail], [data-image-editor-popover]')) return
    if (document.documentElement.dataset.imageEditorPopoverOpen === 'true') return
    event.preventDefault()
    exitImageEditorMode()
  }

  watch(
    () => options.activeTool.value,
    () => {
      if (options.activeTool.value !== 'select' && state.value) {
        exitImageEditorMode()
      }
    },
  )

  watch(activeEditorNodeKind, (kind) => {
    if (!state.value) return
    if (kind !== 'image') exitImageEditorMode()
  })

  useEventListener<PointerEvent>(windowTarget, 'pointerdown', onGlobalPointerDown, true)
  useEventListener<KeyboardEvent>(windowTarget, 'keydown', onGlobalKeydown, true)

  return {
    imageEditorState: state,
    imageEditorMode,
    imageEditorNodeId,
    enterImageEditorMode,
    exitImageEditorMode,
  }
}
