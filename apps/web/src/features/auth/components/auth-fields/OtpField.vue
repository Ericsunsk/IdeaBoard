<template>
  <IbFormField
    :label="label"
    :error="error"
    :field-id="fieldId"
    :required="required"
    :field-class="fieldClass"
  >
    <template
      #default="{
        ariaInvalid,
        describedbyId,
        fieldId: slotFieldId,
        invalid,
        required: slotRequired,
      }"
    >
      <Input
        :id="slotFieldId"
        v-model="model"
        inputmode="numeric"
        autocomplete="one-time-code"
        maxlength="6"
        v-bind="controlState(invalid)"
        :required="slotRequired"
        :class="[inputClass, invalid ? inputErrorClass : '']"
        :invalid="invalid"
        :placeholder="placeholder"
        :aria-invalid="ariaInvalid"
        :aria-describedby="describedbyId"
        @blur="emit('blur')"
      />
    </template>
  </IbFormField>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { Input } from '@/components/ui/input'
import IbFormField from '../../../../shared/components/IbFormField.vue'
import { resolveInteractiveControlState } from '../../../../shared/interaction/interactiveControlState'

const props = withDefaults(
  defineProps<{
    label: string
    error?: string
    fieldId?: string | undefined
    placeholder?: string | undefined
    fieldClass?: string
    inputClass?: HTMLAttributes['class']
    inputErrorClass?: string
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
    disabled: false,
    required: true,
  },
)

const emit = defineEmits<{
  blur: []
}>()

const model = defineModel<string>({ required: true })

function controlState(invalid: boolean) {
  return resolveInteractiveControlState({
    disabled: props.disabled,
    tone: invalid ? 'danger' : 'default',
  })
}
</script>
