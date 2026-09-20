<template>
  <AdminDialogShell
    :visible="visible"
    :header="header"
    description="完整 Code 仅在此展示一次，请立即复制保存。"
    size="compact"
    @update:visible="emit('update:visible', $event)"
  >
    <p class="m-0 text-admin-body text-admin-text-2">关闭后将无法再次查看完整 Code。</p>
    <div
      class="flex items-center gap-2 rounded-lg border border-admin-border bg-admin-surface-muted p-3"
    >
      <code
        class="min-w-0 flex-1 select-all break-all text-admin-content font-semibold text-admin-title-color"
      >
        {{ plaintext }}
      </code>
      <AdminButton
        variant="outline"
        :class="adminActionButtonClass"
        @click="copy"
      >
        {{ copied ? '已复制' : '复制' }}
      </AdminButton>
    </div>
    <AdminAlert v-if="copyError" severity="error">
      复制失败，请手动选择并复制 Code。
    </AdminAlert>
    <template #footer>
      <AdminButton
        :class="adminActionButtonClass"
        @click="emit('update:visible', false)"
      >
        我已保存
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

const props = withDefaults(
  defineProps<{ visible: boolean; plaintext: string; header?: string }>(),
  { header: '保存兑换码' },
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
