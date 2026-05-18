<template>
  <div class="image-node-output-count-wrap node-tool-output-count-wrap">
    <button
      v-if="!isEditing"
      class="image-node-output-count node-tool-output-count node-tool-button"
      :class="{ active: visible, 'node-tool-tooltip-trigger': !!tooltip }"
      type="button"
      :disabled="disabled"
      :title="title"
      :aria-label="ariaLabel"
      @click.stop="emit('toggle')"
      @dblclick.stop.prevent="onDoubleClick"
    >
      {{ modelValue }}
      <span v-if="tooltip" class="node-tool-tooltip">{{ tooltip }}</span>
    </button>
    <input
      v-else
      ref="inputRef"
      class="image-node-output-count image-node-output-count-input node-tool-output-count node-tool-button"
      type="number"
      inputmode="numeric"
      :value="draftValue"
      :min="min"
      :max="max"
      step="1"
      :disabled="disabled"
      :aria-label="editAriaLabel"
      @input="onInput"
      @blur="finishEditing"
      @keydown.enter.stop.prevent="finishEditing"
    />
    <NodeToolPopover :visible="visible && !isEditing">
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
import { nextTick, ref, watch } from 'vue'

import { IMAGE_OUTPUT_COUNT_PRESET_OPTIONS } from '../../types/canvasNodes'
import NodeToolPopover from './NodeToolPopover.vue'

const props = withDefaults(
  defineProps<{
    modelValue: number
    visible: boolean
    disabled?: boolean
    customEditable?: boolean
    title?: string
    ariaLabel?: string
    tooltip?: string
    min?: number
    max?: number
    editAriaLabel?: string
  }>(),
  {
    disabled: false,
    customEditable: false,
    title: '选择张数',
    ariaLabel: '输出张数',
    tooltip: '',
    min: 1,
    max: 8,
    editAriaLabel: '自定义输出张数',
  },
)

const emit = defineEmits<{
  (e: 'toggle'): void
  (e: 'select', value: number): void
  (e: 'custom-commit', value: number): void
  (e: 'edit-start'): void
}>()

const outputCountPresetOptions = IMAGE_OUTPUT_COUNT_PRESET_OPTIONS
const isEditing = ref(false)
const draftValue = ref(String(props.modelValue))
const inputRef = ref<HTMLInputElement | null>(null)

watch(
  () => props.modelValue,
  (value) => {
    if (!isEditing.value) draftValue.value = String(value)
  },
  { immediate: true },
)

watch(
  () => props.visible,
  (visible) => {
    if (!visible) isEditing.value = false
  },
)

function onDoubleClick() {
  if (!props.customEditable || props.disabled) return
  emit('edit-start')
  draftValue.value = String(props.modelValue)
  isEditing.value = true
  void nextTick(() => {
    inputRef.value?.focus()
    inputRef.value?.select()
  })
}

function onInput(event: Event) {
  draftValue.value = (event.target as HTMLInputElement).value
}

function finishEditing() {
  if (!isEditing.value) return
  const parsed = Number(draftValue.value)
  const nextValue = Number.isFinite(parsed) ? parsed : props.modelValue
  draftValue.value = String(nextValue)
  emit('custom-commit', nextValue)
  isEditing.value = false
}
</script>

<style scoped src="./styles/node-tool-buttons.css"></style>
