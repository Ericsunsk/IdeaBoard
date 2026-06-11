<template>
  <label :class="authFieldClass">
    <span>{{ label }}</span>
    <slot v-bind="slotProps" />
    <Message
      v-if="shouldShowMessage"
      :id="errorId"
      unstyled
      severity="error"
      size="small"
      :closable="false"
      :pt="authErrorMessagePt"
    >
      {{ displayErrorMessage(error) }}
    </Message>
  </label>
</template>

<script setup lang="ts">
import Message from 'primevue/message'
import { isRequiredErrorMessage, displayErrorMessage } from '../../../shared/utils/formValidation'
import { authErrorMessagePt, authFieldClass } from './authModalUi'

const props = withDefaults(
  defineProps<{
    label: string
    error: string
    errorId: string
    inlineRequired?: boolean
    showMessage?: boolean
  }>(),
  {
    inlineRequired: true,
    showMessage: true,
  },
)

const isInlineRequiredError = computed(
  () => props.inlineRequired && isRequiredErrorMessage(props.error),
)
const shouldShowMessage = computed(
  () => props.showMessage && props.error && !isInlineRequiredError.value,
)
const describedbyId = computed(() => (shouldShowMessage.value ? props.errorId : undefined))
const slotProps = computed(() => ({
  errorId: props.errorId,
  describedbyId: describedbyId.value,
  inlineError: isInlineRequiredError.value,
  inlineErrorText: isInlineRequiredError.value ? displayErrorMessage(props.error) : '',
  invalid: Boolean(props.error),
}))
</script>
