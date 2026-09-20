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
        class="ib-page-chrome-surface ib-control-state [--ib-control-active-transform:scale(0.97)] inline-flex h-9 items-center justify-center rounded-full px-4 text-xs font-semibold text-ib-text transition-[border-color,background-color,transform,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-[var(--color-ib-page-chrome-border-hover)] hover:bg-[var(--color-ib-page-chrome-bg-hover)] active:scale-[0.97]"
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
          :class="menuContentClass"
          @pointer-down-outside="handlePointerDownOutside"
        >
          <DropdownMenuItem
            v-for="item in props.items"
            :key="item.key ?? item.label"
            :data-control-tone="item.tone ?? 'default'"
            :class="[
              appOverlayMenuItemClass,
              props.menuItemClass,
              item.tone === 'danger' && [appOverlayMenuDangerItemClass, props.menuDangerItemClass],
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
import { User } from '@iconoir/vue'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { appOverlayMenuDangerItemClass, appOverlayMenuItemClass } from '../theme/appOverlayMenu'
import { pageChromeAccountMenuClasses } from '../lib/pageChrome'
import { useMenuTriggerFocusRestore } from '../composables/useMenuTriggerFocusRestore'
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

const menuContentClass = 'ib-user-account-menu-panel w-32 min-w-32 max-w-[8rem]'

const loginItem = computed(() => props.items.find((item) => item.key === 'login'))
const registerItem = computed(() => props.items.find((item) => item.key === 'register'))
const isGuest = computed(() => Boolean(loginItem.value && registerItem.value))

const triggerRef = useTemplateRef('triggerRef')
const { handleMenuOpenUpdate, handlePointerDownOutside } =
  useMenuTriggerFocusRestore(triggerRef)
</script>
