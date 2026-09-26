<template>
  <AccountPane :title="t('account.tabs.profile')">
    <section class="min-w-0" :aria-label="t('account.profile.overviewAria')">
      <div class="flex min-w-0 items-center gap-5 max-[520px]:items-start">
        <div
          class="relative size-[88px] shrink-0 overflow-visible rounded-ib-pill bg-ib-surface-raised shadow-ib-control max-[520px]:size-20"
        >
          <button
            type="button"
            class="group relative flex h-full w-full cursor-pointer items-center justify-center overflow-hidden rounded-ib-pill border-0 bg-transparent p-0 text-ib-text-inverse outline-none transition-shadow duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] focus-visible:shadow-ib-focus disabled:cursor-wait"
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
                class="bg-[linear-gradient(135deg,var(--color-ib-creative-accent),var(--color-ib-creative-accent-secondary))] text-[34px] font-bold text-ib-text-inverse max-[520px]:text-[28px]"
              >
                {{ userInitial }}
              </AvatarFallback>
            </Avatar>
            <span
              class="pointer-events-none absolute inset-0 flex items-center justify-center rounded-ib-pill bg-[color-mix(in_srgb,var(--color-ib-text)_52%,transparent)] transition-opacity duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
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
            class="m-0 truncate text-[30px] leading-tight font-bold tracking-normal text-ib-text max-[520px]:text-[24px]"
          >
            {{ userStore.user?.display_name }}
          </p>
          <div class="mt-3 flex min-w-0 flex-wrap items-center gap-2">
            <Badge v-if="isUserBanned" variant="destructive">
              {{ t('account.profile.banned') }}
            </Badge>
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

    <section class="min-w-0" :aria-label="t('account.profile.accountInfoAria')">
      <dl class="m-0 border-y border-ib-border-soft">
        <div
          v-for="field in profileFields"
          :key="field.label"
          class="grid min-h-14 grid-cols-[148px_minmax(0,1fr)] items-center gap-4 border-b border-ib-border-soft py-3 last:border-b-0 max-[640px]:grid-cols-1 max-[640px]:gap-1"
        >
          <dt class="text-[13px] font-semibold text-ib-text-subtle">
            {{ field.label }}
          </dt>
          <dd class="m-0 min-w-0">
            <span
              class="block truncate text-[15px] font-semibold text-ib-text"
              :class="field.mono ? 'font-mono text-[14px]' : ''"
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
import { Badge } from '@/components/ui/badge'
import { useUserStore } from '../../../auth'
import { useAvatarUpload } from '../../application/profile/useAvatarUpload'
import AccountPane from '../common/AccountPane.vue'

type TagSeverity = 'success' | 'info' | 'warn' | 'danger' | 'secondary'
type ProfileField = {
  label: string
  value: string
  mono?: boolean
}

const { t } = useI18n()
const userStore = useUserStore()
const { uploading: avatarUploading, handleFileChange: onAvatarFileChange } = useAvatarUpload()
const avatarInputRef = ref<HTMLInputElement | null>(null)

const formattedJoinedAt = computed(() => formatDateWithDots(userStore.user?.created_at))
const formattedUpdatedAt = computed(() => formatDateWithDots(userStore.user?.updated_at))
const isUserBanned = computed(() => userStore.user?.status === 0)
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
const profileFields = computed<ProfileField[]>(() => [
  {
    label: t('account.profile.storageNamespace'),
    value: userStore.user?.storage_namespace || '-',
    mono: true,
  },
  {
    label: t('account.profile.joinedAt'),
    value: formattedJoinedAt.value,
  },
  {
    label: t('account.profile.lastUpdated'),
    value: formattedUpdatedAt.value,
  },
])

function formatDateWithDots(value?: string) {
  const datePart = value?.trim().split(/[T\s]/)[0]
  if (!datePart) return '-'
  const [year, month, day] = datePart.split('-')
  if (!year || !month || !day) return datePart.replaceAll('-', '.')
  return `${year}.${month}.${day}`
}

function openAvatarPicker() {
  avatarInputRef.value?.click()
}
</script>
