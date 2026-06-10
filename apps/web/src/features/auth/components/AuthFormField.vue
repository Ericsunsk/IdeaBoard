<template>
  <label :class="authFieldClass">
    <span>{{ label }}</span>
    <slot :error-id="errorId" :describedby-id="errorId" :invalid="Boolean(error)" />
    <Message v-if="error" :id="errorId" severity="error" size="small" :closable="false">
      {{ error }}
    </Message>
  </label>
</template>

<script setup lang="ts">
import Message from 'primevue/message'
import { authFieldClass } from './authModalUi'

defineProps<{
  label: string
  error: string
  errorId: string
}>()
</script>
