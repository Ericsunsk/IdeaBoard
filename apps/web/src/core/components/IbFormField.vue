<template>
  <div :class="fieldClass">
    <div v-if="$slots['header-extra']" class="flex items-center justify-between">
      <label :for="resolvedFieldId" :class="labelClass">
        {{ label }}
        <span v-if="required" aria-hidden="true">*</span>
      </label>
      <slot name="header-extra" />
    </div>
    <label v-else :for="resolvedFieldId" :class="labelClass">
      {{ label }}
      <span v-if="required" aria-hidden="true">*</span>
    </label>

    <slot v-bind="slotProps" />

    <span v-if="!showMessage" :id="errorId" class="sr-only">
      {{ errorMessage }}
    </span>

    <div v-else-if="invalid" :id="errorId">
      <p
        class="m-0 mt-1.5 border-0 bg-transparent p-0 text-admin-meta font-semibold text-destructive"
        role="alert"
        data-severity="error"
      >
        {{ errorMessage }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'
import { displayErrorMessage } from '../../core/utils/formValidation'
import {
  resolveFormFieldState,
  type FormFieldContractProps,
  type FormFieldContractSlotProps,
} from './formFieldContract'

const props = withDefaults(
  defineProps<
    FormFieldContractProps & {
      showMessage?: boolean
      fieldClass?: string
      labelClass?: string
    }
  >(),
  {
    error: '',
    fieldId: undefined,
    required: false,
    showMessage: true,
    fieldClass: '',
    labelClass: '',
  },
)

defineSlots<{
  default(props: FormFieldContractSlotProps): unknown
  'header-extra'?(): unknown
}>()

const generatedId = useId()
// Id/a11y state is derived once in the neutral contract: this adapter renders
// no hint surface, so hint stays out of the describedby chain here.
const state = computed(() =>
  resolveFormFieldState({
    fieldId: props.fieldId,
    fallbackId: `ib-form-field-${generatedId}`,
    error: props.error,
  }),
)
const resolvedFieldId = computed(() => state.value.fieldId)
const invalid = computed(() => state.value.invalid)
const errorId = computed(() => state.value.errorId)
const errorMessage = computed(() => displayErrorMessage(props.error))
const slotProps = computed<FormFieldContractSlotProps>(() => ({
  ...state.value,
  required: props.required,
}))
</script>
