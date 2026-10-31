<template>
  <DialogRoot :open="visible" @update:open="handleOpenUpdate">
    <DialogPortal>
      <DialogOverlay
        data-admin-dialog-mask
        :class="adminOverlayMaskClass"
        @click="requestCloseFromMask"
      />
      <DialogContent
        role="dialog"
        aria-modal="true"
        :aria-label="contentAriaLabel"
        :aria-busy="hasAriaBusy"
        :data-size="props.size ?? 'form'"
        :class="[
          'fixed top-[50%] left-[50%] z-admin-dialog grid max-h-[calc(100dvh-1.5rem)] translate-x-[-50%] translate-y-[-50%] gap-0 overflow-hidden border shadow-admin-dialog overlay-dialog-motion',
          adminDialogChromeClass,
          dialogWidthClasses[props.size ?? 'form'],
        ]"
        @escape-key-down="handleEscape"
        @pointer-down-outside="blockOutsideDismiss"
      >
        <div
          data-admin-dialog-region="header"
          :class="[adminDialogHeaderClass, adminDialogChromeHeaderClass]"
        >
          <div :class="[adminDialogHeaderSplitClass, adminDialogChromeHeaderSplitClass]">
            <div :class="adminDialogHeaderContentClass">
              <template v-if="hasCustomHeader">
                <!-- Custom headers own visual layout; the hidden title supplies the dialog's accessible name. -->
                <DialogTitle class="sr-only">{{ contentAriaLabel }}</DialogTitle>
                <slot name="header" />
              </template>
              <DialogTitle v-else :class="[adminDialogTitleClass, adminDialogChromeTitleClass]">
                {{ props.header }}
              </DialogTitle>
            </div>
            <button
              v-if="canDismiss('close-control')"
              type="button"
              aria-label="关闭"
              :class="[
                '!bg-transparent !shadow-none inline-flex size-8 shrink-0 items-center justify-center rounded-control border-0 p-0 hover:!bg-transparent focus:!outline-none focus-visible:!outline-none focus-visible:!shadow-none focus-visible:!ring-0 cursor-pointer transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] font-admin-strong',
                adminDialogChromeCloseClass,
              ]"
              @click="requestCloseFromControl"
            >
              <Xmark class="!h-5 !w-5" data-icon="xmark" aria-hidden="true" />
            </button>
          </div>
        </div>

        <div data-admin-dialog-region="content" :class="adminDialogChromeContentClass">
          <div :class="contentClass">
            <DialogDescription v-if="props.description" class="sr-only">
              {{ props.description }}
            </DialogDescription>
            <slot />
          </div>
        </div>

        <div
          v-if="$slots.footer"
          data-admin-dialog-region="footer"
          :class="adminDialogChromeFooterClass"
        >
          <div :class="adminDialogFooterActionsClass" data-admin-dialog-footer>
            <slot name="footer" :request-close="requestCloseFromControl" />
          </div>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup lang="ts">
import { computed, useSlots } from 'vue'
import { Xmark } from '@iconoir/vue'
import {
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from 'reka-ui'
import { useAdminDialogInteraction } from '../../session/overlay/useAdminDialogInteraction'
import { dialogShellSizeClasses, type DialogShellSize } from '@/core/components/dialogShell'
import {
  adminDialogChromeClass,
  adminDialogChromeCloseClass,
  adminDialogChromeContentClass,
  adminDialogChromeFooterClass,
  adminDialogChromeHeaderClass,
  adminDialogChromeHeaderSplitClass,
  adminDialogChromeTitleClass,
  adminDialogFooterActionsClass,
  adminDialogFormBodyClass,
  adminDialogHeaderClass,
  adminDialogHeaderContentClass,
  adminDialogHeaderSplitClass,
  adminDialogScrollableBodyClass,
  adminDialogTitleClass,
  adminOverlayMaskClass,
} from '../styling/uiClasses'

export type AdminDialogSize = DialogShellSize
export type AdminDialogContentLayout = 'form' | 'scrollable' | 'plain'

const props = withDefaults(
  defineProps<{
    visible: boolean
    header?: string | undefined
    ariaLabel?: string | undefined
    /** 可选的无障碍描述（reka DialogContent 警告需要 Description 或 aria-describedby）。 */
    description?: string | undefined
    size?: AdminDialogSize
    contentLayout?: AdminDialogContentLayout
    busy?: boolean
  }>(),
  {
    header: undefined,
    ariaLabel: undefined,
    description: undefined,
    size: 'form',
    contentLayout: 'form',
    busy: false,
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  close: []
  reset: []
}>()
const slots = useSlots()
const hasCustomHeader = computed(() => Boolean(slots.header))
// Title fallback keeps the dialog named when only a custom header slot exists.
const contentAriaLabel = computed(() =>
  hasCustomHeader.value ? (props.ariaLabel ?? props.header ?? '对话框') : undefined,
)

const dialogWidthClasses: Record<AdminDialogSize, string> = dialogShellSizeClasses

const dialogScrollableViewportClass = `${adminDialogScrollableBodyClass} max-h-[min(68dvh,40rem)]`
const contentLayoutClasses: Record<AdminDialogContentLayout, string> = {
  form: `${adminDialogFormBodyClass} ${dialogScrollableViewportClass}`,
  scrollable: dialogScrollableViewportClass,
  plain: 'flex flex-col',
}
const contentClass = computed(() => contentLayoutClasses[props.contentLayout])
const {
  hasAriaBusy,
  canDismiss,
  handleOpenUpdate,
  handleEscape,
  blockOutsideDismiss,
  requestCloseFromMask,
  requestCloseFromControl,
} = useAdminDialogInteraction({
  isVisible: () => props.visible,
  isBusy: () => props.busy,
  onReset: () => emit('reset'),
  onVisibilityChange: (visible) => {
    emit('update:visible', visible)
    if (!visible) emit('close')
  },
})
</script>
