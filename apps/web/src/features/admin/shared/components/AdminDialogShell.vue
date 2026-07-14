<template>
  <Dialog
    :visible="visible"
    modal
    :dismissable-mask="false"
    :close-on-escape="!busy"
    :closable="!busy"
    :draggable="false"
    :header="header"
    :pt="dialogPt"
    :aria-busy="busy || undefined"
    v-bind="dialogAccessibilityAttrs"
    @update:visible="handleVisibleUpdate"
  >
    <template v-if="$slots.header" #header>
      <slot name="header" />
    </template>

    <div :class="contentClass">
      <slot />
    </div>

    <template v-if="$slots.footer" #footer>
      <div :class="adminDialogFooterActionsClass" data-admin-dialog-footer>
        <slot name="footer" />
      </div>
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, useSlots } from 'vue'
import Dialog from 'primevue/dialog'
import { createAdminDialogPt } from '../lib/adminOverlayPt'
import {
  adminDialogFooterActionsClass,
  adminDialogFormBodyClass,
  adminDialogScrollableBodyClass,
} from '../lib/adminUiClasses'

export type AdminDialogSize = 'compact' | 'form' | 'wide'
export type AdminDialogContentLayout = 'form' | 'scrollable'

const props = withDefaults(
  defineProps<{
    visible: boolean
    header?: string
    ariaLabel?: string
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
  }),
)
const contentClass = computed(() => contentLayoutClasses[props.contentLayout])

function handleVisibleUpdate(visible: boolean) {
  if (!visible && props.busy) return
  emit('update:visible', visible)
}
</script>
