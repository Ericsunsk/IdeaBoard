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
        class="ib-page-chrome-surface ib-control-state [--ib-control-active-transform:scale(0.97)] inline-flex h-9 items-center justify-center rounded-full px-4 text-xs font-semibold text-foreground transition-[border-color,background-color,transform,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-[var(--color-ib-page-chrome-border-hover)] hover:bg-[var(--color-ib-page-chrome-bg-hover)] active:scale-[0.97]"
        :aria-label="t('auth.signIn')"
        @click="loginItem?.command?.()"
      >
        {{ t('auth.signIn') }}
      </button>
      <button
        type="button"
        class="ib-control-state [--ib-control-hover-transform:scale(1.02)] [--ib-control-active-transform:scale(0.97)] inline-flex h-9 items-center justify-center rounded-full border-0 bg-gradient-to-r from-[var(--color-ib-auth-primary-from)] to-[var(--color-ib-auth-primary-to)] px-4.5 text-xs font-semibold text-[var(--color-ib-auth-primary-text)] shadow-ib-auth-primary transition-[box-shadow,transform,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:shadow-ib-auth-primary-hover active:scale-[0.97]"
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

          <DropdownMenuSub v-model:open="appearanceSubMenuOpen">
            <DropdownMenuSubTrigger
              :class="[appOverlayMenuItemClass, props.menuItemClass, 'cursor-pointer']"
              @click.stop="appearanceSubMenuOpen = !appearanceSubMenuOpen"
            >
              <span class="flex min-w-0 flex-1 items-center justify-between gap-2">
                <span class="truncate">{{ t('auth.menu.appearance') }}</span>
              </span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent
              :side-offset="6"
              class="w-36 rounded-xl border border-border bg-card p-1 text-foreground shadow-lg outline-none"
            >
              <DropdownMenuItem
                class="flex cursor-pointer items-center justify-between rounded-lg px-2.5 py-1.5 text-xs text-foreground transition-colors hover:bg-muted"
                @select="setTheme('light')"
                @click="setTheme('light')"
              >
                <span class="flex items-center gap-2">
                  <SunLight class="size-3.5 text-muted-foreground" />
                  <span>{{ t('auth.menu.themeLight') }}</span>
                </span>
                <Check v-if="mode === 'light'" class="size-3.5 text-primary" />
              </DropdownMenuItem>
              <DropdownMenuItem
                class="flex cursor-pointer items-center justify-between rounded-lg px-2.5 py-1.5 text-xs text-foreground transition-colors hover:bg-muted"
                @select="setTheme('dark')"
                @click="setTheme('dark')"
              >
                <span class="flex items-center gap-2">
                  <HalfMoon class="size-3.5 text-muted-foreground" />
                  <span>{{ t('auth.menu.themeDark') }}</span>
                </span>
                <Check v-if="mode === 'dark'" class="size-3.5 text-primary" />
              </DropdownMenuItem>
              <DropdownMenuItem
                class="flex cursor-pointer items-center justify-between rounded-lg px-2.5 py-1.5 text-xs text-foreground transition-colors hover:bg-muted"
                @select="setTheme('system')"
                @click="setTheme('system')"
              >
                <span class="flex items-center gap-2">
                  <Laptop class="size-3.5 text-muted-foreground" />
                  <span>{{ t('auth.menu.themeSystem') }}</span>
                </span>
                <Check v-if="mode === 'auto'" class="size-3.5 text-primary" />
              </DropdownMenuItem>
            </DropdownMenuSubContent>
          </DropdownMenuSub>

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
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
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
const appearanceSubMenuOpen = ref(false)

const menuContentClass = 'ib-user-account-menu-panel w-36 min-w-36 max-w-[10rem]'

const nonDangerItems = computed(() => props.items.filter((item) => item.tone !== 'danger'))
const dangerItems = computed(() => props.items.filter((item) => item.tone === 'danger'))

const loginItem = computed(() => props.items.find((item) => item.key === 'login'))
const registerItem = computed(() => props.items.find((item) => item.key === 'register'))
const isGuest = computed(() => Boolean(loginItem.value && registerItem.value))

const triggerRef = useTemplateRef('triggerRef')
const { handleMenuOpenUpdate, handlePointerDownOutside } = useMenuTriggerFocusRestore(triggerRef)
</script>
