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
        <span
          v-if="loading"
          class="pointer-events-none absolute top-1/2 right-3.5 z-10 flex -translate-y-1/2 items-center justify-center text-muted-foreground"
        >
          <span role="status" class="flex items-center">
            <Spinner class="size-4 shrink-0" aria-hidden="true" />
            <span class="sr-only">{{ loadingLabel }}</span>
          </span>
        </span>
      </div>
    </template>
  </IbFormField>
</template>

<script setup lang="ts">
import { Spinner } from '@/components/ui/spinner'
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
    loading?: boolean
    loadingLabel?: string | undefined
  }>(),
  {
    error: '',
    fieldId: undefined,
    fieldClass: '',
    iconClass: ibFormInputIconClass,
    required: true,
    loading: false,
    loadingLabel: 'Sending…',
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
