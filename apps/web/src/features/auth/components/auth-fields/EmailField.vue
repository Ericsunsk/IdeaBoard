<template>
  <AuthFieldShell
    :label="label"
    :error="error"
    :field-id="fieldId"
    :required="required"
    :field-class="fieldClass"
    :icon-class="iconClass"
  >
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
      <Input
        :id="slotFieldId"
        v-model="model"
        :type="inputType"
        v-bind="controlState(invalid)"
        :autocomplete="autocomplete"
        :required="slotRequired"
        :class="[inputClass, $slots.prefix ? 'pl-10' : '', invalid ? inputErrorClass : '']"
        :invalid="invalid"
        :placeholder="placeholder"
        :aria-invalid="ariaInvalid"
        :aria-describedby="describedbyId"
        @blur="emit('blur')"
      />
    </template>
  </AuthFieldShell>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { Input } from '@/components/ui/input'
import AuthFieldShell from './AuthFieldShell.vue'
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
    iconClass?: string
    inputType?: string
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
    iconClass: '',
    inputType: 'text',
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

function controlState(invalid: boolean) {
  return resolveInteractiveControlState({
    disabled: props.disabled,
    tone: invalid ? 'danger' : 'default',
  })
}
</script>
