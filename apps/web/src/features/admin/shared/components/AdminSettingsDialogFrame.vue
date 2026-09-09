<template>
  <AdminDialogShell
    :visible="visible"
    :header="header"
    size="form"
    :busy="busy"
    @reset="emit('reset')"
    @update:visible="emit('update:visible', $event)"
  >
    <div v-if="loading" class="flex min-h-36 items-center justify-center">
      <ProgressSpinner :aria-label="loadingLabel" stroke-width="3" />
    </div>

    <template v-else>
      <Message v-if="error" severity="error" :closable="false">
        <div class="flex flex-wrap items-center gap-2">
          <span>{{ error }}</span>
          <Button
            label="重新加载"
            severity="secondary"
            size="small"
            text
            :class="adminTextButtonClass"
            :disabled="busy"
            @click="emit('retry')"
          />
        </div>
      </Message>

      <slot />
    </template>

    <template v-if="$slots.footer" #footer="slotProps">
      <slot name="footer" v-bind="slotProps" />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import AdminDialogShell from './AdminDialogShell.vue'
import { adminTextButtonClass } from '../lib/uiClasses'

defineProps<{
  visible: boolean
  header: string
  busy: boolean
  loading: boolean
  loadingLabel: string
  error: string | null
}>()

const emit = defineEmits<{
  reset: []
  'update:visible': [visible: boolean]
  retry: []
}>()
</script>
