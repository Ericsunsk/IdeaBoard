<template>
  <div class="center-page relative h-screen w-full overflow-hidden bg-[#f8f9fd] text-[#1f2430]">
    <div :class="profileGridBgClass"></div>

    <div
      class="relative z-[1] mx-auto my-[5vh] flex h-[90vh] max-w-[1200px] overflow-hidden rounded-[24px] border border-[#d9dfec] bg-white/[0.82] shadow-[0_24px_80px_rgba(50,62,96,0.08)] backdrop-blur-[16px] max-[900px]:my-0 max-[900px]:h-screen max-[900px]:max-w-none max-[900px]:rounded-none"
    >
      <aside
        class="flex w-[260px] shrink-0 flex-col border-r border-black/5 px-6 py-10 max-[760px]:w-[220px] max-[640px]:hidden"
      >
        <div class="mb-12">
          <button
            type="button"
            class="cursor-pointer border-0 bg-[linear-gradient(135deg,#7f68d5,#9f8fe6)] bg-clip-text p-0 text-[22px] leading-none font-bold text-transparent"
            @click="router.push('/user/home')"
          >
            IdeaBoard
          </button>
        </div>

        <nav class="flex flex-1 flex-col gap-2" aria-label="个人中心导航">
          <button
            v-for="item in navItems"
            :key="item.key"
            type="button"
            class="flex cursor-pointer items-center gap-3 rounded-[14px] border-0 px-4 py-3 text-left text-[15px] font-medium transition duration-200"
            :class="
              activeTab === item.key
                ? 'bg-[#7a6bd5] text-white'
                : 'bg-transparent text-[#86868b] hover:bg-[#eef1fa] hover:text-[#2c3448]'
            "
            @click="activeTab = item.key"
          >
            <span class="flex h-5 w-5 items-center justify-center">
              <component :is="item.icon" class="h-5 w-5 stroke-[2]" />
            </span>
            <span>{{ item.label }}</span>
          </button>
        </nav>

        <div class="mt-auto flex flex-col gap-3">
          <button
            v-if="userStore.user?.role === 'admin'"
            type="button"
            class="flex w-full cursor-pointer items-center gap-2.5 rounded-xl border-0 bg-[linear-gradient(135deg,#7f68d5,#9f8fe6)] px-4 py-3 text-[15px] font-semibold text-white transition duration-200 hover:bg-[linear-gradient(135deg,#6c54c4,#8b7ad6)]"
            @click="router.push('/admin/users')"
          >
            <Shield class="h-[18px] w-[18px] stroke-[2.1]" />
            管理后台
          </button>

          <button
            type="button"
            class="flex w-full cursor-pointer items-center gap-2.5 rounded-xl border-0 bg-[#f1f3f9] px-4 py-3 text-[15px] font-semibold text-[#4d556a] transition duration-200 hover:bg-[#e8ecf7] hover:text-[#2a3142]"
            @click="router.back()"
          >
            <NavArrowLeft class="h-[18px] w-[18px] stroke-[2.4]" />
            返回上级
          </button>

          <button
            type="button"
            class="flex w-full cursor-pointer items-center gap-2.5 rounded-xl border-0 bg-transparent px-4 py-3 text-[15px] font-semibold text-[#ff3b30] transition duration-200 hover:bg-[rgba(255,59,48,0.05)]"
            @click="handleLogout"
          >
            <LogOut class="h-[18px] w-[18px] stroke-[2.1]" />
            退出登录
          </button>
        </div>
      </aside>

      <main
        class="flex flex-1 flex-col overflow-x-hidden overflow-y-auto px-[52px] py-11 [scrollbar-color:rgba(146,120,230,0.38)_transparent] [scrollbar-width:thin] max-[900px]:px-8 max-[640px]:px-5 max-[640px]:py-6"
      >
        <div
          class="mb-10 flex items-center justify-between gap-6 max-[720px]:flex-col max-[720px]:items-start"
        >
          <div class="flex items-center gap-5">
            <div
              class="relative h-20 w-20 overflow-visible rounded-full border-4 border-white bg-white max-[520px]:h-[72px] max-[520px]:w-[72px]"
            >
              <div class="h-full w-full overflow-hidden rounded-full">
                <img
                  v-if="userStore.user?.avatar"
                  :src="userStore.user.avatar"
                  class="h-full w-full object-cover"
                  alt="用户头像"
                />
                <div
                  v-else
                  class="flex h-full w-full items-center justify-center bg-[linear-gradient(135deg,#9278e6,#c084fc)] text-[32px] font-bold text-white"
                >
                  {{ userStore.user?.username?.[0]?.toUpperCase() }}
                </div>
              </div>
              <button
                type="button"
                class="absolute right-[-4px] bottom-[-4px] flex h-7 w-7 cursor-pointer items-center justify-center rounded-full border border-[#d9dfec] bg-white/[0.98] text-[#6b7290] transition duration-200 hover:border-[#c8d0e2] hover:bg-[#f8faff] hover:text-[#4a5169] disabled:cursor-not-allowed disabled:opacity-60"
                :disabled="avatarUploading"
                title="更换头像"
                @click="openAvatarPicker"
              >
                <Camera class="h-[15px] w-[15px] stroke-[2]" />
              </button>
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
              <h1 class="m-0 truncate text-[28px] leading-tight font-bold tracking-normal text-[#1d1d1f]">
                {{ userStore.user?.username }}
              </h1>
              <p class="mt-1 mb-0 text-[15px] text-[#86868b]">
                {{ userStore.user?.role === 'admin' ? '管理员' : '创作者' }}
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

        <div class="flex-1">
          <Transition
            mode="out-in"
            enter-active-class="transition-all duration-300 ease-out"
            enter-from-class="translate-x-5 opacity-0"
            enter-to-class="translate-x-0 opacity-100"
            leave-active-class="transition-all duration-300 ease-in"
            leave-from-class="translate-x-0 opacity-100"
            leave-to-class="-translate-x-5 opacity-0"
          >
            <div v-if="activeTab === 'profile'" key="profile">
              <Card :pt="profileCardPt">
                <template #content>
                  <h2 class="m-0 mb-8 text-xl font-semibold text-[#1d1d1f]">个人基本资料</h2>
                  <div class="grid grid-cols-2 gap-8 max-[720px]:grid-cols-1">
                    <div class="min-w-0">
                      <label class="mb-2 block text-[13px] font-medium text-[#86868b]">用户账号</label>
                      <div class="truncate text-base font-medium text-[#1d1d1f]">
                        {{ userStore.user?.username }}
                      </div>
                    </div>
                    <div>
                      <label class="mb-2 block text-[13px] font-medium text-[#86868b]">账号状态</label>
                      <div>
                        <Tag
                          :value="userStatusLabel"
                          :severity="userStatusSeverity"
                          rounded
                          class="!bg-white"
                        />
                      </div>
                    </div>
                    <div>
                      <label class="mb-2 block text-[13px] font-medium text-[#86868b]">加入时间</label>
                      <div class="text-base font-medium text-[#1d1d1f]">
                        {{ formattedJoinedAt }}
                      </div>
                    </div>
                  </div>
                </template>
              </Card>
            </div>

            <div v-else-if="activeTab === 'security'" key="security">
              <Card :pt="profileCardPt">
                <template #content>
                  <h2 class="m-0 mb-8 text-xl font-semibold text-[#1d1d1f]">账号安全设置</h2>
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
                            'aria-describedby': 'profile-old-password-error',
                          }"
                          :invalid="Boolean(pwdErrors.oldPassword)"
                          :input-class="passwordInputClass"
                          placeholder="请输入当前使用的密码"
                          toggle-mask
                          @blur="validatePasswordField('oldPassword')"
                          @update:model-value="clearPasswordFieldError('oldPassword')"
                        />
                        <Message
                          v-if="pwdErrors.oldPassword"
                          id="profile-old-password-error"
                          class="!mt-1 !rounded-xl"
                          severity="error"
                          size="small"
                          :closable="false"
                        >
                          {{ pwdErrors.oldPassword }}
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
                              'aria-describedby': 'profile-new-password-error',
                            }"
                            :invalid="Boolean(pwdErrors.newPassword)"
                            :input-class="passwordInputClass"
                            placeholder="不少于6位字符"
                            toggle-mask
                            @blur="validatePasswordField('newPassword')"
                            @update:model-value="handleNewPasswordInput"
                          />
                          <Message
                            v-if="pwdErrors.newPassword"
                            id="profile-new-password-error"
                            class="!mt-1 !rounded-xl"
                            severity="error"
                            size="small"
                            :closable="false"
                          >
                            {{ pwdErrors.newPassword }}
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
                              'aria-describedby': 'profile-confirm-password-error',
                            }"
                            :invalid="Boolean(pwdErrors.confirmPassword)"
                            :input-class="passwordInputClass"
                            placeholder="请再次输入新密码"
                            toggle-mask
                            @blur="validatePasswordField('confirmPassword')"
                            @update:model-value="clearPasswordFieldError('confirmPassword')"
                          />
                          <Message
                            v-if="pwdErrors.confirmPassword"
                            id="profile-confirm-password-error"
                            class="!mt-1 !rounded-xl"
                            severity="error"
                            size="small"
                            :closable="false"
                          >
                            {{ pwdErrors.confirmPassword }}
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
                      class="!h-12 !rounded-[14px] !border-0 !bg-[#7a6bd5] !text-[15px] !font-semibold !text-white !shadow-[0_8px_24px_rgba(122,107,213,0.24)] transition hover:!-translate-y-0.5 hover:!bg-[#6d5fd0] hover:!shadow-[0_12px_32px_rgba(122,107,213,0.32)] active:!translate-y-0"
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
import { useAppToast } from '../../../shared/composables/useAppToast'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { uploadApi } from '../../../shared/api/upload'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Tag from 'primevue/tag'
import { formatDisplayErrorMessage } from '../../../shared/utils/errorMessages'
import type { Component } from 'vue'
import { Camera, Lock, LogOut, NavArrowLeft, Shield, User } from '@iconoir/vue'

type NavItem = {
  key: 'profile' | 'security'
  label: string
  icon: Component
}

type PasswordField = 'oldPassword' | 'newPassword' | 'confirmPassword'
type PasswordForm = Record<PasswordField, string>

const router = useRouter()
const userStore = useUserStore()
const appToast = useAppToast()
const { logoutAndRedirect } = useLogoutRedirect()

const activeTab = ref<NavItem['key']>('profile')
const avatarInputRef = ref<HTMLInputElement | null>(null)
const avatarUploading = ref(false)
const pwdLoading = ref(false)

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
  'pointer-events-none fixed inset-0 z-0 overflow-hidden bg-[linear-gradient(135deg,rgba(122,107,213,0.08)_0%,rgba(122,107,213,0)_42%),radial-gradient(circle,#e2e4e8_1.5px,transparent_1.5px)] [background-size:100%_100%,24px_24px]'

const passwordFieldClass = 'flex min-w-0 flex-col gap-2 text-sm font-medium text-[#1d1d1f]'
const passwordInputClass =
  '!h-12 !w-full !rounded-xl !border-[#d2d2d7] !bg-white/90 !px-3 !text-[15px] !text-[#1d1d1f] !shadow-none placeholder:!text-[#86868b] enabled:hover:!border-[#9278e6] enabled:focus:!border-[#9278e6]'

const emptyPasswordForm = (): PasswordForm => ({
  oldPassword: '',
  newPassword: '',
  confirmPassword: '',
})

const pwdForm = ref<PasswordForm>(emptyPasswordForm())
const pwdErrors = ref<Record<PasswordField, string>>(emptyPasswordForm())
const formattedJoinedAt = computed(() => formatDateWithDots(userStore.user?.created_at))
const userStatusLabel = computed(() => (userStore.user?.status === 1 ? '正常使用中' : '已封禁'))
const userStatusSeverity = computed(() => (userStore.user?.status === 1 ? 'success' : 'warn'))

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
    return '请输入当前原密码'
  }
  if (field === 'newPassword') {
    if (!value.trim()) return '请输入新密码'
    if (value.length < 6) return '密码至少6个字符'
  }
  if (field === 'confirmPassword') {
    if (!value.trim()) return '请确认新密码'
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

  try {
    pwdLoading.value = true
    await userStore.updatePassword(pwdForm.value.oldPassword, pwdForm.value.newPassword)
    appToast.success('密码更新成功，请重新登录')
    resetPasswordForm()
    await router.push('/user/login')
  } catch (error: unknown) {
    appToast.error(error instanceof Error && error.message ? error.message : '密码更新失败')
  } finally {
    pwdLoading.value = false
  }
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
  try {
    avatarUploading.value = true
    const uploadRes = await uploadApi.uploadAvatar(file)
    const avatarUrl = uploadRes.data?.avatar?.trim()
    if (!avatarUrl) {
      throw new Error('头像上传失败，请稍后重试')
    }
    userStore.setAvatarLocal(avatarUrl)
    appToast.success('头像已更新')
  } catch (error) {
    appToast.error(formatDisplayErrorMessage(error, '头像更新失败'))
  } finally {
    avatarUploading.value = false
  }
}

onMounted(() => {
  void userStore.fetchUserInfo()
})
</script>
