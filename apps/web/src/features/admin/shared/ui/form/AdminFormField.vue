<template>
  <label :for="props.for" :class="fieldClass">
    <span :class="labelTextClass">
      {{ label }}
      <span v-if="required" class="text-destructive" aria-hidden="true">*</span>
    </span>

    <slot v-bind="slotProps" />

    <p
      v-if="hasError"
      :id="errorElementId"
      class="m-0 text-admin-caption font-medium text-destructive"
    >
      {{ error }}
    </p>
    <p v-else-if="hint" :id="hintElementId" class="m-0 text-admin-caption text-muted-foreground">
      {{ hint }}
    </p>
  </label>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'
import {
  resolveFormFieldState,
  type FormFieldContractProps,
  type FormFieldContractSlotProps,
} from '@/core/components/formFieldContract'

const props = withDefaults(
  defineProps<
    Omit<FormFieldContractProps, 'fieldId'> & {
      for?: string | undefined
      labelClass?: string | undefined
      bodyClass?: string | undefined
    }
  >(),
  {
    for: undefined,
    error: '',
    hint: '',
    required: false,
    labelClass: '',
    bodyClass: '',
  },
)

defineSlots<{
  default(props: FormFieldContractSlotProps): unknown
}>()

const generatedId = useId()
// Id/a11y state comes from the neutral contract: same `describedbyId`/`invalid`
// derivation as the core adapter, Admin only supplies markup and tokens.
const state = computed(() =>
  resolveFormFieldState({
    fieldId: props.for,
    fallbackId: `admin-form-field-${generatedId}`,
    error: props.error,
    hint: props.hint,
  }),
)
const hasError = computed(() => state.value.invalid)
const hintElementId = computed(() => state.value.hintId)
const errorElementId = computed(() => state.value.errorId)
const fieldClass = computed(() =>
  props.bodyClass ? ['flex min-w-0 flex-col', props.bodyClass] : 'flex min-w-0 flex-col gap-1.5',
)
const labelTextClass = computed(
  () => props.labelClass || 'text-admin-meta font-admin-emphasis text-muted-foreground',
)
const slotProps = computed<FormFieldContractSlotProps>(() => ({
  ...state.value,
  required: props.required,
}))
</script>
