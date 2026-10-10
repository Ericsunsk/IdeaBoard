<template>
  <div class="editing-controls-fields retouch-levels-fields">
    <label
      v-for="field in levelFields"
      :key="field.key"
      class="editing-controls-field editing-controls-field--compact retouch-levels-fields__field"
    >
      <NumberField
        :name="`retouch-level-${field.key}`"
        :min="constraints[field.key].min"
        :max="constraints[field.key].max"
        input-mode="numeric"
        :aria-label="field.ariaLabel"
        :aria-valuemin="constraints[field.key].min"
        :aria-valuemax="constraints[field.key].max"
        :value="params[field.key]"
        @update:value="onLevelValue(field.key, $event)"
      />
    </label>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import NumberField from '../shared/NumberField.vue'
import type { RetouchLevelsParams } from '../../core/model.ts'
import {
  moveLevelsHandle,
  type RetouchLevelsConstraint,
  type RetouchLevelsInputHandleId,
} from '../../core/kinds/levels.ts'

const props = defineProps<{
  params: RetouchLevelsParams
  constraints: Record<RetouchLevelsInputHandleId, RetouchLevelsConstraint>
}>()

const emit = defineEmits<{
  (e: 'update:params', value: RetouchLevelsParams): void
}>()

const { t } = useI18n()

const LEVEL_FIELD_LABEL_KEYS: Record<RetouchLevelsInputHandleId, string> = {
  shadows: 'canvas.retouch.params.shadows',
  midtones: 'canvas.retouch.params.midtones',
  highlights: 'canvas.retouch.params.highlights',
}

// Field labels carry only labelKeys; t() runs inside this computed so locale
// switches re-translate (same precedent as definitions ranges / S8 rail).
const levelFields = computed(() =>
  (Object.entries(LEVEL_FIELD_LABEL_KEYS) as Array<[RetouchLevelsInputHandleId, string]>).map(
    ([key, labelKey]) => ({
      key,
      ariaLabel: t(labelKey),
    }),
  ),
)

function onLevelValue(key: RetouchLevelsInputHandleId, value: number) {
  emit('update:params', moveLevelsHandle(props.params, key, value))
}
</script>
