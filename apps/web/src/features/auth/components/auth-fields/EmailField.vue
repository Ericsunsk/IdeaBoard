<template>
  <AuthFieldShell
    :label="label"
    :error="error"
    :field-id="fieldId"
    :required="required"
    :field-class="fieldClass"
    :icon-class="iconClass"
    :loading="loading"
    :loading-label="loadingLabel"
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
      <Input
        :id="slotFieldId"
        v-model="model"
        :type="inputType"
        v-bind="controlState(invalid)"
        :autocomplete="autocomplete"
        :required="slotRequired"
        :aria-busy="loading || undefined"
        :class="[
          inputClass,
          $slots.prefix ? 'pl-10' : '',
          loading ? 'pr-10' : '',
          invalid ? inputErrorClass : '',
        ]"
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
import { resolveInteractiveControlState } from '../../../../core/interaction/interactiveControlState'
import { ibFormInputIconClass } from '../../../../theme/formControls'

const props = withDefaults(
  defineProps<{
    label: string
    error?: string
    fieldId?: string | undefined
    placeholder?: string | undefined
    fieldClass?: string
    inputClass?: HTMLAttributes['class']
    inputErrorClass?: string
    iconClass?: string | undefined
    inputType?: string
    autocomplete?: string | undefined
    disabled?: boolean
    required?: boolean
    loading?: boolean
    loadingLabel?: string | undefined
  }>(),
  {
    error: '',
    fieldId: undefined,
    placeholder: undefined,
    fieldClass: '',
    inputClass: '',
    inputErrorClass: '',
    iconClass: ibFormInputIconClass,
    inputType: 'text',
    autocomplete: undefined,
    disabled: false,
    required: true,
    loading: false,
    loadingLabel: 'Sending…',
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
