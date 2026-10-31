<template>
  <div class="image-node-output-count-wrap node-tool-output-count-wrap">
    <button
      class="image-node-output-count node-tool-output-count node-tool-button"
      :class="{ active: isVisible, 'canvas-tool-tooltip-trigger': !!tooltip }"
      type="button"
      :disabled="isDisabled"
      :aria-label="label"
      @click.stop="emit('toggle')"
    >
      {{ format(modelValue) }}
      <span v-if="tooltip" class="canvas-tool-tooltip" aria-hidden="true">
        {{ tooltip }}
      </span>
    </button>
    <NodeToolPopover :is-visible="isVisible">
      <button
        v-for="option in options"
        :key="option"
        type="button"
        class="image-node-output-count-option node-tool-output-count-option"
        :class="{ active: option === modelValue }"
        :disabled="isDisabled"
        :role="optionRole"
        :aria-checked="optionRole === 'menuitemradio' ? option === modelValue : undefined"
        @click.stop="emit('select', option)"
      >
        {{ format(option) }}
      </button>
    </NodeToolPopover>
  </div>
</template>

<script setup lang="ts" generic="T extends string | number">
import NodeToolPopover from '../tools/NodeToolPopover.vue'

withDefaults(
  defineProps<{
    modelValue: T
    options: readonly T[]
    isVisible: boolean
    label: string
    tooltip: string
    isDisabled?: boolean
    format?: (value: T) => string
    optionRole?: 'menuitemradio' | undefined
  }>(),
  {
    isDisabled: false,
    format: (value: T) => String(value),
    optionRole: undefined,
  },
)

const emit = defineEmits<{
  (event: 'toggle'): void
  (event: 'select', value: T): void
}>()
</script>
