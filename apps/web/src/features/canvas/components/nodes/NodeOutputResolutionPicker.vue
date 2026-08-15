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
      {{ displayValue }}
      <span v-if="resolvedTooltip" class="node-tool-tooltip" aria-hidden="true">
        {{ resolvedTooltip }}
      </span>
    </button>
    <NodeToolPopover :visible="visible">
      <button
        v-for="option in OUTPUT_RESOLUTION_OPTIONS"
        :key="option"
        type="button"
        class="image-node-output-count-option node-tool-output-count-option"
        :class="{ active: option === modelValue }"
        :disabled="disabled"
        role="menuitemradio"
        :aria-checked="option === modelValue"
        @click.stop="emit('select', option)"
      >
        {{ option.toUpperCase() }}
      </button>
    </NodeToolPopover>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { OutputResolution } from '../../../../shared/domain/generationContract'
import { OUTPUT_RESOLUTION_OPTIONS } from '../../../../shared/domain/generationContract'
import NodeToolPopover from './NodeToolPopover.vue'

const props = withDefaults(
  defineProps<{
    modelValue: OutputResolution
    visible: boolean
    disabled?: boolean
    ariaLabel?: string
    tooltip?: string
  }>(),
  {
    ariaLabel: '输出清晰度',
    disabled: false,
    tooltip: '选择清晰度',
  },
)

const emit = defineEmits<{
  (event: 'toggle'): void
  (event: 'select', value: OutputResolution): void
}>()

const displayValue = computed(() => props.modelValue.toUpperCase())
const resolvedTooltip = computed(() => props.tooltip || '选择清晰度')
</script>
