<template>
  <Popover :open="open" @update:open="open = $event">
    <PopoverAnchor v-if="anchor" :reference="anchor" />
    <DropdownMenuItem as-child :class="appOverlayMenuItemClass" @select.prevent="show">
      <div
        ref="anchor"
        :aria-expanded="open"
        aria-haspopup="dialog"
        @pointermove="handlePointerMove"
        @pointerleave="handlePointerLeave"
        @focus="handleAnchorFocus"
        @blur="handleBlur"
        @keydown.enter.prevent="focusQr"
        @keydown.space.prevent="focusQr"
        @keydown.left.prevent="focusQr"
      >
        <span class="truncate flex-1">{{ channel.name }}</span>
        <NavArrowRight class="ml-auto size-4 shrink-0" />
      </div>
    </DropdownMenuItem>
    <PopoverContent
      :side="hasHorizontalSpace ? 'left' : 'bottom'"
      align="start"
      :side-offset="appOverlayMenuSideOffset"
      :collision-padding="appOverlayMenuSideOffset"
      prioritize-position
      :aria-label="t('auth.menu.supportQr', { channel: channel.name })"
      class="ib-user-account-menu-panel w-auto p-2 [direction:ltr]"
      @open-auto-focus.prevent
      @pointerenter="cancelClose"
      @pointerleave="handlePointerLeave"
      @close-auto-focus="handleCloseAutoFocus"
      @escape-key-down="handleEscape"
      @interact-outside="handleInteractOutside"
    >
      <div
        data-support-qr-panel
        :aria-label="t('auth.menu.supportQr', { channel: channel.name })"
        @pointerenter="cancelClose"
        @pointerleave="handlePointerLeave"
        @focusin="cancelClose"
        @focusout="handleBlur"
      >
        <a
          ref="link"
          :href="channel.url"
          target="_blank"
          rel="noopener noreferrer"
          :aria-label="t('auth.menu.supportOpenChannel', { channel: channel.name })"
          class="block bg-media-foreground p-2 rounded-control focus-visible:outline-2 focus-visible:outline-ring"
        >
          <QrcodeVue :value="channel.url" :size="176" level="M" render-as="svg" />
        </a>
      </div>
    </PopoverContent>
  </Popover>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, useTemplateRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMediaQuery } from '@vueuse/core'
import QrcodeVue from 'qrcode.vue'
import { NavArrowRight } from '@iconoir/vue'
import { DropdownMenuItem } from '@/components/ui/dropdown-menu'
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover'
import { appOverlayMenuItemClass, appOverlayMenuSideOffset } from '../../../../theme/appOverlayMenu'
import type { SupportChannel } from '@/api/supportChannels'

const props = defineProps<{ channel: SupportChannel }>()
const activeChannelId = defineModel<string | null>('activeChannelId', { default: null })
const { t } = useI18n()
const hasHorizontalSpace = useMediaQuery('(min-width: 640px)')
const open = computed({
  get: () => activeChannelId.value === props.channel.id,
  set: (value: boolean) => {
    if (value) activeChannelId.value = props.channel.id
    else if (activeChannelId.value === props.channel.id) activeChannelId.value = null
  },
})
const anchor = useTemplateRef('anchor')
const link = useTemplateRef('link')
let closeTimer: ReturnType<typeof setTimeout> | undefined
let restoreFocusOnClose = false
let suppressNextFocus = false
function cancelClose() {
  clearTimeout(closeTimer)
}
function show() {
  cancelClose()
  open.value = true
}
function handleAnchorFocus() {
  if (suppressNextFocus) {
    suppressNextFocus = false
    return
  }
  show()
}
function handleEscape(event: KeyboardEvent) {
  event.preventDefault()
  restoreFocusOnClose = true
  open.value = false
}
function handleCloseAutoFocus(event: Event) {
  event.preventDefault()
  if (!restoreFocusOnClose) return
  restoreFocusOnClose = false
  cancelClose()
  suppressNextFocus = true
  anchor.value?.focus()
  suppressNextFocus = false
}
function scheduleClose() {
  cancelClose()
  closeTimer = setTimeout(() => {
    open.value = false
  }, 250)
}
function handlePointerMove(event: PointerEvent) {
  if (event.pointerType === 'mouse') show()
}
function handlePointerLeave(event: PointerEvent) {
  if (event.pointerType === 'mouse') scheduleClose()
}
function handleBlur(event: FocusEvent) {
  const target = event.relatedTarget
  if (target instanceof Node && (anchor.value?.contains(target) || link.value?.contains(target)))
    return
  scheduleClose()
}
function handleInteractOutside(event: Event) {
  if (event.target instanceof Node && anchor.value?.contains(event.target)) event.preventDefault()
}
async function focusQr() {
  show()
  await nextTick()
  link.value?.focus()
}
onUnmounted(cancelClose)
</script>
