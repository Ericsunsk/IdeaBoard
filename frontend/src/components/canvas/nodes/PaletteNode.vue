<template>
  <div
    class="workflow-node palette-node"
    data-node-kind="palette"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="Palette"
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1">
          <circle cx="12" cy="12" r="3.2" />
          <path d="M12 3.5a8.5 8.5 0 1 0 0 17c1.2 0 2.1-.8 2.1-1.8 0-.5-.2-.9-.2-1.4 0-1 .8-1.8 1.8-1.8h.8a3.8 3.8 0 0 0 0-7.6z" />
        </svg>
      </template>
    </NodeHeader>

    <div class="palette-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div class="palette-mode-card node-mode-card">
        <div class="palette-mode-row node-option-row">
          <NodeActionButton
            v-for="option in paletteModeOptions"
            :key="option.value"
            variant="chip"
            :active="node.mode === option.value"
            @click="emit('update:mode', option.value)"
          >
            {{ option.label }}
          </NodeActionButton>
        </div>

        <div v-if="presetPalettes.length > 0" class="palette-preset-grid">
          <button
            v-for="(palette, index) in presetPalettes"
            :key="`${node.mode}-${index}`"
            type="button"
            class="palette-preset-card"
            @click="emit('apply-preset', palette)"
          >
            <NodeColorSwatch
              v-for="(color, colorIndex) in palette"
              :key="`${color}-${colorIndex}`"
              :color="color"
              :size="10"
              borderless
            />
          </button>
        </div>

        <div class="palette-custom-row">
          <div ref="paletteColorFieldRef" class="palette-color-field-wrap">
            <NodeColorField
              class="palette-color-field"
              :value="customColor"
              swatch-aria-label="选择调色板颜色"
              @swatch-click="togglePopover"
              @update:value="onCustomColorInput"
              @change:value="onCustomColorCommit"
            />
            <NodeColorPopover
              v-if="isPopoverOpen"
              :hex="popoverHex"
              :hue="popoverHue"
              :saturation="popoverSaturation"
              :lightness="popoverLightness"
              :spectrum-style="spectrumStyle"
              :cursor-style="cursorStyle"
              :quick-swatches="quickSwatches"
              @hex-input="onPopoverHexInput"
              @hex-commit="commitPopoverHex"
              @hue-input="onPopoverHueInput"
              @saturation-input="onPopoverSaturationInput"
              @lightness-input="onPopoverLightnessInput"
              @spectrum-pointerdown="onSpectrumPointerDown"
              @select-swatch="setPaletteColorHex"
              @set-spectrum-ref="picker.setSpectrumRef"
            />
          </div>
          <div class="palette-action-buttons">
            <NodeIconButton
              tone="accent"
              :size="28"
              title="添加颜色"
              @click="addCustomColor"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round">
                <path d="M12 5v14" />
                <path d="M5 12h14" />
              </svg>
            </NodeIconButton>
            <NodeIconButton
              tone="accent"
              :size="28"
              :disabled="!eyedropperSupported"
              title="吸管取色"
              @click="onEyedropperClick"
            >
              <svg class="palette-eyedropper-icon" viewBox="0 0 242.63 240.1" fill="none" aria-hidden="true">
                <rect x="94.33" y="57.68" width="124.37" height="55.2" transform="translate(205.65 256.62) rotate(-134.59)" fill="currentColor" />
                <rect x="67.51" y="38.09" width="55.26" height="215.36" rx="27.63" ry="27.63" transform="translate(58.11 315.87) rotate(-134.59)" stroke="currentColor" stroke-width="21" />
                <rect x="162.01" y="-10.5" width="55.53" height="125.99" rx="27.77" ry="27.77" transform="translate(285.61 224.51) rotate(-134.59)" fill="currentColor" />
              </svg>
            </NodeIconButton>
          </div>
        </div>

        <div class="palette-selected-row">
          <button
            v-for="(color, index) in node.colors"
            :key="`${color}-${index}`"
            type="button"
            class="palette-selected-chip"
            :style="{ '--palette-color': color }"
            @click="emit('remove-color', index)"
          >
            <NodeColorSwatch class="palette-selected-swatch" :color="color" :size="12" borderless />
          </button>
        </div>
      </div>

      <NodeConnectionChips :items="connectionChips" />

      <div class="node-primary-action-slot">
        <NodeActionButton variant="primary" :disabled="generating" full @click="emit('generate')">
          生成
        </NodeActionButton>
      </div>
    </div>

    <NodePorts
      node-kind="palette"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import type { PaletteNodeItem } from '../../../types/canvasNodes'
import { useColorPickerState } from '../../../composables/useColorPickerState'
import NodeActionButton from './NodeActionButton.vue'
import NodeConnectionChips from './NodeConnectionChips.vue'
import NodeColorField from './NodeColorField.vue'
import NodeColorPopover from './NodeColorPopover.vue'
import NodeColorSwatch from './NodeColorSwatch.vue'
import NodeHeader from './NodeHeader.vue'
import NodeIconButton from './NodeIconButton.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, PALETTE_NODE_IO_SLOTS } from './nodeIoConfig'
import { PALETTE_MODE_OPTIONS, PALETTE_PRESET_MAP } from './nodeOptionConfig'
import {
  normalizeHexColor,
} from './colorPopoverUtils'
import { isEyeDropperSupported } from '../../../utils/eyedropper'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: PaletteNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  generating: boolean
  imageInputConnected?: boolean
  referenceInputConnected?: boolean
  outputConnected?: boolean
  errorPortKeys: string[]
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'update:mode', value: PaletteNodeItem['mode']): void
  (e: 'apply-preset', value: string[]): void
  (e: 'remove-color', index: number): void
  (e: 'add-color', value: string): void
  (e: 'generate'): void
  (e: 'port-mousedown', event: MouseEvent, port: 'image'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const paletteModeOptions = PALETTE_MODE_OPTIONS

const customColor = ref('#A3E635')
const isPopoverOpen = ref(false)
const paletteColorFieldRef = ref<HTMLElement | null>(null)
const usesReference = computed(() => props.node.mode === 'reference')
const eyedropperSupported = isEyeDropperSupported()
const presetPalettes = computed(() => {
  if (props.node.mode === 'reference') return []
  return PALETTE_PRESET_MAP[props.node.mode]
})
const io = computed(() =>
  buildNodeIoFromSlots(PALETTE_NODE_IO_SLOTS, { usesReference: usesReference.value }, {
    connected: {
      image: props.imageInputConnected,
      reference: props.referenceInputConnected,
      output: props.outputConnected,
    },
    errorPortKeys: props.errorPortKeys,
  }),
)
const connectionChips = computed(() => io.value.connectionChips)
const ports = computed(() => io.value.ports)
const picker = useColorPickerState({
  initialHex: customColor.value,
  syncHslOnHexInput: true,
  onColorChange(hex) {
    customColor.value = hex
  },
})
const popoverHex = picker.hex
const popoverHue = picker.hue
const popoverSaturation = picker.saturation
const popoverLightness = picker.lightness
const quickSwatches = picker.quickSwatches
const spectrumStyle = picker.spectrumStyle
const cursorStyle = picker.cursorStyle

function addCustomColor() {
  emit('add-color', normalizeHexColor(customColor.value))
}

async function onEyedropperClick() {
  try {
    const pickedHex = await picker.pickEyeDropperColor(customColor.value)
    customColor.value = picker.syncFromHex(pickedHex, false)
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') return
  }
}

function setPaletteColorHex(value: string) {
  customColor.value = picker.syncFromHex(value, false)
}

function onPortMouseDown(event: MouseEvent, port: string) {
  emit('port-mousedown', event, port as 'image')
}

function togglePopover() {
  if (isPopoverOpen.value) {
    closePopover()
    return
  }
  setPaletteColorHex(customColor.value)
  isPopoverOpen.value = true
}

function closePopover() {
  isPopoverOpen.value = false
}

function onCustomColorInput(value: string) {
  customColor.value = value
}

function onCustomColorCommit(value: string) {
  customColor.value = picker.syncFromHex(value, false)
}

function onPopoverHexInput(event: Event) {
  picker.handleHexInput((event.target as HTMLInputElement | null)?.value ?? '')
}

function commitPopoverHex() {
  picker.commitHex(customColor.value)
}

function onPopoverHueInput(value: number) {
  picker.setHue(value)
}

function onPopoverSaturationInput(value: number) {
  picker.setSaturation(value)
}

function onPopoverLightnessInput(value: number) {
  picker.setLightness(value)
}

function onSpectrumPointerDown(event: PointerEvent) {
  picker.onSpectrumPointerDown(event)
}

function handleDocumentPointerDown(event: MouseEvent) {
  if (!isPopoverOpen.value) return
  const fieldEl = paletteColorFieldRef.value
  if (fieldEl && fieldEl.contains(event.target as Node)) return
  closePopover()
}

onMounted(() => {
  document.addEventListener('mousedown', handleDocumentPointerDown)
})

onUnmounted(() => {
  document.removeEventListener('mousedown', handleDocumentPointerDown)
})
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.palette-node-body {
  gap: 8px;
}

.palette-preset-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px;
}

.palette-preset-card {
  border: 1px solid #e2e8f0;
  background: #fff;
  border-radius: 12px;
  padding: 6px 8px;
  display: flex;
  align-items: center;
  gap: 5px;
  cursor: pointer;
}

.palette-custom-row {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
}

.palette-action-buttons {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.palette-color-field-wrap {
  position: relative;
  flex: 1;
  min-width: 0;
}

.palette-color-field {
  width: 100%;
}

.palette-eyedropper-icon {
  width: 14px;
  height: 14px;
  display: block;
  object-fit: contain;
}

.palette-selected-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  min-height: 28px;
}

.palette-selected-chip {
  width: 24px;
  height: 24px;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--palette-color) 22%, #d7deea);
  background: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}

</style>
