<template>
  <Dialog
    :visible="visible"
    modal
    :show-header="false"
    :style="{ width: 'min(420px, calc(100vw - 32px))' }"
    :pt="dialogPt"
    @update:visible="emit('update:visible', $event)"
  >
    <div class="p-6">
      <h3 class="m-0 text-lg font-semibold">{{ title }}</h3>
      <p class="mt-5 mb-0 text-sm leading-6 text-board-page-text-muted">{{ message }}</p>
      <div class="mt-6 flex justify-end gap-2">
        <Button
          unstyled
          type="button"
          :label="t('common.actions.cancel')"
          class="rounded-full px-4 py-2 text-sm font-semibold text-board-page-text-muted hover:bg-ib-surface-hover"
          :disabled="loading"
          @click="emit('update:visible', false)"
        />
        <Button
          unstyled
          type="button"
          :label="confirmLabel || t('common.actions.confirm')"
          :disabled="loading"
          :class="[
            'rounded-full px-4 py-2 text-sm font-semibold disabled:opacity-50',
            intent === 'danger'
              ? 'bg-board-danger text-white'
              : 'bg-board-primary text-ib-text-inverse',
          ]"
          @click="emit('confirm')"
        />
      </div>
    </div>
  </Dialog>
</template>

<script setup lang="ts">
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import { useI18n } from 'vue-i18n'

withDefaults(
  defineProps<{
    visible: boolean
    title: string
    message: string
    confirmLabel?: string
    intent?: 'primary' | 'danger'
    loading?: boolean
  }>(),
  {
    confirmLabel: '',
    intent: 'primary',
    loading: false,
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  confirm: []
}>()

const { t } = useI18n()

const dialogPt = {
  root: { class: '!rounded-2xl !border-board-card-border' },
  content: { class: '!p-0' },
}
</script>
