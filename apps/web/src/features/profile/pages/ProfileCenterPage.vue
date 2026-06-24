<template>
  <div class="relative h-screen w-full overflow-hidden bg-[#f8f9fd] text-[#1f2430]">
    <div :class="profileGridBgClass"></div>

    <div
      class="relative z-[1] mx-auto my-[5vh] flex h-[90vh] max-w-[1200px] overflow-hidden rounded-[24px] border border-[#d9dfec] bg-white/[0.82] shadow-[0_24px_80px_rgba(50,62,96,0.08)] backdrop-blur-[16px] max-[900px]:my-0 max-[900px]:h-screen max-[900px]:max-w-none max-[900px]:rounded-none"
    >
      <aside
        class="flex w-[260px] shrink-0 flex-col border-r border-black/5 px-6 py-10 max-[760px]:w-[220px] max-[640px]:hidden"
      >
        <div class="mb-12 flex justify-center">
          <Button
            type="button"
            text
            class="!inline-flex !max-w-full !items-center !gap-2 !border-0 !bg-transparent !p-0 !text-[33px] !leading-none !font-bold !text-[#1d1d1f] !shadow-none hover:!bg-transparent hover:!text-[#1d1d1f]"
            @click="router.push('/user/home')"
          >
            <SiteBrandLogo :src="logoURL" class="h-8 w-8 rounded-lg" />
            <span class="min-w-0 truncate">{{ projectName }}</span>
          </Button>
        </div>

        <nav class="flex flex-1 flex-col gap-2" aria-label="个人中心导航">
          <Button
            v-for="item in navItems"
            :key="item.key"
            type="button"
            text
            class="!flex !w-full !justify-start !gap-3 !rounded-[14px] !border-0 !px-4 !py-3 !text-left !text-[15px] !font-medium !shadow-none !transition !duration-200"
            :class="
              activeTab === item.key
                ? '!bg-canvas-accent-hover !text-white'
                : '!bg-transparent !text-[#86868b] hover:!bg-[#eef1fa] hover:!text-[#2c3448]'
            "
            :aria-current="activeTab === item.key ? 'page' : undefined"
            @click="activeTab = item.key"
          >
            <span class="flex h-5 w-5 items-center justify-center">
              <component :is="item.icon" class="h-5 w-5" />
            </span>
            <span>{{ item.label }}</span>
          </Button>
        </nav>

        <div class="mt-auto flex flex-col gap-3">
          <Button
            v-if="userStore.user?.role === 'admin'"
            type="button"
            class="!flex !w-full !justify-start !gap-2.5 !rounded-xl !border-0 !bg-[linear-gradient(135deg,var(--color-canvas-accent-hover),var(--color-canvas-accent))] !px-4 !py-3 !text-[15px] !font-semibold !text-white !shadow-none !transition !duration-200 hover:!bg-[linear-gradient(135deg,var(--color-canvas-accent-pressed),var(--color-canvas-accent-hover))]"
            @click="router.push('/admin/users')"
          >
            <Shield class="h-[18px] w-[18px]" />
            管理后台
          </Button>

          <Button
            type="button"
            class="!flex !w-full !justify-start !gap-2.5 !rounded-xl !border-0 !bg-[#f1f3f9] !px-4 !py-3 !text-[15px] !font-semibold !text-[#4d556a] !shadow-none !transition !duration-200 hover:!bg-[#e8ecf7] hover:!text-[#2a3142]"
            @click="router.back()"
          >
            <NavArrowLeft class="h-[18px] w-[18px]" />
            返回上级
          </Button>

          <Button
            type="button"
            text
            class="!flex !w-full !justify-start !gap-2.5 !rounded-xl !border-0 !bg-transparent !px-4 !py-3 !text-[15px] !font-semibold !text-[#ff3b30] !shadow-none !transition !duration-200 hover:!bg-[rgba(255,59,48,0.05)]"
            @click="handleLogout"
          >
            <LogOut class="h-[18px] w-[18px]" />
            退出登录
          </Button>
        </div>
      </aside>

      <main
        :class="[mainContentClass, { 'is-scrolling': isProfileMainScrolling }]"
        @scroll.passive="handleProfileMainScroll"
      >
        <nav
          class="mb-6 hidden grid-cols-2 gap-2 rounded-2xl border border-[#e2e7f3] bg-white/80 p-1.5 max-[640px]:grid"
          aria-label="个人中心导航"
        >
          <Button
            v-for="item in navItems"
            :key="`mobile-${item.key}`"
            type="button"
            text
            class="!flex !h-10 !justify-center !gap-2 !rounded-xl !border-0 !px-3 !text-[14px] !font-semibold !shadow-none"
            :class="
              activeTab === item.key
                ? '!bg-canvas-accent-hover !text-white'
                : '!bg-transparent !text-[#6b7280] hover:!bg-[#eef1fa] hover:!text-[#2c3448]'
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
              class="relative h-20 w-20 overflow-visible rounded-full bg-white shadow-[0_10px_26px_rgba(71,82,118,0.1)] max-[520px]:h-[72px] max-[520px]:w-[72px]"
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
                class="!h-full !w-full !bg-[linear-gradient(135deg,var(--color-canvas-accent),var(--color-canvas-accent-secondary))] !text-[32px] !font-bold !text-white"
              />
              <Button
                type="button"
                text
                rounded
                class="!absolute !right-[-4px] !bottom-[-4px] !flex !h-7 !w-7 !min-w-7 !items-center !justify-center !rounded-full !border !border-[#d9dfec] !bg-white/[0.98] !p-0 !text-[#6b7290] !shadow-none !transition !duration-200 hover:!border-[#c8d0e2] hover:!bg-[#f8faff] hover:!text-[#4a5169] disabled:!cursor-not-allowed disabled:!opacity-60"
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
              <h1
                class="m-0 truncate text-[28px] leading-tight font-bold tracking-normal text-[#1d1d1f]"
              >
                {{ userStore.user?.username }}
              </h1>
              <p class="mt-1 mb-0 text-[15px] text-[#86868b]">
                {{ userRoleLabel }}
              </p>
            </div>
          </div>

          <div class="flex gap-6">
            <div
              class="flex min-w-[100px] flex-col rounded-2xl border border-white/80 bg-white/50 px-5 py-3"
            >
              <span class="text-xl font-bold text-[#1d1d1f]">{{ userStore.user?.uid || '-' }}</span>
              <span class="text-[13px] text-[#86868b]">会员编号</span>
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
                  <h2 class="m-0 mb-8 text-xl font-semibold text-[#1d1d1f]">个人基本资料</h2>
                  <dl class="grid grid-cols-2 gap-4 max-[720px]:grid-cols-1">
                    <div
                      v-for="field in profileFields"
                      :key="field.label"
                      class="min-w-0 rounded-2xl border border-[#edf1f7] bg-[#fbfcff] px-4 py-3.5"
                      :class="field.full ? 'col-span-2 max-[720px]:col-span-1' : ''"
                    >
                      <dt class="mb-2 block text-[13px] font-medium text-[#86868b]">
                        {{ field.label }}
                      </dt>
                      <dd class="m-0 min-w-0">
                        <Tag
                          v-if="field.kind === 'tag'"
                          :value="field.value"
                          :severity="field.severity"
                          rounded
                          class="!bg-white"
                        />
                        <span
                          v-else
                          class="block truncate text-base font-medium text-[#1d1d1f]"
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
                  <h2 class="m-0 mb-8 text-xl font-semibold text-[#1d1d1f]">账号安全设置</h2>
                  <form
                    class="flex flex-col gap-8"
                    novalidate
                    @submit.prevent="handleUpdatePassword"
                  >
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
                              isPasswordInlineError('confirmPassword')
                                ? passwordInputErrorClass
                                : '',
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
                      class="!h-12 !rounded-[14px] !border-0 !bg-canvas-accent-hover !text-[15px] !font-semibold !text-white !shadow-[0_8px_24px_var(--color-canvas-accent-shadow)] transition hover:!-translate-y-0.5 hover:!bg-canvas-accent-pressed hover:!shadow-[0_12px_32px_var(--color-canvas-accent-shadow-hover)] active:!translate-y-0"
                    />
                  </form>
                </template>
              </Card>
            </div>
          </Transition>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { useAppToast } from '../../../shared/composables/useAppToast'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import SiteBrandLogo from '../../../shared/components/SiteBrandLogo.vue'
import { uploadApi } from '../../../shared/api/upload'
import {
  isRequiredErrorMessage,
  markRequiredError,
  displayErrorMessage,
} from '../../../shared/utils/formValidation'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Tag from 'primevue/tag'
import type { Component } from 'vue'
import { Camera, Lock, LogOut, NavArrowLeft, Shield, User } from '@iconoir/vue'

type NavItem = {
  key: 'profile' | 'security'
  label: string
  icon: Component
}

type PasswordField = 'oldPassword' | 'newPassword' | 'confirmPassword'
type PasswordForm = Record<PasswordField, string>
type TagSeverity = 'success' | 'info' | 'warn' | 'danger' | 'secondary'
type ProfileField = {
  label: string
  value: string
  kind?: 'tag'
  severity?: TagSeverity
  mono?: boolean
  full?: boolean
}

const router = useRouter()
const userStore = useUserStore()
const appToast = useAppToast()
const { runAsyncAction } = useAsyncAction()
const { logoutAndRedirect } = useLogoutRedirect()
const { logoURL, projectName } = useSiteBranding()

const activeTab = ref<NavItem['key']>('profile')
const avatarInputRef = ref<HTMLInputElement | null>(null)
const avatarUploading = ref(false)
const pwdLoading = ref(false)
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
    class: 'overflow-hidden rounded-[18px] border border-[#e2e7f3] bg-white shadow-none',
  },
  body: {
    class: '!p-[34px] max-[640px]:!p-6',
  },
  content: {
    class: '!p-0',
  },
} as const

const profileGridBgClass =
  'pointer-events-none fixed inset-0 z-0 overflow-hidden bg-[linear-gradient(135deg,var(--color-canvas-accent-wash)_0%,var(--color-canvas-accent-clear)_42%),radial-gradient(circle,#e2e4e8_1px,transparent_1px)] [background-size:100%_100%,12px_12px]'

const mainContentClass =
  'app-scrollbar flex flex-1 flex-col overflow-x-hidden overflow-y-auto px-[52px] py-11 max-[900px]:px-8 max-[640px]:px-5 max-[640px]:py-6'
const passwordFieldClass = 'flex min-w-0 flex-col gap-2 text-sm font-medium text-[#1d1d1f]'
const passwordInputClass =
  '!h-12 !w-full !rounded-xl !border-[#d2d2d7] !bg-white/90 !px-3 !text-[15px] !text-[#1d1d1f] !shadow-none placeholder:!text-[#86868b] enabled:hover:!border-canvas-accent enabled:focus:!border-canvas-accent'
const passwordInputErrorClass =
  '!border-rose-300 !bg-rose-50/50 placeholder:!text-rose-400 enabled:focus:!border-rose-400'

const emptyPasswordForm = (): PasswordForm => ({
  oldPassword: '',
  newPassword: '',
  confirmPassword: '',
})

const pwdForm = ref<PasswordForm>(emptyPasswordForm())
const pwdErrors = ref<Record<PasswordField, string>>(emptyPasswordForm())
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

function resolvePasswordFieldError(field: PasswordField) {
  const value = pwdForm.value[field]

  if (field === 'oldPassword' && !value.trim()) {
    return markRequiredError('请输入当前原密码')
  }
  if (field === 'newPassword') {
    if (!value.trim()) return markRequiredError('请输入新密码')
    if (value.length < 6) return '密码至少6个字符'
  }
  if (field === 'confirmPassword') {
    if (!value.trim()) return markRequiredError('请确认新密码')
    if (value !== pwdForm.value.newPassword) return '两次输入密码不一致'
  }
  return ''
}

function validatePasswordField(field: PasswordField) {
  pwdErrors.value[field] = resolvePasswordFieldError(field)
  return !pwdErrors.value[field]
}

function validatePasswordForm() {
  const fields: PasswordField[] = ['oldPassword', 'newPassword', 'confirmPassword']
  return fields.map((field) => validatePasswordField(field)).every(Boolean)
}

function isPasswordInlineError(field: PasswordField) {
  return isRequiredErrorMessage(pwdErrors.value[field])
}

function shouldShowPasswordMessage(field: PasswordField) {
  return Boolean(pwdErrors.value[field]) && !isPasswordInlineError(field)
}

function getPasswordDescribedby(field: PasswordField, errorId: string) {
  return shouldShowPasswordMessage(field) ? errorId : undefined
}

function clearPasswordFieldError(field: PasswordField) {
  if (pwdErrors.value[field]) {
    pwdErrors.value[field] = ''
  }
}

function handleNewPasswordInput() {
  clearPasswordFieldError('newPassword')
  if (pwdForm.value.confirmPassword && pwdErrors.value.confirmPassword) {
    validatePasswordField('confirmPassword')
  }
}

function resetPasswordForm() {
  pwdForm.value = emptyPasswordForm()
  pwdErrors.value = emptyPasswordForm()
}

async function handleUpdatePassword() {
  if (!validatePasswordForm()) return

  await runAsyncAction(
    async () => {
      await userStore.updatePassword(pwdForm.value.oldPassword, pwdForm.value.newPassword)
      resetPasswordForm()
      const isLoggedOut = await userStore.logout()
      if (!isLoggedOut) {
        throw new Error('密码已更新，但退出登录失败，请手动退出后重新登录')
      }
      appToast.success('密码更新成功，请重新登录')
      await router.replace('/user/login')
    },
    {
      loading: pwdLoading,
      errorMessage: (error) =>
        error instanceof Error && error.message ? error.message : '密码更新失败',
    },
  )
}

async function handleLogout() {
  await logoutAndRedirect('/user/home')
}

function openAvatarPicker() {
  avatarInputRef.value?.click()
}

async function onAvatarFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (!file.type.startsWith('image/')) {
    appToast.warning('仅支持上传图片文件')
    return
  }
  await runAsyncAction(
    async () => {
      const uploadRes = await uploadApi.uploadAvatar(file)
      const avatarUrl = uploadRes.data?.avatar?.trim()
      if (!avatarUrl) {
        throw new Error('头像上传失败，请稍后重试')
      }
      userStore.setAvatarLocal(avatarUrl)
      appToast.success('头像已更新')
    },
    {
      loading: avatarUploading,
      errorMessage: '头像更新失败',
    },
  )
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
