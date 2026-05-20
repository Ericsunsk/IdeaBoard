<template>
  <div class="image-node-output-count-wrap node-tool-output-count-wrap">
    <button
      class="image-node-output-count node-tool-output-count node-tool-button"
      :class="{ active: visible, 'node-tool-tooltip-trigger': !!tooltip }"
      type="button"
      :disabled="disabled"
      :title="title"
      :aria-label="ariaLabel"
      @click.stop="emit('toggle')"
    >
      {{ modelValue }}
      <span v-if="tooltip" class="node-tool-tooltip">{{ tooltip }}</span>
    </button>
    <NodeToolPopover :visible="visible">
      <button
        v-for="option in outputCountPresetOptions"
        :key="option"
        type="button"
        class="image-node-output-count-option node-tool-output-count-option"
        :class="{ active: option === modelValue }"
        :disabled="disabled"
        @click.stop="emit('select', option)"
      >
        {{ option }}
      </button>
    </NodeToolPopover>
  </div>
</template>

<script setup lang="ts">
import { IMAGE_OUTPUT_COUNT_PRESET_OPTIONS } from '../../types/canvasNodes'
import NodeToolPopover from './NodeToolPopover.vue'

withDefaults(
  defineProps<{
    modelValue: number
    visible: boolean
    disabled?: boolean
    title?: string
    ariaLabel?: string
    tooltip?: string
  }>(),
  {
    disabled: false,
    title: '选择张数',
    ariaLabel: '输出张数',
    tooltip: '',
  },
)

const emit = defineEmits<{
  (e: 'toggle'): void
  (e: 'select', value: number): void
}>()

const outputCountPresetOptions = IMAGE_OUTPUT_COUNT_PRESET_OPTIONS
</script>

<style scoped src="./styles/node-tool-buttons.css"></style>
