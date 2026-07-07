<template>
  <section
    :class="mainContentClass"
    aria-label="账户管理"
  >
    <Button
      type="button"
      text
      rounded
      class="!absolute !top-4 !right-4 !z-20 !flex !h-8 !w-8 !min-w-8 !items-center !justify-center !rounded-[var(--radius-ib-pill)] !border !border-transparent !bg-transparent !p-0 !text-ib-text-muted !shadow-none !transition !duration-[var(--duration-ib-standard)] hover:!border-ib-border-soft hover:!bg-ib-surface-hover hover:!text-ib-text focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)]"
      aria-label="关闭账户管理"
      @click="emit('close')"
    >
      <Xmark class="h-4 w-4" />
    </Button>

    <aside
      class="flex min-h-0 flex-col bg-ib-surface px-4 py-5 max-[820px]:px-4 max-[820px]:py-4"
    >
      <div class="mb-7 flex min-w-0 items-center gap-2.5 px-1" :aria-label="projectName">
        <span
          class="inline-flex size-[2.7rem] shrink-0 items-center justify-center overflow-hidden rounded-[var(--radius-ib-control)] border border-ib-border-soft bg-ib-surface-raised text-ib-creative-accent-pressed"
        >
          <SiteBrandLogo :src="logoURL" :fallback-text="fallbackText" class="h-full w-full" />
        </span>
        <span class="min-w-0 truncate text-[19px] leading-[29px] font-bold tracking-normal text-ib-text">
          {{ projectName }}
        </span>
      </div>

      <nav class="min-w-0" aria-label="账户管理导航">
        <p class="mb-2 px-2 text-[12px] leading-5 font-semibold tracking-normal text-ib-text-subtle">
          通用设置
        </p>
        <div class="grid gap-1.5 max-[820px]:grid-cols-2 max-[520px]:grid-cols-1">
          <Button
            v-for="item in navItems"
            :key="item.key"
            type="button"
            text
            class="!flex !h-11 !w-full !justify-start !gap-2 !rounded-[var(--radius-ib-control)] !border-0 !px-2.5 !text-[14px] !font-semibold !shadow-none focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)]"
            :class="
              activeTab === item.key
                ? '!bg-ib-creative-accent-soft !text-ib-creative-accent-pressed'
                : '!bg-transparent !text-ib-text-muted hover:!bg-ib-surface-hover hover:!text-ib-text'
            "
            :aria-current="activeTab === item.key ? 'page' : undefined"
            @click="activeTab = item.key"
          >
            <component :is="item.icon" class="h-4 w-4 shrink-0" />
            <span class="truncate">{{ item.label }}</span>
          </Button>
        </div>
      </nav>

      <div class="mt-auto pt-4">
        <div
          v-if="logoutConfirmationVisible"
          class="mb-3 rounded-[var(--radius-ib-panel)] bg-ib-danger/5 px-3 py-3 text-ib-danger"
          role="group"
          aria-label="退出登录确认"
        >
          <p class="m-0 text-[13px] leading-5 font-semibold">确认退出当前账号？</p>
          <p class="mt-1 mb-3 text-[12px] leading-5 text-ib-text-muted">
            退出后可重新登录继续访问。
          </p>
          <div class="grid grid-cols-2 gap-2">
            <Button
              type="button"
              text
              label="取消"
              :disabled="logoutLoading"
              class="!h-9 !rounded-[var(--radius-ib-control)] !border-0 !bg-transparent !px-2 !text-[13px] !font-semibold !text-ib-text-muted !shadow-none hover:!bg-ib-surface-hover hover:!text-ib-text focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-not-allowed disabled:!opacity-60"
              @click="cancelLogout"
            />
            <Button
              type="button"
              text
              :label="logoutLoading ? '退出中...' : '确认退出'"
              :disabled="logoutLoading"
              class="!h-9 !rounded-[var(--radius-ib-control)] !border-0 !bg-ib-danger !px-2 !text-[13px] !font-semibold !text-ib-text-inverse !shadow-none hover:!bg-ib-danger/90 focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-not-allowed disabled:!opacity-70"
              @click="confirmLogout"
            />
          </div>
        </div>
        <Button
          type="button"
          text
          :disabled="logoutLoading"
          class="!flex !h-11 !w-full !justify-start !gap-2 !rounded-[var(--radius-ib-control)] !border-0 !bg-transparent !px-2.5 !text-[14px] !font-semibold !text-ib-danger !shadow-none !transition !duration-[var(--duration-ib-standard)] hover:!bg-ib-danger/5 hover:!text-ib-danger focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-not-allowed disabled:!opacity-60"
          :aria-label="logoutLoading ? '正在退出登录' : '退出登录'"
          @click="handleLogout"
        >
          <LogOut class="h-4 w-4 shrink-0" />
          <span class="truncate">{{ logoutLoading ? '退出中...' : '退出登录' }}</span>
        </Button>
      </div>
    </aside>

    <main
      :class="[profilePaneClass, { 'is-scrolling': isProfileMainScrolling }]"
      @scroll.passive="handleProfileMainScroll"
    >
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
          <div
            class="mb-8 flex min-w-0 items-center gap-7 max-[720px]:items-start max-[520px]:flex-col"
          >
            <div
              class="relative h-[132px] w-[132px] shrink-0 overflow-visible rounded-[var(--radius-ib-pill)] bg-ib-surface shadow-[var(--shadow-ib-control)] max-[520px]:h-24 max-[520px]:w-24"
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
                class="!h-full !w-full !bg-[linear-gradient(135deg,var(--color-ib-creative-accent),var(--color-ib-creative-accent-secondary))] !text-[44px] !font-bold !text-ib-text-inverse max-[520px]:!text-[32px]"
              />
              <Button
                type="button"
                text
                rounded
                class="!absolute !right-1 !bottom-1 !flex !h-10 !w-10 !min-w-10 !items-center !justify-center !rounded-[var(--radius-ib-pill)] !border !border-ib-border !bg-ib-surface !p-0 !text-ib-text-muted !shadow-[var(--shadow-ib-control)] !transition !duration-[var(--duration-ib-standard)] hover:!border-ib-border-hover hover:!bg-ib-surface-hover hover:!text-ib-text focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-not-allowed disabled:!opacity-60 max-[520px]:!h-8 max-[520px]:!w-8 max-[520px]:!min-w-8"
                :disabled="avatarUploading"
                aria-label="更换头像"
                title="更换头像"
                @click="openAvatarPicker"
              >
                <Camera class="h-[18px] w-[18px] max-[520px]:h-[15px] max-[520px]:w-[15px]" />
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
              <h2 class="m-0 truncate text-[32px] leading-tight font-bold tracking-normal text-ib-text max-[520px]:text-[26px]">
                {{ userStore.user?.username }}
              </h2>
              <p class="mt-2 mb-5 truncate text-[22px] leading-7 font-semibold text-ib-text-subtle max-[520px]:text-lg">
                {{ userStore.user?.uid || '-' }}
              </p>
              <div class="grid gap-3">
                <Tag
                  :value="userStatusLabel"
                  :severity="userStatusSeverity"
                  rounded
                  class="!w-fit !bg-ib-surface !px-0 !text-[22px] !font-bold max-[520px]:!text-lg"
                />
                <Tag
                  :value="userRoleLabel"
                  :severity="userRoleSeverity"
                  rounded
                  class="!w-fit !bg-ib-surface !px-0 !text-[22px] !font-bold max-[520px]:!text-lg"
                />
              </div>
            </div>
          </div>

          <dl class="grid grid-cols-2 gap-4 max-[720px]:grid-cols-1">
            <div
              v-for="field in profileFields"
              :key="field.label"
              class="min-w-0 rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface px-4 py-3.5"
              :class="field.full ? 'col-span-2 max-[720px]:col-span-1' : ''"
            >
              <dt class="mb-2 block text-[13px] font-semibold text-ib-text-subtle">
                {{ field.label }}
              </dt>
              <dd class="m-0 min-w-0">
                <Tag
                  v-if="field.kind === 'tag'"
                  :value="field.value"
                  :severity="field.severity"
                  rounded
                  class="!bg-ib-surface-raised"
                />
                <span
                  v-else
                  class="block truncate text-base font-semibold text-ib-text"
                  :class="field.mono ? 'font-mono text-[15px]' : ''"
                  :title="field.value"
                >
                  {{ field.value }}
                </span>
              </dd>
            </div>
          </dl>
        </div>

        <div v-else-if="activeTab === 'security'" key="security">
          <div class="mb-7">
            <p class="m-0 mb-1 text-[13px] font-semibold text-ib-creative-accent-pressed">
              登录凭据
            </p>
            <h2 class="m-0 text-[28px] leading-tight font-bold tracking-normal text-ib-text">
              账号安全
            </h2>
            <p class="mt-2 mb-0 max-w-2xl text-[14px] leading-6 text-ib-text-subtle">
              更新密码后会退出当前登录状态，请使用新密码重新登录。
            </p>
          </div>

          <form class="flex max-w-3xl flex-col gap-7" novalidate @submit.prevent="handleUpdatePassword">
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

            <Button
              type="submit"
              :label="pwdLoading ? '更新中...' : '确认更新密码'"
              :loading="pwdLoading"
              :disabled="pwdLoading"
              class="!h-12 !w-full !rounded-[var(--radius-ib-panel)] !border-0 !bg-ib-creative-accent-hover !text-[15px] !font-semibold !text-ib-text-inverse !shadow-[var(--shadow-ib-control)] transition hover:!-translate-y-0.5 hover:!bg-ib-creative-accent-pressed hover:!shadow-[var(--shadow-ib-panel)] active:!translate-y-0 focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-not-allowed disabled:!opacity-70"
            />
          </form>
        </div>
      </Transition>
    </main>
  </section>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import { computed, onMounted, onUnmounted, ref, type Component } from 'vue'
import { useAvatarUploadWorkflow } from '../useAvatarUploadWorkflow'
import { usePasswordChangeWorkflow } from '../usePasswordChangeWorkflow'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Tag from 'primevue/tag'
import { Camera, Lock, LogOut, User, Xmark } from '@iconoir/vue'
import SiteBrandLogo from '../../../shared/components/SiteBrandLogo.vue'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { useAppToast } from '../../../shared/composables/useAppToast'
import { USER_HOME_PATH } from '../../../shared/constants/routePaths'

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

const emit = defineEmits<{
  close: []
}>()
const userStore = useUserStore()
const { fallbackText, logoURL, projectName } = useSiteBranding()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const appToast = useAppToast()
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
const logoutConfirmationVisible = ref(false)
const logoutLoading = ref(false)
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

const mainContentClass =
  'relative grid h-[min(78dvh,720px)] min-h-0 grid-cols-[234px_minmax(0,1fr)] overflow-hidden bg-ib-surface-raised max-[820px]:h-[calc(100dvh-64px)] max-[820px]:grid-cols-1'
const profilePaneClass =
  'app-scrollbar min-h-0 overflow-x-hidden overflow-y-auto rounded-l-[var(--radius-ib-overlay)] bg-ib-surface-muted px-8 py-7 pr-14 max-[820px]:rounded-l-none max-[820px]:rounded-t-[var(--radius-ib-overlay)] max-[820px]:px-4 max-[820px]:py-5 max-[820px]:pr-14'
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

function handleLogout() {
  if (logoutLoading.value) return
  logoutConfirmationVisible.value = true
}

function cancelLogout() {
  if (logoutLoading.value) return
  logoutConfirmationVisible.value = false
}

async function confirmLogout() {
  if (logoutLoading.value) return

  logoutLoading.value = true
  try {
    if (await logoutAndRedirect(USER_HOME_PATH)) {
      emit('close')
    } else {
      appToast.error('退出登录失败，请稍后重试')
    }
  } finally {
    logoutLoading.value = false
  }
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
