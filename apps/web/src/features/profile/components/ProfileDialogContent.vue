<template>
  <section
    :class="[mainContentClass, { 'is-scrolling': isProfileMainScrolling }]"
    aria-label="账户管理"
    @scroll.passive="handleProfileMainScroll"
  >
    <nav
      class="mb-6 grid grid-cols-2 gap-2 rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface-raised p-1.5"
      aria-label="账户管理导航"
    >
      <Button
        v-for="item in navItems"
        :key="`mobile-${item.key}`"
        type="button"
        text
        class="!flex !h-10 !justify-center !gap-2 !rounded-[var(--radius-ib-control)] !border-0 !px-3 !text-[14px] !font-semibold !shadow-none focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)]"
        :class="
          activeTab === item.key
            ? '!bg-ib-creative-accent-hover !text-ib-text-inverse'
            : '!bg-transparent !text-ib-text-muted hover:!bg-ib-surface-hover hover:!text-ib-text'
        "
        :aria-current="activeTab === item.key ? 'page' : undefined"
        @click="activeTab = item.key"
      >
        <component :is="item.icon" class="h-4 w-4" />
        <span>{{ item.label }}</span>
      </Button>
    </nav>

    <div
      class="mb-10 flex items-center justify-between gap-6 max-[720px]:flex-col max-[720px]:items-start"
    >
      <div class="flex items-center gap-5">
        <div
          class="relative h-20 w-20 overflow-visible rounded-[var(--radius-ib-pill)] bg-ib-surface shadow-[var(--shadow-ib-control)] max-[520px]:h-[72px] max-[520px]:w-[72px]"
        >
          <Avatar
            v-if="userStore.user?.avatar"
            :image="userStore.user.avatar"
            shape="circle"
            class="!h-full !w-full"
          />
          <Avatar
            v-else
            :label="userInitial"
            shape="circle"
            class="!h-full !w-full !bg-[linear-gradient(135deg,var(--color-ib-creative-accent),var(--color-ib-creative-accent-secondary))] !text-[32px] !font-bold !text-ib-text-inverse"
          />
          <Button
            type="button"
            text
            rounded
            class="!absolute !right-[-4px] !bottom-[-4px] !flex !h-7 !w-7 !min-w-7 !items-center !justify-center !rounded-[var(--radius-ib-pill)] !border !border-ib-border !bg-ib-surface !p-0 !text-ib-text-muted !shadow-none !transition !duration-[var(--duration-ib-standard)] hover:!border-ib-border-hover hover:!bg-ib-surface-hover hover:!text-ib-text focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-not-allowed disabled:!opacity-60"
            :disabled="avatarUploading"
            aria-label="更换头像"
            title="更换头像"
            @click="openAvatarPicker"
          >
            <Camera class="h-[15px] w-[15px]" />
          </Button>
          <input
            ref="avatarInputRef"
            class="hidden"
            type="file"
            accept="image/*"
            :disabled="avatarUploading"
            @change="onAvatarFileChange"
          />
        </div>
        <div class="min-w-0">
          <h1 class="m-0 truncate text-[28px] leading-tight font-bold tracking-normal text-ib-text">
            {{ userStore.user?.username }}
          </h1>
          <p class="mt-1 mb-0 text-[15px] text-ib-text-subtle">
            {{ userRoleLabel }}
          </p>
        </div>
      </div>

      <div class="flex gap-6">
        <div
          class="flex min-w-[100px] flex-col rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface-raised px-5 py-3"
        >
          <span class="text-xl font-bold text-ib-text">{{ userStore.user?.uid || '-' }}</span>
          <span class="text-[13px] text-ib-text-subtle">会员编号</span>
        </div>
      </div>
    </div>

    <div class="relative flex-1">
      <Transition
        mode="out-in"
        enter-active-class="transition-opacity duration-150 ease-out"
        enter-from-class="opacity-0"
        enter-to-class="opacity-100"
        leave-active-class="transition-opacity duration-100 ease-in"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
      >
        <div v-if="activeTab === 'profile'" key="profile">
          <Card :pt="profileCardPt">
            <template #content>
              <h2 class="m-0 mb-8 text-xl font-semibold text-ib-text">个人基本资料</h2>
              <dl class="grid grid-cols-2 gap-4 max-[720px]:grid-cols-1">
                <div
                  v-for="field in profileFields"
                  :key="field.label"
                  class="min-w-0 rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface-muted px-4 py-3.5"
                  :class="field.full ? 'col-span-2 max-[720px]:col-span-1' : ''"
                >
                  <dt class="mb-2 block text-[13px] font-medium text-ib-text-subtle">
                    {{ field.label }}
                  </dt>
                  <dd class="m-0 min-w-0">
                    <Tag
                      v-if="field.kind === 'tag'"
                      :value="field.value"
                      :severity="field.severity"
                      rounded
                      class="!bg-ib-surface"
                    />
                    <span
                      v-else
                      class="block truncate text-base font-medium text-ib-text"
                      :class="field.mono ? 'font-mono text-[15px]' : ''"
                      :title="field.value"
                    >
                      {{ field.value }}
                    </span>
                  </dd>
                </div>
              </dl>
            </template>
          </Card>
        </div>

        <div v-else-if="activeTab === 'security'" key="security">
          <Card :pt="profileCardPt">
            <template #content>
              <h2 class="m-0 mb-8 text-xl font-semibold text-ib-text">账号安全设置</h2>
              <form class="flex flex-col gap-8" novalidate @submit.prevent="handleUpdatePassword">
                <div class="flex flex-col gap-4">
                  <label :class="passwordFieldClass">
                    <span>当前原密码</span>
                    <Password
                      v-model="pwdForm.oldPassword"
                      autocomplete="current-password"
                      :feedback="false"
                      fluid
                      :input-props="{
                        autocomplete: 'current-password',
                        'aria-describedby': getPasswordDescribedby(
                          'oldPassword',
                          'profile-old-password-error',
                        ),
                      }"
                      :invalid="Boolean(pwdErrors.oldPassword)"
                      :input-class="[
                        passwordInputClass,
                        isPasswordInlineError('oldPassword') ? passwordInputErrorClass : '',
                      ]"
                      :placeholder="
                        isPasswordInlineError('oldPassword')
                          ? displayErrorMessage(pwdErrors.oldPassword)
                          : '请输入当前使用的密码'
                      "
                      toggle-mask
                      @blur="validatePasswordField('oldPassword')"
                      @update:model-value="clearPasswordFieldError('oldPassword')"
                    />
                    <Message
                      v-if="shouldShowPasswordMessage('oldPassword')"
                      id="profile-old-password-error"
                      class="!mt-1 !rounded-xl"
                      severity="error"
                      size="small"
                      :closable="false"
                    >
                      {{ displayErrorMessage(pwdErrors.oldPassword) }}
                    </Message>
                  </label>

                  <div class="grid grid-cols-2 gap-5 max-[720px]:grid-cols-1">
                    <label :class="passwordFieldClass">
                      <span>设置新密码</span>
                      <Password
                        v-model="pwdForm.newPassword"
                        autocomplete="new-password"
                        :feedback="false"
                        fluid
                        :input-props="{
                          autocomplete: 'new-password',
                          'aria-describedby': getPasswordDescribedby(
                            'newPassword',
                            'profile-new-password-error',
                          ),
                        }"
                        :invalid="Boolean(pwdErrors.newPassword)"
                        :input-class="[
                          passwordInputClass,
                          isPasswordInlineError('newPassword') ? passwordInputErrorClass : '',
                        ]"
                        :placeholder="
                          isPasswordInlineError('newPassword')
                            ? displayErrorMessage(pwdErrors.newPassword)
                            : '不少于6位字符'
                        "
                        toggle-mask
                        @blur="validatePasswordField('newPassword')"
                        @update:model-value="handleNewPasswordInput"
                      />
                      <Message
                        v-if="shouldShowPasswordMessage('newPassword')"
                        id="profile-new-password-error"
                        class="!mt-1 !rounded-xl"
                        severity="error"
                        size="small"
                        :closable="false"
                      >
                        {{ displayErrorMessage(pwdErrors.newPassword) }}
                      </Message>
                    </label>

                    <label :class="passwordFieldClass">
                      <span>重复新密码</span>
                      <Password
                        v-model="pwdForm.confirmPassword"
                        autocomplete="new-password"
                        :feedback="false"
                        fluid
                        :input-props="{
                          autocomplete: 'new-password',
                          'aria-describedby': getPasswordDescribedby(
                            'confirmPassword',
                            'profile-confirm-password-error',
                          ),
                        }"
                        :invalid="Boolean(pwdErrors.confirmPassword)"
                        :input-class="[
                          passwordInputClass,
                          isPasswordInlineError('confirmPassword') ? passwordInputErrorClass : '',
                        ]"
                        :placeholder="
                          isPasswordInlineError('confirmPassword')
                            ? displayErrorMessage(pwdErrors.confirmPassword)
                            : '请再次输入新密码'
                        "
                        toggle-mask
                        @blur="validatePasswordField('confirmPassword')"
                        @update:model-value="clearPasswordFieldError('confirmPassword')"
                      />
                      <Message
                        v-if="shouldShowPasswordMessage('confirmPassword')"
                        id="profile-confirm-password-error"
                        class="!mt-1 !rounded-xl"
                        severity="error"
                        size="small"
                        :closable="false"
                      >
                        {{ displayErrorMessage(pwdErrors.confirmPassword) }}
                      </Message>
                    </label>
                  </div>
                </div>

                <Button
                  type="submit"
                  :label="pwdLoading ? '更新中...' : '确认更新密码'"
                  :loading="pwdLoading"
                  :disabled="pwdLoading"
                  fluid
                  class="!h-12 !rounded-[var(--radius-ib-panel)] !border-0 !bg-ib-creative-accent-hover !text-[15px] !font-semibold !text-ib-text-inverse !shadow-[var(--shadow-ib-control)] transition hover:!-translate-y-0.5 hover:!bg-ib-creative-accent-pressed hover:!shadow-[var(--shadow-ib-panel)] active:!translate-y-0 focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-not-allowed disabled:!opacity-70"
                />
              </form>
            </template>
          </Card>
        </div>
      </Transition>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import { computed, onMounted, onUnmounted, ref, type Component } from 'vue'
import { useAvatarUploadWorkflow } from '../useAvatarUploadWorkflow'
import { usePasswordChangeWorkflow } from '../usePasswordChangeWorkflow'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Tag from 'primevue/tag'
import { Camera, Lock, User } from '@iconoir/vue'

type NavItem = {
  key: 'profile' | 'security'
  label: string
  icon: Component
}

type TagSeverity = 'success' | 'info' | 'warn' | 'danger' | 'secondary'
type ProfileField = {
  label: string
  value: string
  kind?: 'tag'
  severity?: TagSeverity
  mono?: boolean
  full?: boolean
}

const userStore = useUserStore()
const {
  form: pwdForm,
  errors: pwdErrors,
  loading: pwdLoading,
  validateField: validatePasswordField,
  clearFieldError: clearPasswordFieldError,
  handleNewPasswordInput,
  submit: handleUpdatePassword,
  isInlineError: isPasswordInlineError,
  shouldShowMessage: shouldShowPasswordMessage,
  getDescribedby: getPasswordDescribedby,
  displayErrorMessage,
} = usePasswordChangeWorkflow()
const { uploading: avatarUploading, handleFileChange: onAvatarFileChange } =
  useAvatarUploadWorkflow()

const activeTab = ref<NavItem['key']>('profile')
const avatarInputRef = ref<HTMLInputElement | null>(null)
const isProfileMainScrolling = ref(false)
let profileMainScrollTimer: number | undefined

const navItems = [
  {
    key: 'profile',
    label: '个人资料',
    icon: User,
  },
  {
    key: 'security',
    label: '账号安全',
    icon: Lock,
  },
] satisfies NavItem[]

const profileCardPt = {
  root: {
    class:
      'overflow-hidden rounded-[var(--radius-ib-overlay)] border border-ib-border-soft bg-ib-surface shadow-none',
  },
  body: {
    class: '!p-[34px] max-[640px]:!p-6',
  },
  content: {
    class: '!p-0',
  },
} as const

const mainContentClass =
  'app-scrollbar flex max-h-[min(78dvh,720px)] min-h-0 flex-col overflow-x-hidden overflow-y-auto px-8 py-7 max-[640px]:max-h-[calc(100dvh-128px)] max-[640px]:px-4 max-[640px]:py-5'
const passwordFieldClass = 'flex min-w-0 flex-col gap-2 text-sm font-medium text-ib-text'
const passwordInputClass =
  '!h-12 !w-full !rounded-[var(--radius-ib-control)] !border-ib-border !bg-ib-surface-raised !px-3 !text-[15px] !text-ib-text !shadow-none placeholder:!text-ib-text-subtle enabled:hover:!border-ib-border-hover enabled:focus:!border-ib-creative-accent enabled:focus:!shadow-[var(--shadow-ib-focus)]'
const passwordInputErrorClass =
  '!border-ib-danger !bg-ib-danger/5 placeholder:!text-ib-danger/75 enabled:focus:!border-ib-danger'

const formattedJoinedAt = computed(() => formatDateWithDots(userStore.user?.created_at))
const formattedUpdatedAt = computed(() => formatDateWithDots(userStore.user?.updated_at))
const userStatusLabel = computed(() => (userStore.user?.status === 1 ? '正常使用中' : '已封禁'))
const userStatusSeverity = computed<TagSeverity>(() =>
  userStore.user?.status === 1 ? 'success' : 'danger',
)
const userRoleLabel = computed(() => (userStore.user?.role === 'admin' ? '管理员' : '创作者'))
const userRoleSeverity = computed<TagSeverity>(() =>
  userStore.user?.role === 'admin' ? 'info' : 'secondary',
)
const userInitial = computed(() => {
  const username = userStore.user?.username?.trim()
  return username?.[0]?.toUpperCase() || 'U'
})
const profileFields = computed<ProfileField[]>(() => [
  {
    label: '用户账号',
    value: userStore.user?.username || '-',
  },
  {
    label: '账号状态',
    value: userStatusLabel.value,
    kind: 'tag',
    severity: userStatusSeverity.value,
  },
  {
    label: '用户 UID',
    value: userStore.user?.uid || '-',
    mono: true,
  },
  {
    label: '角色',
    value: userRoleLabel.value,
    kind: 'tag',
    severity: userRoleSeverity.value,
  },
  {
    label: '认证 ID',
    value: userStore.user?.auth_user_id?.trim() || '-',
    mono: true,
    full: true,
  },
  {
    label: '加入时间',
    value: formattedJoinedAt.value,
  },
  {
    label: '最近更新',
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

function handleProfileMainScroll() {
  isProfileMainScrolling.value = true
  if (profileMainScrollTimer) window.clearTimeout(profileMainScrollTimer)
  profileMainScrollTimer = window.setTimeout(() => {
    isProfileMainScrolling.value = false
    profileMainScrollTimer = undefined
  }, 700)
}

onMounted(() => {
  void userStore.fetchUserInfo()
})

onUnmounted(() => {
  if (profileMainScrollTimer) {
    window.clearTimeout(profileMainScrollTimer)
    profileMainScrollTimer = undefined
  }
})
</script>
