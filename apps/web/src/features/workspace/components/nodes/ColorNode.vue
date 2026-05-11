<template>
  <div
    class="workflow-node color-node"
    data-node-kind="color"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode, running: uiState?.isRunning }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="Color"
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

    <div class="color-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div class="color-mode-card node-mode-card">
        <div class="color-strength-row">
          <input
            :value="node.strength"
            class="color-strength-range node-inline-range"
            :style="{ '--slider-pct': `${node.strength}%` }"
            type="range"
            min="0"
            max="100"
            @input="emit('strength-input', toNumber(($event.target as HTMLInputElement).value, node.strength))"
          />
        </div>

        <div class="color-preset-grid">
          <button
            v-for="(preset, index) in colorPresets"
            :key="`color-preset-${index}`"
            type="button"
            class="color-preset-card"
            @click="applyPreset(preset)"
          >
            <NodeColorSwatch
              v-for="(color, colorIndex) in preset"
              :key="`${color}-${colorIndex}`"
              :color="color"
              :size="10"
              borderless
            />
          </button>
        </div>

        <div class="color-custom-row">
          <div ref="colorFieldRef" class="color-field-wrap">
            <NodeColorField
              class="color-field"
              :value="customColor"
              swatch-aria-label="选择颜色"
              @swatch-click="togglePopover"
              @update:value="onCustomColorInput"
              @change:value="onCustomColorCommit"
            />
            <NodeColorPopover
              v-if="isPopoverOpen"
              :hex="popoverHex"
              :hue="popoverHue"
              :saturation="popoverSaturation"
              :brightness="popoverBrightness"
              :spectrum-style="spectrumStyle"
              :cursor-style="cursorStyle"
              :quick-swatches="quickSwatches"
              @hex-input="onPopoverHexInput"
              @hex-commit="commitPopoverHex"
              @hue-input="onPopoverHueInput"
              @saturation-input="onPopoverSaturationInput"
              @brightness-input="onPopoverBrightnessInput"
              @spectrum-pointerdown="onSpectrumPointerDown"
              @select-swatch="setColorHex"
              @set-spectrum-ref="picker.setSpectrumRef"
            />
          </div>
          <div class="color-action-buttons">
            <NodeIconButton
              tone="accent"
              :size="28"
              title="添加颜色"
              @mousedown.prevent
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
              <svg class="color-eyedropper-icon" viewBox="0 0 242.63 240.1" fill="none" aria-hidden="true">
                <rect x="94.33" y="57.68" width="124.37" height="55.2" transform="translate(205.65 256.62) rotate(-134.59)" fill="currentColor" />
                <rect x="67.51" y="38.09" width="55.26" height="215.36" rx="27.63" ry="27.63" transform="translate(58.11 315.87) rotate(-134.59)" stroke="currentColor" stroke-width="21" />
                <rect x="162.01" y="-10.5" width="55.53" height="125.99" rx="27.77" ry="27.77" transform="translate(285.61 224.51) rotate(-134.59)" fill="currentColor" />
              </svg>
            </NodeIconButton>
          </div>
        </div>

        <div v-if="hasSelectedColors" class="color-selected-row">
          <button
            v-for="(color, index) in node.colors"
            :key="`${color}-${index}`"
            type="button"
            class="color-selected-chip"
            :style="{ '--color-chip': color }"
            @mousedown.prevent
            @click.stop="emit('remove-color', index)"
          >
            <NodeColorSwatch class="color-selected-swatch" :color="color" :size="12" borderless />
          </button>
        </div>
      </div>

      <NodeExecutionFooter
        :items="connectionChips"
        :disabled="uiState?.primaryAction?.disabled"
        :label="uiState?.primaryAction?.label || '应用'"
        @generate="emit('generate')"
      />
    </div>


  </div>
</template>

<script setup lang="ts">
import { MAX_COLOR_NODE_SWATCHES } from '../../types/canvasNodes'
import type { ColorNodeItem } from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'
import { useColorPickerState } from '../../composables/useColorPickerState'

import { buildNodeIoFromSlots, COLOR_NODE_IO_SLOTS } from './nodeIoConfig'
import { DEFAULT_COLOR_HEX, normalizeHexColor } from './colorPopoverUtils'
import { toNumber } from './numberUtils'
import { isEyeDropperSupported } from '../../utils/eyedropper'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: ColorNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  uiState?: NodeUiState
  errorPortKeys: string[]
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'apply-preset', value: string[]): void
  (e: 'remove-color', index: number): void
  (e: 'add-color', value: string): void
  (e: 'strength-input', value: number): void
  (e: 'generate'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const colorPresets = [
  ['#A08878', '#6080A8', '#C86040', '#B84050'],
  ['#987058', '#80B0A0', '#C87840', '#D8B8D0'],
  ['#886888', '#489060', '#C8C090', '#788898'],
  ['#D8A8A0', '#E8E0C8', '#98A868', '#8890A0'],
] as const

const customColor = ref(DEFAULT_COLOR_HEX)
const isPopoverOpen = ref(false)
const colorFieldRef = ref<HTMLElement | null>(null)
const eyedropperSupported = isEyeDropperSupported()
const hasSelectedColors = computed(() => props.node.colors.length > 0)
const io = computed(() =>
  buildNodeIoFromSlots(COLOR_NODE_IO_SLOTS, null, {
    connected: {
      image: props.uiState?.connection?.connected?.image,
      mask: props.uiState?.connection?.connected?.mask,
      prompt: props.uiState?.connection?.connected?.prompt,
      output: props.uiState?.connection?.connected?.output,
    },
    pending: {
      image: props.uiState?.connection?.pending?.image,
      mask: props.uiState?.connection?.pending?.mask,
      prompt: props.uiState?.connection?.pending?.prompt,
    },
    errorPortKeys: props.errorPortKeys,
  }),
)
const connectionChips = computed(() => io.value.connectionChips)
const picker = useColorPickerState({
  initialHex: customColor.value,
  syncHsvOnHexInput: true,
  onColorChange(hex) {
    customColor.value = hex
  },
})
const popoverHex = picker.hex
const popoverHue = picker.hue
const popoverSaturation = picker.saturation
const popoverBrightness = picker.brightness
const quickSwatches = picker.quickSwatches
const spectrumStyle = picker.spectrumStyle
const cursorStyle = picker.cursorStyle

function applyCommittedColor(value: string) {
  const nextColor = picker.syncFromHex(value, false)
  customColor.value = nextColor
}

function addCustomColor() {
  if (props.node.colors.length >= MAX_COLOR_NODE_SWATCHES) return
  emit('add-color', normalizeHexColor(customColor.value))
}

function applyPreset(preset: readonly string[]) {
  emit('apply-preset', [...preset])
}

async function onEyedropperClick() {
  try {
    const pickedHex = await picker.pickEyeDropperColor(customColor.value)
    applyCommittedColor(pickedHex)
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') return
  }
}

function setColorHex(value: string) {
  applyCommittedColor(value)
}


function togglePopover() {
  if (isPopoverOpen.value) {
    closePopover()
    return
  }
  picker.syncFromHex(customColor.value, false)
  isPopoverOpen.value = true
}

function closePopover() {
  isPopoverOpen.value = false
}

function onCustomColorInput(value: string) {
  customColor.value = value
}

function onCustomColorCommit(value: string) {
  applyCommittedColor(value)
}

function onPopoverHexInput(event: Event) {
  picker.handleHexInput((event.target as HTMLInputElement | null)?.value ?? '')
}

function commitPopoverHex() {
  applyCommittedColor(picker.hex.value)
}

function onPopoverHueInput(value: number) {
  picker.setHue(value)
}

function onPopoverSaturationInput(value: number) {
  picker.setSaturation(value)
}

function onPopoverBrightnessInput(value: number) {
  picker.setBrightness(value)
}

function onSpectrumPointerDown(event: PointerEvent) {
  picker.onSpectrumPointerDown(event)
}

function handleDocumentPointerDown(event: MouseEvent) {
  if (!isPopoverOpen.value) return
  const fieldEl = colorFieldRef.value
  if (fieldEl && fieldEl.contains(event.target as Node)) return
  closePopover()
}

watch(
  () => props.node.colors[0],
  (value) => {
    const nextColor = normalizeHexColor(value || customColor.value)
    customColor.value = nextColor
    picker.syncFromHex(nextColor, false)
  },
  { immediate: true },
)

onMounted(() => {
  document.addEventListener('mousedown', handleDocumentPointerDown)
})

onUnmounted(() => {
  document.removeEventListener('mousedown', handleDocumentPointerDown)
})
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.color-node-body {
  gap: 8px;
}

.color-strength-row {
  display: flex;
  align-items: center;
}

.color-strength-range {
  width: 100%;
}

.color-preset-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px;
}

.color-preset-card {
  border: 1px solid #e2e8f0;
  background: #fff;
  border-radius: 12px;
  padding: 6px 8px;
  display: flex;
  align-items: center;
  gap: 5px;
  cursor: pointer;
}

.color-custom-row {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
}

.color-action-buttons {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.color-field-wrap {
  position: relative;
  flex: 1;
  min-width: 0;
}

.color-field {
  width: 100%;
}

.color-eyedropper-icon {
  width: 14px;
  height: 14px;
  display: block;
  object-fit: contain;
}

.color-selected-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  min-height: 28px;
}

.color-selected-chip {
  width: 24px;
  height: 24px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border-radius: 999px;
  background: #fff;
  border: 1px solid color-mix(in srgb, var(--color-chip) 22%, #d7deea);
  cursor: pointer;
}
</style>
