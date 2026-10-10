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
      <ActionButton variant="outline" @click="copy">
        {{ REWARD_CODES_MESSAGES.plaintextCopyLabel(copied) }}
      </ActionButton>
    </div>
    <template #footer>
      <ActionButton @click="emit('update:visible', false)">
        {{ REWARD_CODES_MESSAGES.plaintextAcknowledged }}
      </ActionButton>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import ActionButton from '../../../../core/components/ActionButton.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import { useAdminToast } from '../../shared/lib/toast'
import { useAppClipboard } from '../../../../core/composables/useAppClipboard'
import { REWARD_CODES_MESSAGES } from './messages'

const props = withDefaults(
  defineProps<{ visible: boolean; plaintext: string; header?: string }>(),
  { header: REWARD_CODES_MESSAGES.plaintextHeader },
)
const emit = defineEmits<{ 'update:visible': [visible: boolean] }>()
const copied = ref(false)
const toast = useAdminToast()

watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      copied.value = false
    }
  },
)

const { copyText } = useAppClipboard()

async function copy() {
  const ok = await copyText(props.plaintext)
  copied.value = ok
  if (!ok) toast.error(REWARD_CODES_MESSAGES.plaintextCopyFailed)
}
</script>
