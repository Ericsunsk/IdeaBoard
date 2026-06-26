<template>
  <div
    class="node-aspect-ratio-wrap relative h-[var(--node-tool-button-size)] w-[var(--node-tool-button-size)]"
  >
    <button
      class="node-aspect-ratio-trigger node-tool-tooltip-trigger inline-flex h-[var(--node-tool-button-size)] w-[var(--node-tool-button-size)] flex-[0_0_var(--node-tool-button-size)] cursor-pointer items-center justify-center rounded-[var(--node-tool-button-radius)] border-[length:var(--node-tool-border-width)] border-solid border-[color:var(--node-tool-button-border)] bg-white p-0 text-xs leading-none font-bold text-[#505766] outline-none disabled:cursor-wait disabled:opacity-[0.55]"
      type="button"
      :class="{ active: visible }"
      :disabled="disabled"
      :aria-label="ariaLabel"
      @click.stop="emit('toggle')"
    >
      <span
        class="node-aspect-ratio-icon grid h-full w-full place-items-center"
        :data-ratio="ratio"
      >
        <span
          class="grid h-5 w-5 place-items-center rounded border border-[#cbd5e1] text-[#94a3b8]"
          aria-hidden="true"
        ></span>
      </span>
      <span class="node-tool-tooltip" aria-hidden="true">{{ resolvedTooltip }}</span>
    </button>
    <NodeToolPopover :visible="visible" layout="row" class="node-aspect-ratio-popover">
      <button
        v-for="option in PROMPT_ASPECT_RATIO_OPTIONS"
        :key="option"
        type="button"
        class="node-aspect-ratio-option node-tool-tooltip-trigger relative h-[var(--node-tool-popover-option-height)] w-[var(--node-tool-button-size)] cursor-pointer rounded-none border-0 bg-transparent p-0 text-[#5f6878]"
        :class="{ active: option === ratio }"
        :disabled="disabled"
        :aria-label="option"
        @click.stop="emit('select', option)"
      >
        <span
          class="node-aspect-ratio-icon grid h-full w-full place-items-center"
          :data-ratio="option"
        >
          <span
            class="grid h-5 w-5 place-items-center rounded border border-[#cbd5e1] text-[#94a3b8]"
            aria-hidden="true"
          ></span>
        </span>
        <span class="node-tool-tooltip" aria-hidden="true">{{ option }}</span>
      </button>
    </NodeToolPopover>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  PROMPT_ASPECT_RATIO_OPTIONS,
  normalizePromptAspectRatio,
  type PromptAspectRatio,
} from '../../config/promptAspectRatio'
import NodeToolPopover from './NodeToolPopover.vue'

const props = withDefaults(
  defineProps<{
    modelValue: PromptAspectRatio
    visible: boolean
    disabled?: boolean
    ariaLabel?: string
    tooltip?: string
  }>(),
  {
    disabled: false,
    ariaLabel: '尺寸比例',
    tooltip: undefined,
  },
)

const emit = defineEmits<{
  (e: 'toggle'): void
  (e: 'select', value: PromptAspectRatio): void
}>()

const ratio = computed(() => normalizePromptAspectRatio(props.modelValue))
const resolvedTooltip = computed(() => props.tooltip || '选择尺寸比例')
</script>

<style scoped src="./styles/node-tool-buttons.css"></style>
<style scoped>
.node-aspect-ratio-icon[data-ratio='3:4'] > span {
  width: 17px;
  height: 22px;
}

.node-aspect-ratio-icon[data-ratio='9:16'] > span {
  width: 13px;
  height: 22px;
}

.node-aspect-ratio-icon[data-ratio='4:3'] > span {
  width: 22px;
  height: 17px;
}

.node-aspect-ratio-icon[data-ratio='16:9'] > span {
  width: 22px;
  height: 13px;
}

.node-aspect-ratio-popover {
  right: 0;
}

.node-aspect-ratio-trigger,
.node-aspect-ratio-option,
.node-aspect-ratio-icon > span {
  transition:
    background-color var(--workspace-motion-fast) ease,
    border-color var(--workspace-motion-fast) ease,
    color var(--workspace-motion-fast) ease;
}

.node-aspect-ratio-trigger:hover:not(:disabled),
.node-aspect-ratio-trigger:focus,
.node-aspect-ratio-trigger.active {
  background: #f8f7ff;
  border-color: var(--node-tool-button-accent);
  color: #272932;
}

.node-aspect-ratio-option:hover,
.node-aspect-ratio-option.active {
  color: var(--node-tool-button-accent);
}

.node-aspect-ratio-option:hover .node-aspect-ratio-icon > span,
.node-aspect-ratio-option.active .node-aspect-ratio-icon > span,
.node-aspect-ratio-trigger:hover .node-aspect-ratio-icon > span,
.node-aspect-ratio-trigger.active .node-aspect-ratio-icon > span {
  border-color: var(--node-tool-button-accent);
  color: var(--node-tool-button-accent);
}
</style>
