<template>
  <AccountPane :title="t('account.tabs.profile')">
    <section class="min-w-0" :aria-label="t('account.profile.overviewAria')">
      <div class="flex min-w-0 items-start gap-5 sm:items-center">
        <div
          class="relative size-20 shrink-0 overflow-visible rounded-pill bg-card shadow-sm sm:size-[88px]"
        >
          <button
            type="button"
            class="group relative flex h-full w-full cursor-pointer items-center justify-center overflow-hidden rounded-pill border-0 bg-transparent p-0 text-primary-foreground outline-none transition-shadow duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] focus-visible:shadow-[0_0_0_3px_var(--color-ring)] disabled:cursor-wait"
            :disabled="avatarUploading"
            :aria-busy="avatarUploading"
            :aria-label="t('account.profile.changeAvatar')"
            :title="t('account.profile.changeAvatar')"
            @click="openAvatarPicker"
          >
            <Avatar class="h-full w-full">
              <AvatarImage
                v-if="userStore.user?.avatar"
                :src="userStore.user.avatar"
                :alt="userStore.user.display_name"
                class="rounded-full object-cover"
              />
              <AvatarFallback
                class="bg-primary text-admin-summary-value font-bold text-primary-foreground sm:text-[34px]"
              >
                {{ userInitial }}
              </AvatarFallback>
            </Avatar>
            <span
              class="pointer-events-none absolute inset-0 flex items-center justify-center rounded-pill bg-[color-mix(in_srgb,var(--color-foreground)_52%,transparent)] transition-opacity duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
              :class="
                avatarUploading
                  ? 'opacity-100'
                  : 'opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100'
              "
              aria-hidden="true"
            >
              <Camera class="size-6" :class="avatarUploading ? 'animate-pulse' : ''" />
            </span>
          </button>
          <input
            id="account-avatar-file"
            ref="avatarInputRef"
            class="hidden"
            type="file"
            name="avatar-file"
            accept="image/*"
            :disabled="avatarUploading"
            @change="onAvatarFileChange"
          />
        </div>

        <div class="min-w-0">
          <p
            class="m-0 truncate text-[24px] leading-tight font-bold tracking-normal text-foreground sm:text-[30px]"
          >
            {{ userStore.user?.display_name }}
          </p>
          <div class="mt-3 flex min-w-0 flex-wrap items-center gap-2">
            <Badge
              :variant="
                userRoleSeverity === 'danger'
                  ? 'destructive'
                  : userRoleSeverity === 'secondary'
                    ? 'secondary'
                    : 'default'
              "
            >
              {{ userRoleLabel }}
            </Badge>
          </div>
        </div>
      </div>
    </section>
  </AccountPane>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Camera } from '@iconoir/vue'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { useUserStore } from '../../../auth'
import { useAvatarUpload } from '../../application/profile/useAvatarUpload'
import AccountPane from '../common/AccountPane.vue'

type TagSeverity = 'success' | 'info' | 'warn' | 'danger' | 'secondary'

const { t } = useI18n()
const userStore = useUserStore()
const { uploading: avatarUploading, handleFileChange: onAvatarFileChange } = useAvatarUpload()
const avatarInputRef = ref<HTMLInputElement | null>(null)

// Canonical principal facts only (issue #913): display_name/role come from
// `GET /api/v1/user`; avatar uploads persist the profile URL for later session reloads.
// Storage and operational fields are outside the public session contract.
const userRoleLabel = computed(() =>
  userStore.user?.role === 'admin'
    ? t('account.profile.roleAdmin')
    : t('account.profile.roleCreator'),
)
const userRoleSeverity = computed<TagSeverity>(() =>
  userStore.user?.role === 'admin' ? 'info' : 'secondary',
)
const userInitial = computed(() => {
  const displayName = userStore.user?.display_name?.trim()
  return displayName?.[0]?.toUpperCase() || 'U'
})

function openAvatarPicker() {
  avatarInputRef.value?.click()
}
</script>
