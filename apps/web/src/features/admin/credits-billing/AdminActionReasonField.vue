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
    <Textarea
      :id="inputId"
      :model-value="modelValue"
      :rows="rows"
      :disabled="disabled"
      fluid
      :invalid="invalid"
      :aria-describedby="describedbyId"
      @update:model-value="emit('update:modelValue', $event ?? '')"
    />
  </AdminFormField>
</template>

<script setup lang="ts">
import Textarea from 'primevue/textarea'
import AdminFormField from '../shared/components/AdminFormField.vue'

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
    label: '操作原因',
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
