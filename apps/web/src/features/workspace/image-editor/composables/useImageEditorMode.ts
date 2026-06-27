import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { ImageEditorMode } from '../types'
import { containImageRect } from '../../utils/canvasExtendGeometry'
import { isEditableEventTarget } from '../../utils/domEventTargets'

type ImageDisplayPayload = {
  url: string
  ratio: number
}

export type ImageEditorImageRect = {
  left: number
  top: number
  width: number
  height: number
}

export type ImageEditorModeState = {
  nodeId: string
  mode: ImageEditorMode
  imageRect: ImageEditorImageRect
}

type UseImageEditorModeOptions = {
  nodes: Ref<Node[]>
  activeTool: Ref<string>
  getImageNodeDisplayPayload: (nodeId: string) => ImageDisplayPayload | null
  getImageStageElement: (nodeId: string) => HTMLElement | null
  onBeforePointerCancel?: () => void
  onBeforeExit?: (state: ImageEditorModeState) => void
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
    if (!current) return
    if (exitOptions.runBeforeExit !== false) {
      options.onBeforeExit?.(current)
    }
    state.value = null
  }

  function enterImageEditorMode(nodeId: string, mode: ImageEditorMode): boolean {
    if (!isEditableImageNode(nodeId)) return false
    const display = options.getImageNodeDisplayPayload(nodeId)
    if (!display?.url) return false
    const stage = options.getImageStageElement(nodeId)
    if (!stage) return false
    const containRect = containImageRect(stage.clientWidth, stage.clientHeight, display.ratio)
    if (!containRect) return false

    exitImageEditorMode()
    state.value = {
      nodeId,
      mode,
      imageRect: containRect,
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

  if (typeof window !== 'undefined') {
    window.addEventListener('pointerdown', onGlobalPointerDown, true)
    window.addEventListener('keydown', onGlobalKeydown, true)
  }

  onBeforeUnmount(() => {
    if (typeof window === 'undefined') return
    window.removeEventListener('pointerdown', onGlobalPointerDown, true)
    window.removeEventListener('keydown', onGlobalKeydown, true)
  })

  return {
    imageEditorState: state,
    imageEditorMode,
    imageEditorNodeId,
    enterImageEditorMode,
    exitImageEditorMode,
  }
}
