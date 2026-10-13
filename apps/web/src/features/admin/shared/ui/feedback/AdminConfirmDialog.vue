<template>
  <AlertDialogRoot :open="Boolean(activeAdminConfirm?.open)">
    <AlertDialogPortal>
      <AlertDialogOverlay
        class="fixed inset-0 z-admin-dialog bg-scrim backdrop-blur-xs data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
      />
      <AlertDialogContent
        class="fixed top-[50%] left-[50%] z-admin-dialog grid w-full max-w-md translate-x-[-50%] translate-y-[-50%] gap-4 rounded-admin-card border border-border bg-card text-card-foreground p-6 shadow-admin-dialog data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[state=closed]:slide-out-to-left-1/2 data-[state=closed]:slide-out-to-top-[48%] data-[state=open]:slide-in-from-left-1/2 data-[state=open]:slide-in-from-top-[48%]"
      >
        <div class="flex flex-col gap-2">
          <AlertDialogTitle class="text-admin-title font-bold text-foreground">
            {{ activeAdminConfirm?.title }}
          </AlertDialogTitle>
          <AlertDialogDescription class="text-admin-meta leading-5 text-muted-foreground">
            {{ activeAdminConfirm?.message }}
          </AlertDialogDescription>
        </div>

        <div class="flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2 gap-2 pt-2">
          <button
            type="button"
            class="inline-flex items-center justify-center rounded-lg border border-input bg-card px-4 py-2 text-admin-body font-medium text-foreground shadow-xs hover:bg-muted active:bg-muted/80 cursor-pointer transition-colors"
            :disabled="activeAdminConfirm?.loading"
            @click="activeAdminConfirm?.cancel()"
          >
            取消
          </button>
          <button
            type="button"
            :class="[
              'inline-flex items-center justify-center rounded-lg px-4 py-2 text-admin-body font-medium shadow-xs cursor-pointer transition-colors disabled:opacity-50',
              activeAdminConfirm?.intent === 'danger'
                ? 'bg-destructive text-destructive-foreground hover:bg-destructive-hover active:bg-destructive-pressed'
                : 'bg-primary text-primary-foreground hover:bg-primary-hover active:bg-primary-pressed',
            ]"
            :disabled="activeAdminConfirm?.loading"
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
import { activeAdminConfirm } from '../../session/feedback/useAdminConfirm'
import { formatAdminDisplayError, useAdminToast } from '../../session/feedback/toast'

const toast = useAdminToast()

async function handleConfirm() {
  try {
    await activeAdminConfirm.value?.confirm()
  } catch (error) {
    toast.error(formatAdminDisplayError(error, '操作失败，请重试'))
  }
}
</script>
