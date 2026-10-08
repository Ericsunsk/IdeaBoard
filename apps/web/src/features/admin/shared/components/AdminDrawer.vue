<template>
  <DialogRoot :open="visible" @update:open="handleOpenUpdate">
    <DialogPortal>
      <DialogOverlay
        data-admin-drawer-mask
        class="fixed inset-0 z-50 bg-black/40 backdrop-blur-xs data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
        @click="requestCloseFromMask"
      />
      <DialogContent
        :show-close="false"
        role="dialog"
        aria-modal="true"
        :aria-busy="ariaBusy"
        data-admin-drawer
        class="fixed inset-y-0 right-0 z-50 flex h-full w-[min(38rem,calc(100vw-1rem))] flex-col border-l border-border bg-card shadow-2xl transition-transform data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:slide-out-to-right data-[state=open]:slide-in-from-right"
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
            class="cursor-pointer rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
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
import {
  defineDialogSurfaceIntent,
  useOverlayInteractionLifecycle,
} from '../../../../shared/composables/useOverlayInteractionLifecycle'

// 右侧抽屉与居中弹窗是两种 overlay 形态（定位/滑入动画/chrome 不同），
// 独立成组件而不是给 AdminDialogShell 加 drawer 变体，避免分叉其尺寸与
// 布局契约；开关/遮罩/Escape/忙时锁定走同一 useOverlayInteractionLifecycle。
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

const adminDrawerSurfaceIntent = defineDialogSurfaceIntent()
const { ariaBusy, canDismiss, completeClose, handleVisibilityChange, requestClose } =
  useOverlayInteractionLifecycle({
    visible: () => props.visible,
    busy: () => props.busy,
    surface: adminDrawerSurfaceIntent,
    onReset: () => emit('reset'),
    onVisibilityChange: (visible) => {
      emit('update:visible', visible)
      if (!visible) {
        emit('close')
      }
    },
  })

// Focus is restored on a macrotask so a reopen racing the close wins and
// keeps focus inside the drawer instead of yanking it back to a stale trigger.
let restoreTimer: ReturnType<typeof setTimeout> | undefined
function scheduleFocusRestore() {
  cancelScheduledFocusRestore()
  restoreTimer = setTimeout(() => {
    restoreTimer = undefined
    completeClose()
  }, 0)
}
function cancelScheduledFocusRestore() {
  if (restoreTimer !== undefined) {
    clearTimeout(restoreTimer)
    restoreTimer = undefined
  }
}

function handleOpenUpdate(open: boolean) {
  if (open) {
    cancelScheduledFocusRestore()
    handleVisibilityChange(true)
    return
  }
  if (handleVisibilityChange(false)) scheduleFocusRestore()
}

function handleEscape(event: KeyboardEvent) {
  // Allowed Escape flows through reka's update:open above; only block it.
  if (!canDismiss('escape')) event.preventDefault()
}

// Mask dismissal follows the unified dialog contract (close-control, Escape,
// mask while idle; all paths locked while an irreversible commit is busy).
// Reka's outside dismiss stays blocked so overlay clicks only reach the
// explicit mask path gated by the lifecycle.
function blockOutsideDismiss(event: Event) {
  event.preventDefault()
}

function requestCloseFromMask() {
  requestClose('mask')
}

function requestCloseFromControl() {
  if (requestClose('close-control')) scheduleFocusRestore()
}
</script>
