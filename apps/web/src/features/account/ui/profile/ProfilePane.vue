<template>
  <AccountPane :title="t('account.tabs.profile')">
    <section class="min-w-0" :aria-label="t('account.profile.overviewAria')">
      <div class="flex min-w-0 items-start gap-5 sm:items-center">
        <div
          class="relative size-20 shrink-0 overflow-visible rounded-pill bg-card shadow-sm sm:size-[88px]"
        >
          <button
            type="button"
            class="group relative flex h-full w-full cursor-pointer items-center justify-center overflow-hidden rounded-pill border-0 bg-transparent p-0 text-primary-foreground outline-none transition-shadow duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] focus-visible:shadow-[0_0_0_3px_var(--ring)] disabled:cursor-wait"
            :disabled="isAvatarUploading"
            :aria-busy="isAvatarUploading"
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
              class="pointer-events-none absolute inset-0 flex items-center justify-center rounded-pill bg-[color-mix(in_srgb,var(--foreground)_52%,transparent)] transition-opacity duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
              :class="
                isAvatarUploading
                  ? 'opacity-100'
                  : 'opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100'
              "
              aria-hidden="true"
            >
              <Camera class="size-6" :class="isAvatarUploading ? 'animate-pulse' : ''" />
            </span>
          </button>
          <input
            id="account-avatar-file"
            ref="avatarInputRef"
            class="hidden"
            type="file"
            name="avatar-file"
            accept="image/*"
            :disabled="isAvatarUploading"
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
            <StatusBadge v-if="isUserBanned" tone="danger" :dot="false">
              {{ t('account.profile.banned') }}
            </StatusBadge>
            <StatusBadge :tone="userRoleTone" :dot="false">
              {{ userRoleLabel }}
            </StatusBadge>
          </div>
        </div>
      </div>
    </section>

    <section class="min-w-0" :aria-label="t('account.profile.accountInfoAria')">
      <dl class="m-0 border-y border-border">
        <div
          v-for="field in profileFields"
          :key="field.label"
          class="grid min-h-14 grid-cols-1 items-center gap-1 border-b border-border py-3 last:border-b-0 sm:grid-cols-[148px_minmax(0,1fr)] sm:gap-4"
        >
          <dt class="text-admin-body font-semibold text-muted-foreground">
            {{ field.label }}
          </dt>
          <dd class="m-0 min-w-0">
            <span
              class="block truncate text-admin-title-sm font-semibold text-foreground"
              :class="field.mono ? 'font-mono text-admin-content' : ''"
              :title="field.value"
            >
              {{ field.value }}
            </span>
          </dd>
        </div>
      </dl>
    </section>
  </AccountPane>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Camera } from '@iconoir/vue'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import StatusBadge, { type AppStatusTone } from '../../../../core/components/StatusBadge.vue'
import { useUserStore } from '../../../auth'
import { useAvatarUpload } from '../../session/profile/useAvatarUpload'
import { formatDottedIsoDate } from '../../../../core/utils/dateTime'
import AccountPane from '../shared/AccountPane.vue'

type ProfileField = {
  label: string
  value: string
  mono?: boolean
}

const { t } = useI18n()
const userStore = useUserStore()
const { isUploading: isAvatarUploading, handleFileChange: onAvatarFileChange } = useAvatarUpload()
const avatarInputRef = ref<HTMLInputElement | null>(null)

// Session profile facts from `GET /api/v1/user`: display_name/role/avatar
// plus the storage namespace, numeric status (0 = banned), and timestamps.
const isUserBanned = computed(() => userStore.user?.status === 0)
const userRoleLabel = computed(() =>
  userStore.user?.role === 'admin'
    ? t('account.profile.roleAdmin')
    : t('account.profile.roleCreator'),
)
const userRoleTone = computed<AppStatusTone>(() =>
  userStore.user?.role === 'admin' ? 'info' : 'neutral',
)
const userInitial = computed(() => {
  const displayName = userStore.user?.display_name?.trim()
  return displayName?.[0]?.toUpperCase() || 'U'
})
const profileFields = computed<ProfileField[]>(() => [
  {
    label: t('account.profile.storageNamespace'),
    value: userStore.user?.storage_namespace || '-',
    mono: true,
  },
  {
    label: t('account.profile.joinedAt'),
    value: formatDottedIsoDate(userStore.user?.created_at),
  },
  {
    label: t('account.profile.lastUpdated'),
    value: formatDottedIsoDate(userStore.user?.updated_at),
  },
])

function openAvatarPicker() {
  avatarInputRef.value?.click()
}
</script>
