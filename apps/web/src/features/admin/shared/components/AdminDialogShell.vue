<template>
  <Dialog
    ref="dialogRef"
    :visible="visible"
    modal
    :dismissable-mask="canDismiss('mask')"
    :close-on-escape="canDismiss('escape')"
    :closable="canDismiss('close-control')"
    :draggable="false"
    :header="header"
    :pt="dialogPt"
    :aria-busy="ariaBusy"
    v-bind="dialogAccessibilityAttrs"
    @update:visible="handleVisibleUpdate"
    @hide="disableVendorFocusRestore"
    @after-hide="completeClose"
  >
    <template v-if="$slots.header" #header>
      <slot name="header" />
    </template>

    <template #closeicon>
      <Xmark class="!h-5 !w-5" data-icon="xmark" aria-hidden="true" />
    </template>

    <div :class="contentClass">
      <slot />
    </div>

    <template v-if="$slots.footer" #footer>
      <div :class="adminDialogFooterActionsClass" data-admin-dialog-footer>
        <slot name="footer" :request-close="requestCloseFromControl" />
      </div>
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, useSlots } from 'vue'
import { Xmark } from '@iconoir/vue'
import Dialog from 'primevue/dialog'
import { usePrimeDialogOverlayLifecycle } from '../../../../shared/composables/usePrimeDialogOverlayLifecycle'
import { createAdminDialogPt } from '../lib/overlayPt'
import {
  adminDialogContentClass,
  adminDialogFooterClass,
  adminDialogFooterActionsClass,
  adminDialogFormBodyClass,
  adminDialogHeaderClass,
  adminDialogScrollableBodyClass,
} from '../lib/uiClasses'

export type AdminDialogSize = 'compact' | 'form' | 'medium' | 'wide'
export type AdminDialogContentLayout = 'form' | 'scrollable'

const props = withDefaults(
  defineProps<{
    visible: boolean
    header?: string | undefined
    ariaLabel?: string | undefined
    size?: AdminDialogSize
    contentLayout?: AdminDialogContentLayout
    busy?: boolean
  }>(),
  {
    header: undefined,
    ariaLabel: undefined,
    size: 'form',
    contentLayout: 'form',
    busy: false,
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  reset: []
}>()
const slots = useSlots()
const hasCustomHeader = computed(() => Boolean(slots.header))
const dialogAccessibilityAttrs = computed(() =>
  hasCustomHeader.value
    ? {
        'aria-label': props.ariaLabel,
        'aria-labelledby': null,
      }
    : {},
)

const dialogWidthClasses: Record<AdminDialogSize, string> = {
  compact: 'w-[min(30rem,calc(100vw-1.5rem))]',
  form: 'w-[min(35rem,calc(100vw-1.5rem))]',
  medium: 'w-[min(46rem,calc(100vw-1.5rem))]',
  wide: 'w-[min(61.25rem,calc(100vw-1.5rem))]',
}

const dialogScrollableViewportClass = `${adminDialogScrollableBodyClass} max-h-[min(68dvh,40rem)]`
const contentLayoutClasses: Record<AdminDialogContentLayout, string> = {
  form: `${adminDialogFormBodyClass} ${dialogScrollableViewportClass}`,
  scrollable: dialogScrollableViewportClass,
}

const dialogPt = computed(() =>
  createAdminDialogPt({
    rootClass: dialogWidthClasses[props.size],
    headerClass: adminDialogHeaderClass,
    contentClass: adminDialogContentClass,
    footerClass: adminDialogFooterClass,
  }),
)
const contentClass = computed(() => contentLayoutClasses[props.contentLayout])
const adminDialogSurfaceIntent = {
  dismissibleBy: ['close-control', 'escape'],
  restoreFocus: true,
} as const
const {
  ariaBusy,
  canDismiss,
  completeClose,
  handleVisibilityChange,
  requestClose: requestOverlayClose,
  dialogRef,
  disableVendorFocusRestore,
} = usePrimeDialogOverlayLifecycle({
  visible: () => props.visible,
  busy: () => props.busy,
  surface: adminDialogSurfaceIntent,
  onReset: () => emit('reset'),
  onVisibilityChange: (visible) => emit('update:visible', visible),
})

function handleVisibleUpdate(visible: boolean) {
  handleVisibilityChange(visible)
}

function requestCloseFromControl() {
  requestOverlayClose('close-control')
}
</script>
