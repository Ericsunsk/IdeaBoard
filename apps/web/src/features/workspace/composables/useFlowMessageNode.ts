import type { ComponentPublicInstance, Ref } from 'vue'
import type { CanvasImageItem } from '../types/canvasNodes'

export type ResizeDirection = 'n' | 's' | 'e' | 'w' | 'nw' | 'ne' | 'sw' | 'se'
export type MessageAction = 'remove-bg' | 'style-innovate' | 'to-vector' | 'sketch-to-design'

export const MESSAGE_RESIZE_DIRECTIONS: ResizeDirection[] = ['n', 's', 'e', 'w', 'nw', 'ne', 'sw', 'se']

const STYLE_STRENGTH_MENU_GAP = 16

export function useFlowMessageNode(messageSnapshot: Ref<CanvasImageItem | null>) {
  const messageImageElement = ref<HTMLImageElement | null>(null)
  const messageImageLayoutVersion = ref(0)

  const messageDisplayWidth = computed(() => {
    const width = Number(messageSnapshot.value?.width)
    return Number.isFinite(width) && width > 0 ? width : 300
  })

  const messageDisplayHeight = computed(() => {
    const width = messageDisplayWidth.value
    const directRatio = Number(messageSnapshot.value?.height_ratio)
    if (Number.isFinite(directRatio) && directRatio > 0) {
      return Math.max(72, Math.round(width * directRatio))
    }
    const snapshot = messageSnapshot.value as (CanvasImageItem & { height?: unknown; width?: unknown }) | null
    const sourceHeight = Number(snapshot?.height)
    const sourceWidth = Number(snapshot?.width)
    const fallbackRatio = Number.isFinite(sourceHeight) && sourceHeight > 0 && Number.isFinite(sourceWidth) && sourceWidth > 0
      ? sourceHeight / sourceWidth
      : 1
    return Math.max(72, Math.round(width * fallbackRatio))
  })

  const messageStyleStrengthMenuStyle = computed(() => {
    messageImageLayoutVersion.value
    const imageEl = messageImageElement.value
    if (!imageEl) {
      return { top: '8px', left: 'calc(100% + 16px)' }
    }
    return {
      top: `${Math.max(0, imageEl.offsetTop)}px`,
      left: `${Math.max(0, imageEl.offsetLeft + imageEl.clientWidth + STYLE_STRENGTH_MENU_GAP)}px`,
    }
  })

  const isMessagePending = computed(() => messageSnapshot.value?.renderState === 'pending')

  function setMessageImageRef(element: Element | ComponentPublicInstance | null) {
    messageImageElement.value = element instanceof HTMLImageElement ? element : null
    messageImageLayoutVersion.value += 1
  }

  function onMessageImageLoad() {
    messageImageLayoutVersion.value += 1
  }

  return {
    messageImageElement,
    messageImageLayoutVersion,
    messageDisplayWidth,
    messageDisplayHeight,
    messageStyleStrengthMenuStyle,
    isMessagePending,
    setMessageImageRef,
    onMessageImageLoad,
  }
}
