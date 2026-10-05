<template>
  <div
    class="inline-flex shrink-0 items-center gap-2 leading-none"
    :class="props.wrapClass"
    @pointerdown.stop
    @mousedown.stop
    @click.stop
  >
    <slot name="before-balance" />
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
      <DropdownMenu :modal="false" @update:open="handleMenuOpenUpdate">
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
          align="end"
          :side-offset="appOverlayMenuSideOffset"
          :class="menuContentClass"
          @pointer-down-outside="handlePointerDownOutside"
        >
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

          <DropdownMenuSeparator class="my-1" />

          <div class="px-1 py-0.5">
            <div
              role="radiogroup"
              :aria-label="t('auth.menu.appearance')"
              @keydown="handleThemeKeydown"
              class="flex h-8 w-full items-center rounded-lg border border-border bg-muted/60 p-0.5"
            >
              <button
                type="button"
                role="radio"
                :aria-checked="mode === 'light'"
                :tabindex="mode === 'light' ? 0 : -1"
                :title="t('auth.menu.themeLight')"
                :aria-label="t('auth.menu.themeLight')"
                class="flex flex-1 h-full items-center justify-center rounded-md text-xs transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                :class="
                  mode === 'light'
                    ? 'bg-card font-semibold text-foreground shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                "
                @click.stop="setTheme('light')"
              >
                <SunLight class="size-4" />
              </button>
              <button
                type="button"
                role="radio"
                :aria-checked="mode === 'dark'"
                :tabindex="mode === 'dark' ? 0 : -1"
                :title="t('auth.menu.themeDark')"
                :aria-label="t('auth.menu.themeDark')"
                class="flex flex-1 h-full items-center justify-center rounded-md text-xs transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                :class="
                  mode === 'dark'
                    ? 'bg-card font-semibold text-foreground shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                "
                @click.stop="setTheme('dark')"
              >
                <HalfMoon class="size-4" />
              </button>
              <button
                type="button"
                role="radio"
                :aria-checked="mode === 'auto'"
                :tabindex="mode === 'auto' ? 0 : -1"
                :title="t('auth.menu.themeSystem')"
                :aria-label="t('auth.menu.themeSystem')"
                class="flex flex-1 h-full items-center justify-center rounded-md text-xs transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                :class="
                  mode === 'auto'
                    ? 'bg-card font-semibold text-foreground shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                "
                @click.stop="setTheme('system')"
              >
                <Laptop class="size-4" />
              </button>
            </div>
          </div>

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
import { computed, useTemplateRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { HalfMoon, Laptop, SunLight, User } from '@iconoir/vue'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  appOverlayMenuDangerItemClass,
  appOverlayMenuItemClass,
  appOverlayMenuSideOffset,
} from '../theme/appOverlayMenu'
import { pageChromeAccountMenuClasses } from '../lib/pageChrome'
import { useMenuTriggerFocusRestore } from '../composables/useMenuTriggerFocusRestore'
import { useTheme } from '../theme/composables/useTheme'
import UserCreditBalanceButton from './UserCreditBalanceButton.vue'
import type { UserAccountMenuItem, UserIdeaBalanceControl } from './userAccountMenu.types'

const props = withDefaults(
  defineProps<{
    avatarUrl?: string | undefined
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
  }>(),
  {
    avatarUrl: undefined,
    balance: undefined,
    wrapClass: pageChromeAccountMenuClasses.wrap,
    avatarButtonClass: pageChromeAccountMenuClasses.avatarButton,
    avatarImageClass: pageChromeAccountMenuClasses.avatarImage,
    avatarPlaceholderClass: pageChromeAccountMenuClasses.avatarPlaceholder,
    avatarIconClass: pageChromeAccountMenuClasses.avatarIcon,
    menuItemClass: appOverlayMenuItemClass,
    menuDangerItemClass: appOverlayMenuDangerItemClass,
    showCredits: true,
  },
)

const { t } = useI18n()
const { mode, setTheme } = useTheme()

const themeModes = ['light', 'dark', 'auto'] as const

function handleThemeKeydown(event: KeyboardEvent) {
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const current = themeModes.indexOf(mode.value)
  const next =
    event.key === 'Home'
      ? 0
      : event.key === 'End'
        ? 2
        : (current + (event.key === 'ArrowRight' ? 1 : 2)) % 3
  setTheme(themeModes[next] ?? 'auto')
  requestAnimationFrame(() => {
    const group = event.currentTarget as HTMLElement
    ;(group.querySelectorAll('[role="radio"]')[next] as HTMLElement | undefined)?.focus()
  })
}

const menuContentClass = 'ib-user-account-menu-panel w-36 min-w-36 max-w-[10rem]'

const nonDangerItems = computed(() => props.items.filter((item) => item.tone !== 'danger'))
const dangerItems = computed(() => props.items.filter((item) => item.tone === 'danger'))

const loginItem = computed(() => props.items.find((item) => item.key === 'login'))
const registerItem = computed(() => props.items.find((item) => item.key === 'register'))
const isGuest = computed(() => Boolean(loginItem.value && registerItem.value))

const triggerRef = useTemplateRef('triggerRef')
const { handleMenuOpenUpdate, handlePointerDownOutside } = useMenuTriggerFocusRestore(triggerRef)
</script>
