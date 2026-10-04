<template>
  <AdminDialogShell
    :visible="visible"
    :header="header"
    :description="REWARD_CODES_MESSAGES.plaintextDescription"
    size="compact"
    @update:visible="emit('update:visible', $event)"
  >
    <p class="m-0 text-admin-body text-muted-foreground">
      {{ REWARD_CODES_MESSAGES.plaintextWarning }}
    </p>
    <div class="flex items-center gap-2 rounded-lg border border-border bg-muted p-3">
      <code
        class="min-w-0 flex-1 select-all break-all text-admin-content font-semibold text-foreground"
      >
        {{ plaintext }}
      </code>
      <AdminButton variant="outline" :class="adminActionButtonClass" @click="copy">
        {{ REWARD_CODES_MESSAGES.plaintextCopyLabel(copied) }}
      </AdminButton>
    </div>
    <AdminAlert v-if="copyError" severity="error">{{
      REWARD_CODES_MESSAGES.plaintextCopyFailed
    }}</AdminAlert>
    <template #footer>
      <AdminButton :class="adminActionButtonClass" @click="emit('update:visible', false)">
        {{ REWARD_CODES_MESSAGES.plaintextAcknowledged }}
      </AdminButton>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import { useAppClipboard } from '../../../../shared/composables/useAppClipboard'
import { adminActionButtonClass } from '../../shared/lib/uiClasses'
import { REWARD_CODES_MESSAGES } from './messages'

const props = withDefaults(
  defineProps<{ visible: boolean; plaintext: string; header?: string }>(),
  { header: REWARD_CODES_MESSAGES.plaintextHeader },
)
const emit = defineEmits<{ 'update:visible': [visible: boolean] }>()
const copied = ref(false)
const copyError = ref(false)

watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      copied.value = false
      copyError.value = false
    }
  },
)

const { copyText } = useAppClipboard()

async function copy() {
  const ok = await copyText(props.plaintext)
  copied.value = ok
  copyError.value = !ok
}
</script>
