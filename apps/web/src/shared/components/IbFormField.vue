<template>
  <div :class="fieldClass">
    <label :for="resolvedFieldId" :class="labelClass">
      {{ label }}
      <span v-if="required" aria-hidden="true">*</span>
    </label>

    <slot v-bind="slotProps" />

    <span v-if="inlineError || !showMessage" :id="errorId" class="sr-only">
      {{ errorMessage }}
    </span>

    <div v-else-if="shouldShowMessage" :id="errorId">
      <Message unstyled severity="error" size="small" :closable="false" :pt="messagePt">
        {{ errorMessage }}
      </Message>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'
import Message from 'primevue/message'
import { displayErrorMessage, isRequiredErrorMessage } from '../utils/formValidation'

export type IbFormFieldSlotProps = {
  ariaInvalid: true | undefined
  describedbyId?: string
  errorId?: string
  fieldId: string
  inlineError: boolean
  inlineErrorText: string
  invalid: boolean
  required: boolean
}

const props = withDefaults(
  defineProps<{
    label: string
    error?: string
    fieldId?: string
    required?: boolean
    inlineRequired?: boolean
    showMessage?: boolean
    fieldClass?: string
    labelClass?: string
    messagePt?: object
  }>(),
  {
    error: '',
    fieldId: undefined,
    required: false,
    inlineRequired: false,
    showMessage: true,
    fieldClass: '',
    labelClass: '',
    messagePt: undefined,
  },
)

defineSlots<{
  default(props: IbFormFieldSlotProps): unknown
}>()

const generatedId = useId()
const resolvedFieldId = computed(() => props.fieldId || `ib-form-field-${generatedId}`)
const errorId = computed(() => `${resolvedFieldId.value}-error`)
const invalid = computed(() => Boolean(props.error))
const inlineError = computed(() => props.inlineRequired && isRequiredErrorMessage(props.error))
const shouldShowMessage = computed(() => props.showMessage && invalid.value && !inlineError.value)
const describedbyId = computed(() => (invalid.value ? errorId.value : undefined))
const errorMessage = computed(() => displayErrorMessage(props.error))
const slotProps = computed<IbFormFieldSlotProps>(() => ({
  ariaInvalid: invalid.value ? true : undefined,
  describedbyId: describedbyId.value,
  errorId: invalid.value ? errorId.value : undefined,
  fieldId: resolvedFieldId.value,
  inlineError: inlineError.value,
  inlineErrorText: inlineError.value ? errorMessage.value : '',
  invalid: invalid.value,
  required: props.required,
}))
</script>
