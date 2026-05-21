<template>
  <div class="node-aspect-ratio-wrap node-tool-output-count-wrap">
    <button
      class="node-aspect-ratio-trigger node-tool-output-count node-tool-button node-tool-tooltip-trigger"
      type="button"
      :class="{ active: visible }"
      :disabled="disabled"
      :title="title"
      :aria-label="ariaLabel"
      @click.stop="emit('toggle')"
    >
      <span class="node-aspect-ratio-icon" :data-ratio="ratio">
        <span aria-hidden="true"></span>
      </span>
      <span class="node-tool-tooltip">
        {{ ratio === 'auto' ? 'Auto' : ratio }}
      </span>
    </button>
    <NodeToolPopover :visible="visible" layout="row" class="node-aspect-ratio-popover">
      <button
        v-for="option in PROMPT_ASPECT_RATIO_OPTIONS"
        :key="option"
        type="button"
        class="node-aspect-ratio-option node-tool-tooltip-trigger"
        :class="{ active: option === ratio }"
        :disabled="disabled"
        @click.stop="emit('select', option)"
      >
        <span class="node-aspect-ratio-icon" :data-ratio="option">
          <span aria-hidden="true"></span>
        </span>
        <span class="node-tool-tooltip">
          {{ option === 'auto' ? 'Auto' : option }}
        </span>
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
    title?: string
    ariaLabel?: string
  }>(),
  {
    disabled: false,
    title: '选择尺寸比例',
    ariaLabel: '尺寸比例',
  },
)

const emit = defineEmits<{
  (e: 'toggle'): void
  (e: 'select', value: PromptAspectRatio): void
}>()

const ratio = computed(() => normalizePromptAspectRatio(props.modelValue))
</script>

<style scoped src="./styles/node-tool-buttons.css"></style>
<style scoped>
.node-aspect-ratio-trigger {
  flex: 0 0 var(--node-tool-button-size);
  position: relative;
}

.node-aspect-ratio-icon {
  width: 100%;
  height: 100%;
  display: grid;
  place-items: center;
}

.node-aspect-ratio-icon > span {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border: 1px solid #cbd5e1;
  border-radius: 4px;
  color: #94a3b8;
  transition:
    border-color 0.16s ease,
    color 0.16s ease;
}

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

.node-aspect-ratio-option {
  position: relative;
  width: var(--node-tool-button-size);
  height: var(--node-tool-popover-option-height);
  border: 0;
  border-radius: 0;
  background: transparent;
  color: #5f6878;
  cursor: pointer;
  padding: 0;
  transition:
    background-color 0.16s ease,
    border-color 0.16s ease,
    color 0.16s ease;
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
