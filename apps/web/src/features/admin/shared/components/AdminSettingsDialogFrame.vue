<template>
  <AdminDialogShell
    :visible="visible"
    :header="header"
    :description="description"
    size="form"
    :busy="busy"
    @reset="emit('reset')"
    @update:visible="emit('update:visible', $event)"
  >
    <div v-if="loading" class="flex min-h-36 items-center justify-center">
      <AdminSpinner :aria-label="loadingLabel" class="size-8" />
    </div>

    <template v-else>
      <AdminAlert v-if="error" severity="error">
        <div class="flex flex-wrap items-center gap-2">
          <span>{{ error }}</span>
          <AdminButton
            variant="ghost"
            size="small"
            :class="adminTextButtonClass"
            :disabled="busy"
            @click="emit('retry')"
          >
            重新加载
          </AdminButton>
        </div>
      </AdminAlert>

      <slot />
    </template>

    <template v-if="$slots.footer" #footer="slotProps">
      <slot name="footer" v-bind="slotProps" />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import AdminAlert from './AdminAlert.vue'
import AdminButton from './AdminButton.vue'
import AdminDialogShell from './AdminDialogShell.vue'
import AdminSpinner from './AdminSpinner.vue'
import { adminTextButtonClass } from '../lib/uiClasses'

defineProps<{
  visible: boolean
  header: string
  description: string
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
