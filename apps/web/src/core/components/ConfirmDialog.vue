<template>
  <!-- Admin surface: destructive operator confirm owns `alertdialog`. -->
  <AlertDialogRoot v-if="isAdminBranch" :open="Boolean(resolvedOpen)">
    <AlertDialogPortal>
      <AlertDialogOverlay :class="adminConfirmOverlayMaskClass" />
      <AlertDialogContent
        class="fixed top-[50%] left-[50%] z-admin-dialog grid w-full max-w-md translate-x-[-50%] translate-y-[-50%] gap-4 rounded-overlay border border-border bg-popover text-card-foreground p-6 shadow-admin-dialog overlay-dialog-motion"
      >
        <div class="flex flex-col gap-2">
          <AlertDialogTitle class="text-admin-title font-bold text-foreground">
            {{ resolvedTitle }}
          </AlertDialogTitle>
          <AlertDialogDescription class="text-admin-meta leading-5 text-muted-foreground">
            {{ resolvedMessage }}
          </AlertDialogDescription>
        </div>

        <div class="flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2 gap-2 pt-2">
          <button
            type="button"
            class="inline-flex items-center justify-center rounded-control border border-input bg-card px-4 py-2 text-admin-body font-medium text-foreground shadow-raised hover:bg-muted active:bg-muted/80 cursor-pointer transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
            :disabled="resolvedLoading"
            @click="handleCancel"
          >
            取消
          </button>
          <button
            type="button"
            :class="[
              'inline-flex items-center justify-center rounded-control px-4 py-2 text-admin-body font-medium shadow-raised cursor-pointer transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] disabled:opacity-50',
              resolvedIntent === 'danger'
                ? 'bg-destructive text-destructive-foreground hover:bg-destructive-hover active:bg-destructive-pressed'
                : 'bg-primary text-primary-foreground hover:bg-primary-hover active:bg-primary-pressed',
            ]"
            :disabled="resolvedLoading"
            @click="handleConfirm"
          >
            {{ resolvedLoading ? '处理中...' : resolvedConfirmLabel }}
          </button>
        </div>
      </AlertDialogContent>
    </AlertDialogPortal>
  </AlertDialogRoot>
  <!-- User surface: blocking confirm owns `dialog`. -->
  <DialogShell
    v-else
    :is-open="resolvedOpen"
    :title="resolvedTitle"
    :description="resolvedMessage"
    size="slim-inset"
    :is-busy="resolvedLoading"
    :has-close="confirmPreset.hasClose"
    content-class="bg-popover"
    @update:open="handleOpenUpdate"
  >
    <div class="min-h-0 overflow-y-auto">
      <div class="p-6">
        <h3 class="m-0 text-lg font-semibold text-foreground">
          {{ resolvedTitle }}
        </h3>
        <p class="mt-5 mb-0 text-sm leading-6 text-muted-foreground">
          {{ resolvedMessage }}
        </p>
        <div v-if="props.expectedConfirmText" class="mt-4 flex flex-col gap-2">
          <p class="m-0 text-sm font-medium text-foreground">
            {{ props.confirmInputLabel }}
          </p>
          <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The typed-confirm input carries its accessible name via aria-label -->
          <Input
            :id="confirmInputId"
            v-model="confirmInput"
            size="sm"
            class="h-9 text-sm bg-background"
            :disabled="resolvedLoading"
            :aria-label="props.confirmInputLabel || props.expectedConfirmText"
            :placeholder="props.confirmInputPlaceholder"
            @keydown.enter="handleConfirmInputEnter"
          />
        </div>
        <div class="mt-6 flex justify-end gap-2">
          <Button
            type="button"
            variant="ghost"
            :is-disabled="resolvedLoading"
            class="rounded-pill px-4 py-2 text-sm font-semibold text-muted-foreground hover:bg-muted hover:text-foreground"
            @click="handleCancel"
          >
            {{ t('common.actions.cancel') }}
          </Button>
          <Button
            type="button"
            :is-disabled="resolvedLoading || !isConfirmTextSatisfied"
            :class="[
              'rounded-pill px-4 py-2 text-sm font-semibold disabled:opacity-50',
              resolvedIntent === 'danger'
                ? 'bg-destructive text-destructive-foreground hover:bg-destructive-hover'
                : 'bg-primary text-primary-foreground hover:bg-primary-hover',
            ]"
            @click="handleConfirm"
          >
            {{ resolvedConfirmLabel || t('common.actions.confirm') }}
          </Button>
        </div>
      </div>
    </div>
  </DialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogOverlay,
  AlertDialogPortal,
  AlertDialogRoot,
  AlertDialogTitle,
} from 'reka-ui'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useAppToast } from '@/core/composables/useAppToast'
import { formatDisplayErrorMessage } from '@/core/utils/errorMessages'
import {
  activeConfirm,
  type ConfirmIntent,
  type ConfirmSurface,
} from '@/core/composables/useConfirmDialog'
import DialogShell from './DialogShell.vue'
import { dialogShellPresets } from './dialogShell'

const props = withDefaults(
  defineProps<{
    surface?: ConfirmSurface
    isVisible?: boolean | undefined
    title?: string
    message?: string
    confirmLabel?: string
    intent?: ConfirmIntent
    isLoading?: boolean
    expectedConfirmText?: string | undefined
    confirmInputLabel?: string | undefined
    confirmInputPlaceholder?: string | undefined
  }>(),
  {
    surface: 'user',
    isVisible: undefined,
    title: '',
    message: '',
    confirmLabel: '',
    intent: 'primary',
    isLoading: false,
    expectedConfirmText: undefined,
    confirmInputLabel: '',
    confirmInputPlaceholder: '',
  },
)

const emit = defineEmits<{
  'update:isVisible': [visible: boolean]
  confirm: []
}>()

const { t } = useI18n()
const confirmPreset = dialogShellPresets.confirm

// Mirrors the admin overlay mask (theme tokens only); core must not import
// the admin feature barrel, so the literal lives here for the admin branch.
const adminConfirmOverlayMaskClass =
  'fixed inset-0 z-admin-dialog bg-scrim backdrop-blur-xs overlay-mask-motion'

// Controlled mode (user surfaces pass `isVisible`); global mode (admin shell
// mounts bare `<ConfirmDialog surface="admin" />`) reads the shared singleton.
const isAdminBranch = computed(() => props.surface === 'admin')
const isGlobalMode = computed(() => isAdminBranch.value && props.isVisible === undefined)

const resolvedOpen = computed(() =>
  isGlobalMode.value ? Boolean(activeConfirm.value?.open) : (props.isVisible ?? false),
)
const resolvedTitle = computed(() =>
  isGlobalMode.value ? (activeConfirm.value?.title ?? '') : props.title,
)
const resolvedMessage = computed(() =>
  isGlobalMode.value ? (activeConfirm.value?.message ?? '') : props.message,
)
const resolvedConfirmLabel = computed(() =>
  isGlobalMode.value ? (activeConfirm.value?.confirmLabel ?? '') : props.confirmLabel,
)
const resolvedIntent = computed<ConfirmIntent>(() =>
  isGlobalMode.value ? (activeConfirm.value?.intent ?? 'primary') : props.intent,
)
const resolvedLoading = computed(() =>
  isGlobalMode.value ? Boolean(activeConfirm.value?.loading) : props.isLoading,
)

// Typed confirmation for irreversible actions (e.g. workspace permanent
// delete): the caller passes the exact expected text and the confirm control
// stays disabled until the input matches. Callers without
// `expectedConfirmText` keep the direct confirm.
const confirmInputId = 'confirm-typed-input'
const confirmInput = ref('')
watch([() => props.isVisible, () => activeConfirm.value?.open], () => {
  confirmInput.value = ''
})
const isConfirmTextSatisfied = computed(() => {
  const expected = props.expectedConfirmText
  if (!expected) return true
  return confirmInput.value === expected
})

function handleConfirmInputEnter() {
  if (resolvedLoading.value || !isConfirmTextSatisfied.value) return
  void handleConfirm()
}

// Unified dialog contract lives in DialogShell (mask/Escape/close-control
// dismiss while idle; the irreversible confirm commit locks every path).
// Promise settlement on parent unmount stays in useConfirmDialog.dispose.
function handleOpenUpdate(open: boolean) {
  if (!open) handleCancel()
}

function handleCancel() {
  if (isGlobalMode.value) {
    activeConfirm.value?.cancel()
    return
  }
  emit('update:isVisible', false)
}

const toast = useAppToast()

async function handleConfirm() {
  if (isGlobalMode.value) {
    try {
      await activeConfirm.value?.confirm()
    } catch (error) {
      toast.error(formatDisplayErrorMessage(error, '操作失败，请重试'))
    }
    return
  }
  emit('confirm')
}
</script>
