<template>
  <div class="editing-controls-fields retouch-levels-fields">
    <label
      v-for="field in levelFields"
      :key="field.key"
      class="editing-controls-field editing-controls-field--compact retouch-levels-fields__field"
    >
      <input
        :name="`retouch-level-${field.key}`"
        class="editing-controls-control editing-controls-number-input"
        type="number"
        :min="constraints[field.key].min"
        :max="constraints[field.key].max"
        step="1"
        inputmode="numeric"
        :aria-label="field.ariaLabel"
        :aria-valuemin="constraints[field.key].min"
        :aria-valuemax="constraints[field.key].max"
        :value="params[field.key]"
        @input="onLevelInput(field.key, $event)"
      />
    </label>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { RetouchLevelsParams } from '../types'
import {
  moveLevelsHandle,
  type RetouchLevelsConstraint,
  type RetouchLevelsInputHandleId,
} from '../utils/levels'
import { readFiniteNumberInputValue } from '../utils/readFiniteNumberInputValue'

const props = defineProps<{
  params: RetouchLevelsParams
  constraints: Record<RetouchLevelsInputHandleId, RetouchLevelsConstraint>
}>()

const emit = defineEmits<{
  (e: 'update:params', value: RetouchLevelsParams): void
}>()

const { t } = useI18n()

const levelFields = [
  { key: 'shadows', ariaLabel: t('canvas.retouch.params.shadows') },
  { key: 'midtones', ariaLabel: t('canvas.retouch.params.midtones') },
  { key: 'highlights', ariaLabel: t('canvas.retouch.params.highlights') },
] satisfies Array<{
  key: RetouchLevelsInputHandleId
  ariaLabel: string
}>

function onLevelInput(key: RetouchLevelsInputHandleId, event: Event) {
  const value = readFiniteNumberInputValue(event)
  if (value == null) return
  emit('update:params', moveLevelsHandle(props.params, key, value))
}
</script>
