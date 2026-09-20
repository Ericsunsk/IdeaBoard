<template>
  <DropdownMenu
    :modal="false"
    :disabled="props.disabled || isSwitching"
    @update:open="handleMenuOpenUpdate"
  >
    <DropdownMenuTrigger ref="triggerRef" as-child>
      <button
        type="button"
        :title="selectLanguageLabel"
        :aria-label="selectLanguageLabel"
        :class="[localeTriggerClass, props.wrapClass]"
        :disabled="props.disabled || isSwitching"
        :aria-busy="isSwitching"
        data-testid="user-locale-select"
      >
        <Translate
          class="size-5 shrink-0 text-current"
          aria-hidden="true"
          data-testid="icon-translate"
        />
      </button>
    </DropdownMenuTrigger>
    <DropdownMenuContent
      align="end"
      :class="menuContentClass"
      @pointer-down-outside="handlePointerDownOutside"
    >
      <DropdownMenuItem
        v-for="item in localeMenuItems"
        :key="item.value"
        :class="[
          menuItemClass,
          item.value === activeLocale && 'bg-ib-surface-hover text-[var(--app-overlay-menu-item-hover-color,var(--color-ib-text))] font-medium',
        ]"
        :data-control-state="item.value === activeLocale ? 'active' : 'idle'"
        :aria-current="item.value === activeLocale ? 'true' : undefined"
        @select="handleMenuSelect(item)"
      >
        <span class="flex min-w-0 flex-1 items-center justify-between gap-4">
          <span class="truncate">{{ item.label }}</span>
        </span>
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>

<script setup lang="ts">
import { computed, ref, useTemplateRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { Translate } from '@iconoir/vue'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { createPageChromeAccountRoundControlClass } from '../lib/pageChrome'
import { appOverlayMenuItemClass } from '../theme/appOverlayMenu'
import { getActiveUserLocale, setUserLocale, type SupportedLocale } from '../i18n/userLocale'
import { LOCALE_MANIFEST } from '../i18n/localeManifest'
import { useMenuTriggerFocusRestore } from '../composables/useMenuTriggerFocusRestore'

interface UserLocaleSelectProps {
  wrapClass?: string
  disabled?: boolean
}

const props = withDefaults(defineProps<UserLocaleSelectProps>(), {
  wrapClass: '',
  disabled: false,
})

const emit = defineEmits<{
  'update:modelValue': [locale: SupportedLocale]
  change: [locale: SupportedLocale]
}>()

const { locale: i18nLocale, t } = useI18n()
const isSwitching = ref(false)
const localeTriggerClass = createPageChromeAccountRoundControlClass('select-none')
const localeOptions = (Object.keys(LOCALE_MANIFEST) as SupportedLocale[]).map((locale) => ({
  value: locale,
  label: LOCALE_MANIFEST[locale].displayLabel,
}))

const menuContentClass = 'ib-user-account-menu-panel w-32 min-w-32 max-w-[8rem]'

const activeLocale = computed<SupportedLocale>(() => {
  const current = i18nLocale.value as SupportedLocale
  return current || getActiveUserLocale()
})

const selectLanguageLabel = computed(() => t('common.locale.selectLanguage'))

const menuItemClass = appOverlayMenuItemClass

type LocaleItem = { label: string; value: SupportedLocale }

const localeMenuItems = computed<LocaleItem[]>(() =>
  localeOptions.map((opt) => ({
    label: opt.label,
    value: opt.value,
  })),
)

function handleMenuSelect(item: LocaleItem) {
  if (typeof item.value === 'string') void handleLocaleChange(item.value)
}

const triggerRef = useTemplateRef('triggerRef')
const { handleMenuOpenUpdate, handlePointerDownOutside } =
  useMenuTriggerFocusRestore(triggerRef)

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
