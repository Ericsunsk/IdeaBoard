<template>
  <div class="image-node-output-count-wrap node-tool-output-count-wrap">
    <button
      class="image-node-output-count node-tool-output-count node-tool-button"
      :class="{ active: visible, 'node-tool-tooltip-trigger': !!resolvedTooltip }"
      type="button"
      :disabled="disabled"
      :aria-label="ariaLabel"
      @click.stop="emit('toggle')"
    >
      {{ modelValue }}
      <span v-if="resolvedTooltip" class="node-tool-tooltip" aria-hidden="true">
        {{ resolvedTooltip }}
      </span>
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
import { computed } from 'vue'
import { IMAGE_OUTPUT_COUNT_PRESET_OPTIONS } from '../../../canvas-board/domain'
import NodeToolPopover from './NodeToolPopover.vue'

const props = withDefaults(
  defineProps<{
    modelValue: number
    visible: boolean
    disabled?: boolean
    ariaLabel?: string
    tooltip?: string
  }>(),
  {
    disabled: false,
    ariaLabel: '输出张数',
    tooltip: '',
  },
)

const emit = defineEmits<{
  (e: 'toggle'): void
  (e: 'select', value: number): void
}>()

const outputCountPresetOptions = IMAGE_OUTPUT_COUNT_PRESET_OPTIONS
const resolvedTooltip = computed(() => props.tooltip || '选择张数')
</script>

<style scoped src="./styles/node-tool-buttons.css"></style>
