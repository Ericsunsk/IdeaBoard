<template>
  <label :for="props.for" :class="fieldClass">
    <span :class="labelTextClass">
      {{ label }}
      <span v-if="required" class="text-admin-danger" aria-hidden="true">*</span>
    </span>

    <slot v-bind="slotProps" />

    <p v-if="hint" :id="hintElementId" class="m-0 text-admin-caption text-admin-text-3">
      {{ hint }}
    </p>

    <Message
      v-if="hasError"
      :id="errorElementId"
      severity="error"
      size="small"
      :closable="false"
    >
      {{ error }}
    </Message>
  </label>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'
import Message from 'primevue/message'

interface AdminFormFieldSlotProps {
  describedbyId?: string
  errorId?: string
  hintId?: string
  invalid: boolean
  required: boolean
}

const props = withDefaults(
  defineProps<{
    label: string
    for?: string
    error?: string
    hint?: string
    required?: boolean
    labelClass?: string
    bodyClass?: string
  }>(),
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
  default(props: AdminFormFieldSlotProps): unknown
}>()

const generatedId = useId()
const fieldId = computed(() => props.for || `admin-form-field-${generatedId}`)
const hasError = computed(() => Boolean(props.error))
const hasHint = computed(() => Boolean(props.hint))
const hintElementId = computed(() => (hasHint.value ? `${fieldId.value}-hint` : undefined))
const errorElementId = computed(() => (hasError.value ? `${fieldId.value}-error` : undefined))
const describedbyId = computed(() =>
  [hintElementId.value, errorElementId.value].filter(Boolean).join(' ') || undefined,
)
const fieldClass = computed(() =>
  props.bodyClass ? ['flex min-w-0 flex-col', props.bodyClass] : 'flex min-w-0 flex-col gap-1.5',
)
const labelTextClass = computed(
  () => props.labelClass || 'text-admin-meta font-[680] text-admin-text-2',
)
const slotProps = computed<AdminFormFieldSlotProps>(() => ({
  describedbyId: describedbyId.value,
  errorId: errorElementId.value,
  hintId: hintElementId.value,
  invalid: hasError.value,
  required: props.required,
}))
</script>
