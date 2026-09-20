<template>
  <slot :field="field" :error="error" :invalid="invalid" />
</template>

<script setup lang="ts">
import { computed, inject, provide, useId } from 'vue'
import {
  FORM_CONTEXT_KEY,
  FORM_FIELD_CONTEXT_KEY,
  FORM_ITEM_CONTEXT_KEY,
} from './context'

const props = defineProps<{
  name: string
}>()

provide(FORM_FIELD_CONTEXT_KEY, { name: props.name })

const formContext = inject(FORM_CONTEXT_KEY, null)
const itemContext = inject(FORM_ITEM_CONTEXT_KEY, null)

const generatedId = useId()
const id = computed(() => itemContext?.id ?? `form-item-${generatedId}`)
const formItemId = id
const formDescriptionId = computed(() => `${id.value}-description`)
const formMessageId = computed(() => `${id.value}-message`)

const error = computed(() => {
  if (!formContext) return undefined
  return formContext.errors.value[props.name]
})

const isTouched = computed(() => {
  if (!formContext) return false
  return Boolean(formContext.touched.value[props.name])
})

const invalid = computed(() => Boolean(error.value))

const ariaDescribedby = computed(() => {
  return error.value ? `${formDescriptionId.value} ${formMessageId.value}` : formDescriptionId.value
})

const field = computed(() => ({
  id: id.value,
  name: props.name,
  formItemId: formItemId.value,
  formDescriptionId: formDescriptionId.value,
  formMessageId: formMessageId.value,
  error: error.value,
  isTouched: isTouched.value,
  invalid: invalid.value,
  ariaDescribedby: ariaDescribedby.value,
}))
</script>
