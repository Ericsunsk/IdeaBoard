<template>
  <Button
    ref="triggerRef"
    unstyled
    type="button"
    :title="selectLanguageLabel"
    :aria-label="selectLanguageLabel"
    aria-haspopup="menu"
    :aria-expanded="isOpen"
    :class="[localeTriggerClass, wrapClass]"
    :disabled="disabled || isSwitching"
    :aria-busy="isSwitching"
    data-testid="user-locale-select"
    @click="toggle"
  >
    <Translate
      class="size-5 shrink-0 text-current"
      aria-hidden="true"
      data-testid="icon-translate"
    />
  </Button>
  <Menu
    ref="menuRef"
    :model="localeMenuItems"
    popup
    :append-to="appendTo"
    :pt="userAccountMenuPt"
    @show="handleShow"
    @hide="handleHide"
  >
    <template #item="{ item, props: itemProps }">
      <a
        v-bind="itemProps.action"
        :class="[
          menuItemClass,
          item.value === activeLocale
            ? 'bg-ib-surface-hover text-[var(--app-overlay-menu-item-hover-color,var(--color-ib-text))]'
            : '',
        ]"
        :aria-current="item.value === activeLocale ? 'true' : undefined"
        :data-control-state="item.value === activeLocale ? 'active' : 'idle'"
        :data-control-tone="item.tone"
      >
        <span class="flex min-w-0 flex-1 items-center justify-between gap-4">
          <span class="truncate">{{ item.label }}</span>
        </span>
      </a>
    </template>
  </Menu>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Menu from 'primevue/menu'
import type { MenuItem } from 'primevue/menuitem'
import { Translate } from '@iconoir/vue'
import { createPageChromeAccountRoundControlClass } from '../lib/pageChrome'
import { appOverlayMenuItemClass } from '../theme/appOverlayMenu'
import { userAccountMenuPt } from '../theme/userAccountMenuSkin'
import { usePrimePopupMenuOverlayLifecycle } from '../composables/usePrimePopupMenuOverlayLifecycle'
import {
  getActiveUserLocale,
  setUserLocale,
  type SupportedLocale,
} from '../i18n/userLocale'
import { LOCALE_MANIFEST } from '../i18n/localeManifest'

interface UserLocaleSelectProps {
  wrapClass?: string
  appendTo?: string
  disabled?: boolean
}

withDefaults(defineProps<UserLocaleSelectProps>(), {
  wrapClass: '',
  appendTo: 'body',
  disabled: false,
})

const emit = defineEmits<{
  'update:modelValue': [locale: SupportedLocale]
  change: [locale: SupportedLocale]
}>()

const { locale: i18nLocale, t } = useI18n()
const isSwitching = ref(false)
const triggerRef = ref<HTMLElement | { $el?: HTMLElement } | null>(null)
const localeTriggerClass = createPageChromeAccountRoundControlClass('select-none')
const localeOptions = (Object.keys(LOCALE_MANIFEST) as SupportedLocale[]).map((locale) => ({
  value: locale,
  label: LOCALE_MANIFEST[locale].displayLabel,
}))

const {
  popupRef: menuRef,
  isOpen,
  toggle: toggleMenuOverlay,
  handleShow,
  handleHide,
} = usePrimePopupMenuOverlayLifecycle({})

function resolveTriggerElement(): HTMLElement | undefined {
  const button = triggerRef.value
  if (!button) return undefined
  return button instanceof HTMLElement ? button : button.$el
}

function toggle(event: Event) {
  toggleMenuOverlay(event, resolveTriggerElement())
}

const activeLocale = computed<SupportedLocale>(() => {
  const current = i18nLocale.value as SupportedLocale
  return current || getActiveUserLocale()
})

const selectLanguageLabel = computed(() => t('common.locale.selectLanguage'))

const menuItemClass = appOverlayMenuItemClass

const localeMenuItems = computed<MenuItem[]>(() =>
  localeOptions.map((opt) => ({
    label: opt.label,
    value: opt.value,
    command: () => {
      void handleLocaleChange(opt.value)
    },
  })),
)

async function handleLocaleChange(newLocale: SupportedLocale) {
  if (!newLocale || newLocale === activeLocale.value || isSwitching.value) return
  isSwitching.value = true
  try {
    const success = await setUserLocale(newLocale)
    if (success) {
      emit('update:modelValue', newLocale)
      emit('change', newLocale)
    }
  } finally {
    isSwitching.value = false
  }
}
</script>
