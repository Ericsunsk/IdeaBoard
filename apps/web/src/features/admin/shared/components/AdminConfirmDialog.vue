<template>
  <AlertDialogRoot :open="Boolean(activeAdminConfirm?.open)">
    <AlertDialogPortal>
      <AlertDialogOverlay
        class="fixed inset-0 z-50 bg-black/40 backdrop-blur-xs data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
      />
      <AlertDialogContent
        class="fixed top-[50%] left-[50%] z-50 grid w-full max-w-md translate-x-[-50%] translate-y-[-50%] gap-4 rounded-xl border border-admin-border bg-admin-card-strong p-6 shadow-xl data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[state=closed]:slide-out-to-left-1/2 data-[state=closed]:slide-out-to-top-[48%] data-[state=open]:slide-in-from-left-1/2 data-[state=open]:slide-in-from-top-[48%]"
      >
        <div class="flex flex-col gap-2">
          <AlertDialogTitle class="text-admin-title font-bold text-admin-title-color">
            {{ activeAdminConfirm?.title }}
          </AlertDialogTitle>
          <AlertDialogDescription class="text-admin-meta leading-5 text-admin-text-2">
            {{ activeAdminConfirm?.message }}
          </AlertDialogDescription>
        </div>

        <label
          v-if="activeAdminConfirm?.reasonEnabled"
          for="admin-confirm-reason-textarea"
          class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
        >
          <span>{{ activeAdminConfirm?.reasonLabel }}</span>
          <textarea
            id="admin-confirm-reason-textarea"
            :value="activeAdminConfirm?.reason ?? ''"
            :placeholder="activeAdminConfirm?.reasonPlaceholder"
            rows="3"
            class="w-full rounded-lg border border-admin-border bg-white px-3 py-2 text-admin-body text-admin-text-1 placeholder:text-admin-text-3 focus:outline-none"
            @input="handleReasonInput"
          />
        </label>

        <div class="flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2 gap-2 pt-2">
          <button
            type="button"
            class="inline-flex items-center justify-center rounded-lg border border-admin-border bg-white px-4 py-2 text-admin-body font-medium text-admin-text-1 shadow-xs hover:bg-admin-surface-hover active:bg-admin-surface-pressed cursor-pointer transition-colors"
            :disabled="activeAdminConfirm?.loading"
            @click="activeAdminConfirm?.cancel()"
          >
            取消
          </button>
          <button
            type="button"
            :class="[
              'inline-flex items-center justify-center rounded-lg px-4 py-2 text-admin-body font-medium text-white shadow-xs cursor-pointer transition-colors disabled:opacity-50',
              activeAdminConfirm?.intent === 'danger'
                ? 'bg-admin-danger hover:bg-admin-danger-hover active:bg-admin-danger-pressed'
                : 'bg-admin-accent hover:bg-admin-accent-hover active:bg-admin-accent-pressed',
            ]"
            :disabled="
              activeAdminConfirm?.loading ||
              (activeAdminConfirm?.reasonRequired && !activeAdminConfirm?.reason.trim())
            "
            @click="handleConfirm"
          >
            {{ activeAdminConfirm?.loading ? '处理中...' : activeAdminConfirm?.confirmLabel }}
          </button>
        </div>
      </AlertDialogContent>
    </AlertDialogPortal>
  </AlertDialogRoot>
</template>

<script setup lang="ts">
import {
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogOverlay,
  AlertDialogPortal,
  AlertDialogRoot,
  AlertDialogTitle,
} from 'reka-ui'
import { activeAdminConfirm } from '../composables/useAdminConfirm'
import { formatAdminDisplayError, useAdminToast } from '../lib/toast'

const toast = useAdminToast()

function handleReasonInput(event: Event) {
  const target = event.target as HTMLTextAreaElement | null
  activeAdminConfirm.value?.setReason(target?.value ?? '')
}

async function handleConfirm() {
  try {
    await activeAdminConfirm.value?.confirm()
  } catch (error) {
    toast.error(formatAdminDisplayError(error, '操作失败，请重试'))
  }
}
</script>
