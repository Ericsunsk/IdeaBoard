<template>
  <IbFormField
    :label="label"
    :error="error"
    :field-id="fieldId"
    :required="required"
    :field-class="fieldClass"
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
      <div class="relative w-full">
        <span v-if="$slots.prefix" :class="iconClass">
          <slot name="prefix" />
        </span>
        <slot
          v-bind="{
            ariaInvalid,
            describedbyId,
            fieldId: slotFieldId,
            invalid,
            required: slotRequired,
          }"
        />
      </div>
    </template>
  </IbFormField>
</template>

<script setup lang="ts">
import IbFormField, {
  type IbFormFieldSlotProps,
} from '../../../../shared/components/IbFormField.vue'
import { ibFormInputIconClass } from '../../../../shared/theme/formControls'

withDefaults(
  defineProps<{
    label: string
    error?: string
    fieldId?: string | undefined
    fieldClass?: string
    iconClass?: string | undefined
    required?: boolean
  }>(),
  {
    error: '',
    fieldId: undefined,
    fieldClass: '',
    iconClass: ibFormInputIconClass,
    required: true,
  },
)

defineSlots<{
  default(
    props: Pick<
      IbFormFieldSlotProps,
      'ariaInvalid' | 'describedbyId' | 'fieldId' | 'invalid' | 'required'
    >,
  ): unknown
  prefix?: () => unknown
  'header-extra'?: () => unknown
}>()
</script>
