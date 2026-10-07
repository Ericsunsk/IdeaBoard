<template>
  <AuthFieldShell
    :label="label"
    :error="error"
    :field-id="fieldId"
    :required="required"
    :field-class="fieldClass"
    :icon-class="iconClass"
  >
    <template v-if="$slots.prefix" #prefix>
      <slot name="prefix" />
    </template>
    <template v-if="$slots['header-extra']" #header-extra>
      <slot name="header-extra" />
    </template>
    <template
      #default="{
        ariaInvalid,
        describedbyId,
        fieldId: slotFieldId,
        invalid,
        required: slotRequired,
      }"
    >
      <PasswordInput
        :id="slotFieldId"
        v-model="model"
        :autocomplete="autocomplete"
        :required="slotRequired"
        :disabled="disabled"
        :aria-invalid="ariaInvalid"
        :aria-describedby="describedbyId"
        :invalid="invalid"
        :input-class="[inputClass, $slots.prefix ? 'pl-10' : '', invalid ? inputErrorClass : '']"
        :placeholder="placeholder"
        @blur="emit('blur')"
      />
    </template>
  </AuthFieldShell>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { PasswordInput } from '@/components/ui/input'
import AuthFieldShell from './AuthFieldShell.vue'

withDefaults(
  defineProps<{
    label: string
    error?: string
    fieldId?: string | undefined
    placeholder?: string | undefined
    fieldClass?: string
    inputClass?: HTMLAttributes['class']
    inputErrorClass?: string
    iconClass?: string
    autocomplete?: string | undefined
    disabled?: boolean
    required?: boolean
  }>(),
  {
    error: '',
    fieldId: undefined,
    placeholder: undefined,
    fieldClass: '',
    inputClass: '',
    inputErrorClass: '',
    autocomplete: undefined,
    disabled: false,
    required: true,
  },
)

const emit = defineEmits<{
  blur: []
}>()

defineSlots<{
  prefix?: () => unknown
  'header-extra'?: () => unknown
}>()

const model = defineModel<string>({ required: true })
</script>
