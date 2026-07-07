<template>
  <Dialog
    :visible="visible"
    modal
    dismissable-mask
    :draggable="false"
    :style="profileDialogStyle"
    :pt="profileDialogPt"
    aria-labelledby="profile-dialog-title"
    @update:visible="emit('update:visible', $event)"
  >
    <template #header>
      <div class="min-w-0">
        <h2 id="profile-dialog-title" class="m-0 text-[22px] leading-tight font-bold text-ib-text">
          个人中心
        </h2>
        <p class="mt-1 mb-0 text-sm leading-5 text-ib-text-subtle">管理个人资料与账号安全</p>
      </div>
    </template>

    <ProfileDialogContent v-if="visible" />
  </Dialog>
</template>

<script setup lang="ts">
import Dialog from 'primevue/dialog'
import type { DialogPassThroughOptions } from 'primevue/dialog'
import ProfileDialogContent from './ProfileDialogContent.vue'

defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  'update:visible': [visible: boolean]
}>()

const profileDialogStyle = {
  width: 'min(920px, calc(100vw - 32px))',
} as const

const profileDialogPt = {
  root: {
    class:
      '!max-h-[calc(100dvh-32px)] !overflow-hidden !rounded-[var(--radius-ib-overlay)] !border !border-ib-border-soft !bg-ib-surface-raised !shadow-[var(--shadow-ib-overlay)] max-[640px]:!max-h-[calc(100dvh-20px)]',
  },
  header: {
    class: '!border-b !border-ib-border-soft !px-8 !py-5 max-[640px]:!px-4 max-[640px]:!py-4',
  },
  content: {
    class: '!overflow-hidden !p-0',
  },
} satisfies DialogPassThroughOptions
</script>
