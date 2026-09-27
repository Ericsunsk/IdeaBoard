<template>
  <DialogRoot :open="visible" @update:open="handleOpenUpdate">
    <DialogPortal>
      <DialogOverlay
        data-admin-dialog-mask
        class="fixed inset-0 z-50 bg-black/40 backdrop-blur-xs data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
        @click="requestCloseFromMask"
      />
      <DialogContent
        role="dialog"
        aria-modal="true"
        :aria-label="contentAriaLabel"
        :aria-busy="ariaBusy"
        :data-size="props.size ?? 'form'"
        :class="[
          'fixed top-[50%] left-[50%] z-50 grid max-h-[calc(100dvh-1.5rem)] translate-x-[-50%] translate-y-[-50%] gap-0 overflow-hidden border shadow-xl data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95',
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
                '!bg-transparent !shadow-none inline-flex size-8 shrink-0 items-center justify-center rounded-lg border-0 p-0 hover:!bg-transparent focus:!outline-none focus-visible:!outline-none focus-visible:!shadow-none focus-visible:!ring-0 cursor-pointer transition-colors font-[650]',
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
import { useOverlayInteractionLifecycle } from '../../../../shared/composables/useOverlayInteractionLifecycle'
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
} from '../lib/uiClasses'

export type AdminDialogSize = 'slim' | 'compact' | 'form' | 'medium' | 'wide'
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

const dialogWidthClasses: Record<AdminDialogSize, string> = {
  slim: 'w-[min(23rem,calc(100vw-1.5rem))] max-w-[23rem]',
  compact: 'w-[min(30rem,calc(100vw-1.5rem))] max-w-[30rem]',
  form: 'w-[min(35rem,calc(100vw-1.5rem))] max-w-[35rem]',
  medium: 'w-[min(46rem,calc(100vw-1.5rem))] max-w-[46rem]',
  wide: 'w-[min(61.25rem,calc(100vw-1.5rem))] max-w-[61.25rem]',
}

const dialogScrollableViewportClass = `${adminDialogScrollableBodyClass} max-h-[min(68dvh,40rem)]`
const contentLayoutClasses: Record<AdminDialogContentLayout, string> = {
  form: `${adminDialogFormBodyClass} ${dialogScrollableViewportClass}`,
  scrollable: dialogScrollableViewportClass,
  plain: 'flex flex-col',
}
const contentClass = computed(() => contentLayoutClasses[props.contentLayout])
const adminDialogSurfaceIntent = {
  dismissibleBy: ['close-control', 'escape', 'mask'],
  restoreFocus: true,
} as const
const { ariaBusy, canDismiss, completeClose, handleVisibilityChange, requestClose } =
  useOverlayInteractionLifecycle({
    visible: () => props.visible,
    busy: () => props.busy,
    surface: adminDialogSurfaceIntent,
    onReset: () => emit('reset'),
    onVisibilityChange: (visible) => {
      emit('update:visible', visible)
      if (!visible) {
        emit('close')
      }
    },
  })

// Focus is restored on a macrotask so a reopen racing the close wins and
// keeps focus inside the dialog instead of yanking it back to a stale trigger.
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

// Mask dismissal is never in the surface intent: block reka's outside
// dismiss so overlay clicks only reach the explicit (blocked) mask path.
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
