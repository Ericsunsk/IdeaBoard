<template>
  <DialogShell
    :is-open="true"
    :title="props.ariaLabel ?? t('auth.signIn')"
    :description="props.description"
    :size="authPreset.size"
    :is-busy="props.busy"
    :has-close="authPreset.hasClose"
    :content-class="authDialogContentClass"
    @update:open="handleOpenUpdate"
    @open-auto-focus="handleOpenAutoFocus"
  >
    <div ref="panelRef" class="min-h-0 overflow-y-auto">
      <slot />
    </div>
  </DialogShell>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import DialogShell from '../../../core/components/DialogShell.vue'
import { dialogShellPresets } from '../../../core/components/dialogShell'
import { authDialogContentClass } from './modalUi'

const props = withDefaults(
  defineProps<{
    busy: boolean
    ariaLabel?: string | undefined
    description?: string | undefined
  }>(),
  { ariaLabel: undefined, description: undefined },
)

const emit = defineEmits<{
  close: []
}>()

const { t } = useI18n()
const authPreset = dialogShellPresets.auth

function handleOpenUpdate(open: boolean) {
  if (!open) emit('close')
}

/*
 * reka-ui autofocuses the first tabbable node in the dialog, which is the
 * trailing close action. Landing there puts a focus ring on a dismiss control
 * instead of the first field, so redirect autofocus to the first form control.
 */
const FIRST_FORM_CONTROL =
  'input:not([type="hidden"]):not([disabled]), select:not([disabled]), textarea:not([disabled])'

const panelRef = ref<HTMLElement | null>(null)

function handleOpenAutoFocus(event: Event) {
  const target = panelRef.value?.querySelector<HTMLElement>(FIRST_FORM_CONTROL)
  if (!target) return
  event.preventDefault()
  target.focus()
}
</script>
