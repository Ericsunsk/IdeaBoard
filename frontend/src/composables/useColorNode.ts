import { ref, type Ref } from 'vue'
import type { ColorNodeItem } from '../types/canvasNodes'
import { normalizeHexColor } from '../components/canvas/nodes/colorPopoverUtils'
import { useColorPickerState } from './useColorPickerState'
import { isEyeDropperSupported } from '../utils/eyedropper'

interface ColorNodeMessageApi {
  warning(content: string): unknown
  error(content: string): unknown
}

interface UseColorNodeOptions {
  colorNodes: Ref<ColorNodeItem[]>
  message: ColorNodeMessageApi
}

export function useColorNode(options: UseColorNodeOptions) {
  const activeColorPopoverNodeId = ref<number | null>(null)
  const picker = useColorPickerState({
    initialHex: '#A3E635',
    syncHslOnHexInput: false,
    onColorChange(hex) {
      const nodeId = activeColorPopoverNodeId.value
      if (nodeId === null) return
      const node = options.colorNodes.value.find(item => item.id === nodeId)
      if (node) node.targetColor = hex
    },
  })

  function syncPickerFromNodeColor(value: string) {
    picker.syncFromHex(value, false)
  }

  function applyColorPopoverToNode() {
    const nodeId = activeColorPopoverNodeId.value
    if (nodeId === null) return
    const node = options.colorNodes.value.find(item => item.id === nodeId)
    if (!node) return
    node.targetColor = picker.commitHex(node.targetColor)
  }

  function setColorPopoverHex(hex: string) {
    picker.syncFromHex(hex, true)
  }

  function openColorPopover(node: ColorNodeItem) {
    activeColorPopoverNodeId.value = node.id
    syncPickerFromNodeColor(node.targetColor)
  }

  function closeColorPopover() {
    activeColorPopoverNodeId.value = null
  }

  function toggleColorPopover(node: ColorNodeItem) {
    if (activeColorPopoverNodeId.value === node.id) {
      closeColorPopover()
      return
    }
    openColorPopover(node)
  }

  function onColorPopoverHexInput(event: Event) {
    picker.handleHexInput((event.target as HTMLInputElement | null)?.value ?? '')
  }

  function commitColorPopoverHex() {
    const nodeId = activeColorPopoverNodeId.value
    if (nodeId !== null) {
      const node = options.colorNodes.value.find(item => item.id === nodeId)
      if (node) {
        picker.commitHex(node.targetColor)
        return
      }
    }
    picker.commitHex('#A3E635')
  }

  function onColorNodeTargetColorChange(node: ColorNodeItem, eventOrValue: Event | string) {
    const input = typeof eventOrValue === 'string'
      ? eventOrValue
      : (eventOrValue.target as HTMLInputElement | null)?.value ?? ''
    node.targetColor = normalizeHexColor(input || node.targetColor)
    if (activeColorPopoverNodeId.value === node.id) {
      syncPickerFromNodeColor(node.targetColor)
    }
  }

  async function onColorNodeEyedropperClick(node: ColorNodeItem) {
    if (!isEyeDropperSupported()) {
      options.message.warning('当前浏览器不支持吸管取色')
      return
    }
    try {
      const pickedHex = await picker.pickEyeDropperColor(node.targetColor)
      node.targetColor = pickedHex
      if (activeColorPopoverNodeId.value === node.id) {
        syncPickerFromNodeColor(pickedHex)
      }
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') return
      options.message.error('吸管取色失败，请重试')
    }
  }

  return {
    activeColorPopoverNodeId,
    colorPopoverHex: picker.hex,
    colorPopoverHue: picker.hue,
    colorPopoverSaturation: picker.saturation,
    colorPopoverLightness: picker.lightness,
    quickSwatches: picker.quickSwatches,
    setColorSpectrumRef: picker.setSpectrumRef,
    colorSpectrumStyle: picker.spectrumStyle,
    colorSpectrumCursorStyle: picker.cursorStyle,
    normalizeHexColor,
    setColorPopoverHex,
    applyColorPopoverToNode,
    onColorSpectrumPointerDown: picker.onSpectrumPointerDown,
    closeColorPopover,
    toggleColorPopover,
    onColorPopoverHexInput,
    commitColorPopoverHex,
    onColorNodeTargetColorChange,
    onColorNodeEyedropperClick,
  }
}
