<template>
  <div
    class="inline-flex shrink-0 items-center gap-2 leading-none"
    :class="props.wrapClass"
    @pointerdown.stop
    @mousedown.stop
    @click.stop
  >
    <template v-if="isGuest">
      <button
        type="button"
        class="page-chrome-surface control-state [--control-active-transform:scale(0.97)] inline-flex h-9 items-center justify-center rounded-full px-4 text-xs font-semibold text-foreground transition-[border-color,background-color,transform,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-[var(--color-page-chrome-border-hover)] hover:bg-[var(--color-page-chrome-bg-hover)] active:scale-[0.97]"
        :aria-label="t('auth.signIn')"
        @click="loginItem?.command?.()"
      >
        {{ t('auth.signIn') }}
      </button>
      <button
        type="button"
        class="control-state [--control-hover-transform:scale(1.02)] [--control-active-transform:scale(0.97)] inline-flex h-9 items-center justify-center rounded-full border-0 bg-gradient-to-r from-[var(--color-auth-primary-from)] to-[var(--color-auth-primary-to)] px-4.5 text-xs font-semibold text-[var(--color-auth-primary-text)] shadow-auth-primary transition-[box-shadow,transform,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:shadow-auth-primary-hover active:scale-[0.97]"
        :aria-label="t('auth.signUpFree')"
        @click="registerItem?.command?.()"
      >
        {{ t('auth.signUpFree') }}
      </button>
    </template>
    <template v-else>
      <UserCreditBalanceButton v-if="showCredits && balance" :balance="balance" />
      <slot name="actions" />
      <DropdownMenu
        v-model:open="open"
        dir="rtl"
        :modal="false"
        @update:open="handleMenuOpenUpdate"
      >
        <DropdownMenuTrigger ref="triggerRef" as-child>
          <button
            type="button"
            :title="t('auth.menu.title')"
            :aria-label="t('auth.menu.title')"
            :class="props.avatarButtonClass"
          >
            <img
              v-if="props.avatarUrl"
              :src="props.avatarUrl"
              :class="props.avatarImageClass"
              :alt="t('auth.menu.avatarAlt')"
            />
            <span v-else :class="[props.avatarPlaceholderClass, 'bg-transparent']">
              <User :class="props.avatarIconClass" />
            </span>
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="start"
          :side-offset="appOverlayMenuSideOffset"
          :collision-padding="appOverlayMenuSideOffset"
          :class="menuContentClass"
          @pointer-down-outside="handlePointerDownOutside"
          @focus-outside="keepSupportQrFocus"
        >
          <!-- 极简用户信息：展示当前登录账号邮箱 -->
          <template v-if="props.userEmail">
            <DropdownMenuLabel class="px-2.5 py-1.5 font-normal">
              <span class="block truncate text-xs text-muted-foreground">
                {{ props.userEmail }}
              </span>
            </DropdownMenuLabel>
            <DropdownMenuSeparator class="my-1" />
          </template>

          <DropdownMenuItem
            v-for="item in nonDangerItems"
            :key="item.key ?? item.label"
            :data-control-tone="item.tone ?? 'default'"
            :class="[appOverlayMenuItemClass, props.menuItemClass]"
            @select="item.command?.()"
          >
            <span class="flex min-w-0 flex-1 items-center justify-between gap-4">
              <span class="truncate">{{ item.label }}</span>
            </span>
          </DropdownMenuItem>

          <DropdownMenuSub>
            <DropdownMenuSubTrigger :class="[appOverlayMenuItemClass, props.menuItemClass]">
              <span class="truncate">{{ t('auth.menu.appearance') }}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent
              :side-offset="appOverlayMenuSideOffset"
              :class="submenuContentClass"
            >
              <DropdownMenuItem
                v-for="item in themeItems"
                :key="item.value"
                :class="[
                  appOverlayMenuItemClass,
                  props.menuItemClass,
                  item.isActive &&
                    'bg-muted text-[var(--app-overlay-menu-item-hover-color,var(--foreground))] font-medium',
                ]"
                :data-control-state="item.isActive ? 'active' : 'idle'"
                :aria-current="item.isActive ? 'true' : undefined"
                @select="setTheme(item.value)"
              >
                <component :is="item.icon" class="size-4 shrink-0 mr-2" />
                <span class="truncate flex-1">{{ item.label }}</span>
                <Check v-if="item.isActive" class="size-3.5 shrink-0 ml-auto" />
              </DropdownMenuItem>
            </DropdownMenuSubContent>
          </DropdownMenuSub>

          <DropdownMenuSub>
            <DropdownMenuSubTrigger :class="[appOverlayMenuItemClass, props.menuItemClass]">
              <span class="truncate">{{ t('auth.menu.language') }}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent
              :side-offset="appOverlayMenuSideOffset"
              :class="submenuContentClass"
            >
              <DropdownMenuItem
                v-for="item in localeItems"
                :key="item.value"
                :class="[
                  appOverlayMenuItemClass,
                  props.menuItemClass,
                  item.isActive &&
                    'bg-muted text-[var(--app-overlay-menu-item-hover-color,var(--foreground))] font-medium',
                ]"
                :data-control-state="item.isActive ? 'active' : 'idle'"
                :aria-current="item.isActive ? 'true' : undefined"
                @select="handleLocaleSelect(item.value)"
              >
                <span class="truncate flex-1">{{ item.label }}</span>
                <Check v-if="item.isActive" class="size-3.5 shrink-0 ml-auto" />
              </DropdownMenuItem>
            </DropdownMenuSubContent>
          </DropdownMenuSub>

          <DropdownMenuSub
            v-if="supportChannels.length"
            @update:open="!$event && (activeSupportChannelId = null)"
          >
            <DropdownMenuSubTrigger :class="[appOverlayMenuItemClass, props.menuItemClass]">
              <span class="truncate">{{ t('auth.menu.support') }}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent
              :side-offset="appOverlayMenuSideOffset"
              :collision-padding="appOverlayMenuSideOffset"
              :class="submenuContentClass"
              @focus-outside="keepSupportQrFocus"
            >
              <SupportChannelQr
                v-for="channel in supportChannels"
                :key="channel.id"
                v-model:active-channel-id="activeSupportChannelId"
                :channel="channel"
              />
            </DropdownMenuSubContent>
          </DropdownMenuSub>

          <DropdownMenuSeparator class="my-1" />

          <DropdownMenuItem
            v-for="item in dangerItems"
            :key="item.key ?? item.label"
            data-control-tone="danger"
            :class="[
              appOverlayMenuItemClass,
              props.menuItemClass,
              appOverlayMenuDangerItemClass,
              props.menuDangerItemClass,
            ]"
            @select="item.command?.()"
          >
            <span class="flex min-w-0 flex-1 items-center justify-between gap-4">
              <span class="truncate">{{ item.label }}</span>
            </span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, useTemplateRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, HalfMoon, Laptop, SunLight, User } from '@iconoir/vue'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  appOverlayMenuDangerItemClass,
  appOverlayMenuItemClass,
  appOverlayMenuSideOffset,
} from '../../../../theme/appOverlayMenu'
import { pageChromeAccountMenuClasses } from '../../../../core/lib/pageChrome'
import { useMenuTriggerFocusRestore } from '../../../../core/composables/useMenuTriggerFocusRestore'
import { useTheme } from '../../../../theme/composables/useTheme'
import {
  getActiveUserLocale,
  setUserLocale,
  type SupportedLocale,
} from '../../../../core/i18n/userLocale'
import { LOCALE_MANIFEST } from '../../../../core/i18n/localeManifest'
import type { SupportChannel } from '../../../../core/composables/useSupportChannels'
import SupportChannelQr from './SupportChannelQr.vue'
import UserCreditBalanceButton from './UserCreditBalanceButton.vue'
import type { UserAccountMenuItem, UserIdeaBalanceControl } from './userAccountMenu.types'

const props = withDefaults(
  defineProps<{
    avatarUrl?: string | undefined
    userEmail?: string | undefined
    items: UserAccountMenuItem[]
    balance?: UserIdeaBalanceControl | undefined
    wrapClass?: string
    avatarButtonClass?: string
    avatarImageClass?: string
    avatarPlaceholderClass?: string
    avatarIconClass?: string
    menuItemClass?: string
    menuDangerItemClass?: string
    showCredits?: boolean
    supportChannels?: SupportChannel[]
  }>(),
  {
    avatarUrl: undefined,
    userEmail: undefined,
    balance: undefined,
    wrapClass: pageChromeAccountMenuClasses.wrap,
    avatarButtonClass: pageChromeAccountMenuClasses.avatarButton,
    avatarImageClass: pageChromeAccountMenuClasses.avatarImage,
    avatarPlaceholderClass: pageChromeAccountMenuClasses.avatarPlaceholder,
    avatarIconClass: pageChromeAccountMenuClasses.avatarIcon,
    menuItemClass: appOverlayMenuItemClass,
    menuDangerItemClass: appOverlayMenuDangerItemClass,
    showCredits: true,
    supportChannels: () => [],
  },
)

const open = defineModel<boolean>('open', { default: false })
const activeSupportChannelId = ref<string | null>(null)
const { locale: i18nLocale, t } = useI18n()
const { mode, setTheme } = useTheme()

const menuContentClass = 'ib-user-account-menu-panel w-auto min-w-36 max-w-56 [direction:ltr]'
const submenuContentClass =
  'ib-user-account-menu-panel w-32 min-w-32 max-w-[8.5rem] [direction:ltr]'

const themeItems = computed(() => [
  {
    value: 'light' as const,
    label: t('auth.menu.themeLight'),
    icon: SunLight,
    isActive: mode.value === 'light',
  },
  {
    value: 'dark' as const,
    label: t('auth.menu.themeDark'),
    icon: HalfMoon,
    isActive: mode.value === 'dark',
  },
  {
    value: 'system' as const,
    label: t('auth.menu.themeSystem'),
    icon: Laptop,
    isActive: mode.value === 'auto',
  },
])

const activeLocale = computed<SupportedLocale>(() => {
  const current = i18nLocale.value as SupportedLocale
  return current || getActiveUserLocale()
})

const localeItems = computed(() =>
  (Object.keys(LOCALE_MANIFEST) as SupportedLocale[]).map((locale) => ({
    value: locale,
    label: LOCALE_MANIFEST[locale].displayLabel,
    isActive: locale === activeLocale.value,
  })),
)

async function handleLocaleSelect(locale: SupportedLocale) {
  if (locale === activeLocale.value) return
  await setUserLocale(locale)
}

const nonDangerItems = computed(() => props.items.filter((item) => item.tone !== 'danger'))
const dangerItems = computed(() => props.items.filter((item) => item.tone === 'danger'))

const loginItem = computed(() => props.items.find((item) => item.key === 'login'))
const registerItem = computed(() => props.items.find((item) => item.key === 'register'))
const isGuest = computed(() => Boolean(loginItem.value && registerItem.value))

function keepSupportQrFocus(event: Event) {
  if (event.target instanceof Element && event.target.closest('[data-support-qr-panel]'))
    event.preventDefault()
}

const triggerRef = useTemplateRef('triggerRef')
const { handleMenuOpenUpdate, handlePointerDownOutside } = useMenuTriggerFocusRestore(triggerRef)

defineExpose({
  closeMenu: () => {
    open.value = false
  },
})
</script>
