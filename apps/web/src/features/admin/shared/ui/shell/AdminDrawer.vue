<template>
  <DialogRoot :open="visible" @update:open="handleOpenUpdate">
    <DialogPortal>
      <DialogOverlay
        data-admin-drawer-mask
        :class="adminOverlayMaskClass"
        @click="requestCloseFromMask"
      />
      <DialogContent
        :show-close="false"
        role="dialog"
        aria-modal="true"
        :aria-busy="hasAriaBusy"
        data-admin-drawer
        class="fixed inset-y-0 right-0 z-admin-dialog flex h-full w-admin-drawer flex-col border-l border-border bg-popover shadow-admin-drawer overlay-drawer-motion"
        @escape-key-down="handleEscape"
        @pointer-down-outside="blockOutsideDismiss"
      >
        <div
          data-admin-drawer-region="header"
          class="flex items-center justify-between border-b border-border px-6 py-4"
        >
          <DialogTitle class="text-admin-title-sm font-bold text-foreground">
            {{ title }}
          </DialogTitle>
          <DialogDescription v-if="description" class="sr-only">
            {{ description }}
          </DialogDescription>
          <DialogClose
            v-if="canDismiss('close-control')"
            class="cursor-pointer rounded-control p-1.5 text-muted-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted hover:text-foreground"
            :aria-label="closeLabel"
          >
            <Xmark class="size-5" aria-hidden="true" />
          </DialogClose>
        </div>

        <div data-admin-drawer-region="content" class="flex-1 overflow-y-auto">
          <slot />
        </div>

        <div
          v-if="$slots.footer"
          data-admin-drawer-region="footer"
          class="border-t border-border px-6 py-4"
        >
          <slot name="footer" :request-close="requestCloseFromControl" />
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup lang="ts">
import { Xmark } from '@iconoir/vue'
import {
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from 'reka-ui'
import { useAdminDialogInteraction } from '../../session/overlay/useAdminDialogInteraction'
import { adminOverlayMaskClass } from '../../core/uiClasses'

const props = withDefaults(
  defineProps<{
    visible: boolean
    title: string
    /** 可选的无障碍描述（reka DialogContent 警告需要 Description 或 aria-describedby）。 */
    description?: string | undefined
    closeLabel?: string
    busy?: boolean
  }>(),
  {
    description: undefined,
    closeLabel: '关闭',
    busy: false,
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  close: []
  reset: []
}>()

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
