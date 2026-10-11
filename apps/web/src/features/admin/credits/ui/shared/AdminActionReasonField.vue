<template>
  <AdminFormField
    v-slot="{ describedbyId, invalid }"
    :label="label"
    :for="inputId"
    :error="error"
    :hint="hint"
    required
    :body-class="bodyClass"
  >
    <AdminTextarea
      :id="inputId"
      :model-value="modelValue"
      :rows="rows"
      :disabled="disabled"
      :class="invalid ? '!border-destructive focus-visible:ring-destructive/20' : undefined"
      :aria-describedby="describedbyId"
      @update:model-value="emit('update:modelValue', $event)"
    />
  </AdminFormField>
</template>

<script setup lang="ts">
import { AdminFormField } from '../../../shared'
import { AdminTextarea } from '../../../shared'
import { CREDITS_SHARED_MESSAGES } from './messages'

withDefaults(
  defineProps<{
    modelValue: string
    inputId: string
    error?: string
    label?: string
    hint?: string
    rows?: number
    bodyClass?: string
    disabled?: boolean
  }>(),
  {
    error: '',
    label: CREDITS_SHARED_MESSAGES.actionReasonLabel,
    hint: '',
    rows: 3,
    bodyClass: '',
    disabled: false,
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()
</script>
