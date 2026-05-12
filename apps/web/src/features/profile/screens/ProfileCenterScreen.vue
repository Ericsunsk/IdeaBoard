<template>
  <div
    class="relative h-screen w-full overflow-hidden bg-[#f8f9fd] bg-[radial-gradient(circle,#e2e4e8_1.5px,transparent_1.5px)] bg-[length:24px_24px] [background-attachment:fixed] bg-center font-[-apple-system,BlinkMacSystemFont,'PingFang_SC','Segoe_UI',sans-serif] text-[#1f2430]"
  >
    <div
      class="relative z-[1] mx-auto my-[5vh] flex h-[90vh] max-w-[1200px] overflow-hidden rounded-[24px] border border-[#d9dfec] bg-[rgba(255,255,255,0.88)] backdrop-blur-[16px]"
    >
      <aside
        class="flex w-[260px] shrink-0 flex-col border-r border-[rgba(0,0,0,0.05)] px-[24px] py-[34px]"
      >
        <button
          class="mb-[34px] w-fit border-0 bg-transparent p-0 text-left text-[22px] leading-none font-[800] text-[#7a6bd5]"
          type="button"
          @click="router.push('/user/home')"
        >
          IdeaBoard
        </button>

        <div
          class="mb-[30px] rounded-[22px] border border-[#e4e8f3] bg-[rgba(255,255,255,0.72)] p-[18px]"
        >
          <div class="flex items-center gap-[12px]">
            <div
              class="flex h-[46px] w-[46px] shrink-0 items-center justify-center rounded-[16px] bg-[linear-gradient(135deg,#9278e6,#b087f4)] text-[22px] font-[800] text-[#ffffff]"
            >
              {{ userStore.user?.username?.[0]?.toUpperCase() }}
            </div>
            <div class="min-w-0">
              <div class="truncate text-[15px] leading-[1.2] font-[800] text-[#1d2333]">
                {{ userStore.user?.username || '-' }}
              </div>
              <div class="mt-[5px] text-[12px] leading-none font-[600] text-[#8a91a4]">
                {{ userStore.user?.role === 'admin' ? '管理员账户' : '创作者账户' }}
              </div>
            </div>
          </div>
          <div class="mt-[16px] flex items-center justify-between text-[12px]">
            <span class="font-[600] text-[#8a91a4]">会员编号</span>
            <span class="font-[800] text-[#5d52c9]">{{ userStore.user?.uid || '-' }}</span>
          </div>
        </div>

        <nav class="flex flex-1 flex-col gap-[10px]">
          <div
            v-for="item in navItems"
            :key="item.key"
            class="group flex h-[48px] cursor-pointer items-center gap-[12px] rounded-[16px] px-[14px] text-[14px] font-[700] transition-colors duration-200"
            :class="
              activeTab === item.key
                ? 'bg-[#7a6bd5] text-[#ffffff]'
                : 'text-[#86868b] hover:bg-[#eef1fa] hover:text-[#2c3448]'
            "
            @click="activeTab = item.key"
          >
            <span
              class="flex h-[26px] w-[26px] shrink-0 items-center justify-center rounded-[10px] transition-colors"
              :class="
                activeTab === item.key
                  ? 'bg-[rgba(255,255,255,0.18)]'
                  : 'bg-[#f1f3fa] text-[#7f86a0] group-hover:bg-[#e6eaf6]'
              "
            >
              <svg
                v-if="item.key === 'profile'"
                width="15"
                height="15"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2.2"
              >
                <circle cx="12" cy="8" r="4" />
                <path d="M4 21a8 8 0 0 1 16 0" />
              </svg>
              <svg
                v-else-if="item.key === 'tasks'"
                width="15"
                height="15"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2.2"
              >
                <path d="M4 5a2 2 0 0 1 2-2h8l6 6v10a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z" />
                <path d="M14 3v6h6" />
                <path d="M8 14h8" />
                <path d="M8 18h5" />
              </svg>
              <svg
                v-else
                width="15"
                height="15"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2.2"
              >
                <rect x="4" y="11" width="16" height="9" rx="2" />
                <path d="M8 11V7a4 4 0 0 1 8 0v4" />
              </svg>
            </span>
            <span>{{ item.label }}</span>
          </div>
        </nav>

        <div class="mt-auto flex flex-col gap-[10px]">
          <button
            v-if="userStore.user?.role === 'admin'"
            class="flex h-[44px] w-full items-center gap-[10px] rounded-[14px] border-0 bg-[#7a6bd5] px-[15px] text-[14px] font-[700] text-[#ffffff] transition-colors hover:bg-[#6c54c4]"
            @click="router.push('/admin/users')"
          >
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
            </svg>
            管理后台
          </button>

          <button
            class="flex h-[44px] w-full items-center gap-[10px] rounded-[14px] border-0 bg-[#f1f3f9] px-[15px] text-[14px] font-[700] text-[#4d556a] transition-colors hover:bg-[#e8ecf7]"
            @click="router.back()"
          >
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
            >
              <polyline points="15 18 9 12 15 6" />
            </svg>
            返回上级
          </button>

          <button
            class="flex h-[44px] w-full items-center gap-[10px] rounded-[14px] border-0 bg-transparent px-[15px] text-[14px] font-[700] text-[#ff3b30] transition-colors hover:bg-[rgba(255,59,48,0.06)]"
            @click="handleLogout"
          >
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
              <polyline points="16 17 21 12 16 7" />
              <line x1="21" y1="12" x2="9" y2="12" />
            </svg>
            退出登录
          </button>
        </div>
      </aside>

      <main
        class="custom-scrollbar flex flex-1 flex-col gap-[22px] overflow-x-hidden overflow-y-auto px-[38px] py-[34px]"
      >
        <section
          class="rounded-[24px] border border-[#e3e8f4] bg-[linear-gradient(135deg,rgba(255,255,255,0.92),rgba(247,248,255,0.84))] p-[26px]"
        >
          <div class="flex items-center justify-between gap-[24px]">
            <div class="flex min-w-0 items-center gap-[18px]">
              <div
                class="flex h-[82px] w-[82px] shrink-0 items-center justify-center overflow-hidden rounded-[24px] bg-[linear-gradient(135deg,#9278e6,#b072f1)] text-[34px] font-[800] text-[#ffffff] shadow-[0_18px_34px_rgba(122,107,213,0.22)]"
              >
                <img
                  v-if="userStore.user?.avatar"
                  class="h-full w-full object-cover"
                  :src="userStore.user.avatar"
                />
                <span v-else>{{ userStore.user?.username?.[0]?.toUpperCase() }}</span>
              </div>
              <div class="min-w-0">
                <div class="flex items-center gap-[10px]">
                  <h1 class="m-0 truncate text-[30px] leading-[1.05] font-[800] text-[#111827]">
                    {{ userStore.user?.username || '-' }}
                  </h1>
                  <span
                    class="inline-flex h-[26px] shrink-0 items-center rounded-full border border-[#b7eadf] bg-[rgba(93,216,184,0.08)] px-[10px] text-[12px] font-[800] text-[#5fd2b2]"
                  >
                    {{ userStore.user?.status === 1 ? '正常使用中' : '已封禁' }}
                  </span>
                </div>
                <p class="mt-[10px] text-[15px] leading-none font-[600] text-[#858b9c]">
                  {{ userStore.user?.role === 'admin' ? '管理员' : '创作者' }} · UID
                  {{ userStore.user?.uid || '-' }}
                </p>
              </div>
            </div>

            <div class="grid shrink-0 grid-cols-2 gap-[12px]">
              <div
                class="min-w-[112px] rounded-[18px] border border-[#e4e8f3] bg-[rgba(255,255,255,0.82)] px-[18px] py-[15px]"
              >
                <span class="block text-[24px] leading-none font-[800] text-[#1d1d1f]">
                  {{ taskPagination.total }}
                </span>
                <span class="mt-[10px] block text-[12px] leading-none font-[700] text-[#8b92a4]">
                  累计作品
                </span>
              </div>
              <div
                class="min-w-[112px] rounded-[18px] border border-[#e4e8f3] bg-[rgba(255,255,255,0.82)] px-[18px] py-[15px]"
              >
                <span class="block text-[24px] leading-none font-[800] text-[#1d1d1f]">
                  {{ userStore.user?.uid || '-' }}
                </span>
                <span class="mt-[10px] block text-[12px] leading-none font-[700] text-[#8b92a4]">
                  会员编号
                </span>
              </div>
            </div>
          </div>
        </section>

        <div class="flex-1">
          <Transition name="fade-slide" mode="out-in">
            <div
              v-if="activeTab === 'profile'"
              key="profile"
              class="grid grid-cols-[minmax(0,1fr)_260px] gap-[18px]"
            >
              <section class="rounded-[24px] border border-[#e2e7f3] bg-[#ffffff] p-[30px]">
                <div class="mb-[30px] flex items-center justify-between">
                  <div>
                    <h2 class="m-0 text-[22px] leading-none font-[800] text-[#1d1d1f]">
                      个人基本资料
                    </h2>
                    <p class="mt-[10px] text-[13px] leading-none font-[600] text-[#8a91a4]">
                      账号身份与基础信息
                    </p>
                  </div>
                  <span
                    class="rounded-full bg-[#f2efff] px-[12px] py-[7px] text-[12px] leading-none font-[800] text-[#6f5bd3]"
                  >
                    {{ userStore.user?.role === 'admin' ? 'Admin' : 'Creator' }}
                  </span>
                </div>

                <div class="grid grid-cols-2 gap-[14px]">
                  <div class="rounded-[18px] bg-[#f7f8fc] p-[18px]">
                    <label class="block text-[12px] leading-none font-[800] text-[#8b92a4]">
                      用户账号
                    </label>
                    <div
                      class="mt-[14px] truncate text-[18px] leading-none font-[800] text-[#1d1d1f]"
                    >
                      {{ userStore.user?.username || '-' }}
                    </div>
                  </div>

                  <div class="rounded-[18px] bg-[#f7f8fc] p-[18px]">
                    <label class="block text-[12px] leading-none font-[800] text-[#8b92a4]">
                      账号状态
                    </label>
                    <div class="mt-[12px]">
                      <span
                        class="inline-flex h-[30px] items-center rounded-full border border-[#a6ecdd] bg-[rgba(93,216,184,0.08)] px-[12px] text-[13px] font-[800] text-[#5fd2b2]"
                      >
                        {{ userStore.user?.status === 1 ? '正常使用中' : '已封禁' }}
                      </span>
                    </div>
                  </div>

                  <div class="col-span-2 rounded-[18px] bg-[#f7f8fc] p-[18px]">
                    <label class="block text-[12px] leading-none font-[800] text-[#8b92a4]">
                      加入时间
                    </label>
                    <div
                      class="mt-[14px] text-[18px] leading-[1.25] font-[700] break-all text-[#1d1d1f]"
                    >
                      {{ userStore.user?.created_at || '-' }}
                    </div>
                  </div>
                </div>
              </section>

              <aside class="flex flex-col gap-[14px]">
                <div
                  class="rounded-[24px] border border-[#e2e7f3] bg-[linear-gradient(180deg,#ffffff,#f8f7ff)] p-[22px]"
                >
                  <div class="text-[13px] leading-none font-[800] text-[#8b92a4]">身份权限</div>
                  <div class="mt-[18px] text-[26px] leading-none font-[800] text-[#1d1d1f]">
                    {{ userStore.user?.role === 'admin' ? 'Admin' : 'User' }}
                  </div>
                  <p class="mt-[14px] text-[13px] leading-[1.6] font-[600] text-[#7e8597]">
                    {{
                      userStore.user?.role === 'admin'
                        ? '可进入后台管理与内容工作区。'
                        : '可进入创作工作区。'
                    }}
                  </p>
                </div>

                <div class="rounded-[24px] border border-[#e2e7f3] bg-[#ffffff] p-[22px]">
                  <div class="flex items-center justify-between">
                    <span class="text-[13px] font-[800] text-[#8b92a4]">作品统计</span>
                    <span class="text-[22px] leading-none font-[800] text-[#7a6bd5]">
                      {{ taskPagination.total }}
                    </span>
                  </div>
                  <div class="mt-[18px] h-[8px] overflow-hidden rounded-full bg-[#eef1fa]">
                    <div
                      class="h-full w-[64%] rounded-full bg-[linear-gradient(90deg,#7a6bd5,#b087f4)]"
                    ></div>
                  </div>
                </div>
              </aside>
            </div>

            <div v-else-if="activeTab === 'tasks'" key="tasks">
              <section class="rounded-[24px] border border-[#e2e7f3] bg-[#ffffff] p-[30px]">
                <div class="mb-[24px] flex items-center justify-between">
                  <div>
                    <h2 class="m-0 text-[22px] leading-none font-[800] text-[#1d1d1f]">
                      我的作品记录
                    </h2>
                    <p class="mt-[10px] text-[13px] leading-none font-[600] text-[#8a91a4]">
                      最近生成与创作历史
                    </p>
                  </div>
                  <button
                    class="flex h-[36px] items-center gap-[7px] rounded-full border-0 bg-[#f2efff] px-[15px] text-[13px] font-[800] text-[#6f5bd3] transition-colors hover:bg-[#e9e3ff] disabled:cursor-not-allowed disabled:opacity-60"
                    :disabled="taskLoading"
                    @click="loadTasks"
                  >
                    <svg
                      :class="{ 'animate-spin': taskLoading }"
                      width="16"
                      height="16"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2"
                    >
                      <path d="M23 4v6h-6" />
                      <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10" />
                    </svg>
                    同步数据
                  </button>
                </div>
                <n-data-table
                  :columns="taskColumns"
                  :data="tasks"
                  :loading="taskLoading"
                  :pagination="taskPagination"
                  :bordered="false"
                  size="medium"
                  class="profile-table"
                />
              </section>
            </div>

            <div v-else-if="activeTab === 'security'" key="security">
              <section class="rounded-[24px] border border-[#e2e7f3] bg-[#ffffff] p-[30px]">
                <div class="mb-[28px]">
                  <h2 class="m-0 text-[22px] leading-none font-[800] text-[#1d1d1f]">
                    修改登录密码
                  </h2>
                  <p class="mt-[10px] text-[13px] leading-none font-[600] text-[#8a91a4]">
                    更新后需要重新登录
                  </p>
                </div>
                <n-form ref="pwdFormRef" :model="pwdForm" :rules="pwdRules" label-placement="top">
                  <div class="mb-[26px] flex flex-col gap-[16px]">
                    <n-form-item label="当前原密码" path="oldPassword">
                      <n-input
                        v-model:value="pwdForm.oldPassword"
                        type="password"
                        placeholder="验证您的当前密码"
                        show-password-on="click"
                        class="profile-input"
                      />
                    </n-form-item>
                    <div class="grid grid-cols-2 gap-[16px]">
                      <n-form-item label="设置新密码" path="newPassword">
                        <n-input
                          v-model:value="pwdForm.newPassword"
                          type="password"
                          placeholder="不少于6位字符"
                          show-password-on="click"
                          class="profile-input"
                        />
                      </n-form-item>
                      <n-form-item label="重复新密码" path="confirmPassword">
                        <n-input
                          v-model:value="pwdForm.confirmPassword"
                          type="password"
                          placeholder="请再次确认新密码"
                          show-password-on="click"
                          class="profile-input"
                        />
                      </n-form-item>
                    </div>
                  </div>
                  <button
                    class="flex h-[50px] w-full items-center justify-center rounded-[16px] border-0 bg-[#7a6bd5] text-[15px] font-[800] text-[#ffffff] transition-colors hover:bg-[#6c54c4] disabled:cursor-not-allowed disabled:opacity-50"
                    type="button"
                    :disabled="pwdLoading"
                    @click.prevent="handleUpdatePassword"
                  >
                    确认更新账户密码
                  </button>
                </n-form>
              </section>
            </div>
          </Transition>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import { useListUserTasks } from '../../../shared/api/generated-query/tasks/tasks'
import { NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'

const router = useRouter()
const userStore = useUserStore()
const message = useMessage()

const activeTab = ref('profile')

const navItems = [
  { key: 'profile', label: '个人资料' },
  { key: 'tasks', label: '我的作品' },
  { key: 'security', label: '账号安全' },
]

const taskPage = ref(1)
const taskPageSize = 8

const tasksQuery = useListUserTasks(
  computed(() => ({
    page: taskPage.value,
    page_size: taskPageSize,
  })),
  {
    query: {
      placeholderData: (prev) => prev,
    },
  },
)

const tasks = computed(() => tasksQuery.data.value?.data?.list || [])
const taskLoading = computed(() => tasksQuery.isLoading.value)

const taskPagination = computed(() => ({
  page: taskPage.value,
  pageSize: taskPageSize,
  total: tasksQuery.data.value?.data?.total || 0,
  onChange: (page: number) => {
    taskPage.value = page
  },
}))

const getStatusConfig = (status: string) => {
  switch (status) {
    case 'pending':
      return { type: 'default' as const, text: '等待中' }
    case 'running':
      return { type: 'warning' as const, text: '生成中' }
    case 'completed':
      return { type: 'success' as const, text: '已完成' }
    case 'failed':
      return { type: 'error' as const, text: '失败' }
    default:
      return { type: 'default' as const, text: status }
  }
}

const taskColumns: DataTableColumns<any> = [
  {
    title: '状态',
    key: 'status',
    width: 120,
    render: (row) => {
      const cfg = getStatusConfig(row.status)
      return h(NTag, { type: cfg.type, size: 'small', round: true, ghost: true }, () => cfg.text)
    },
  },
  {
    title: '作品描述',
    key: 'prompt',
    ellipsis: { tooltip: true },
    render: (row) =>
      h(
        'span',
        { style: { color: '#1d1d1f', fontWeight: '600' } },
        row.prompt?.replace(/^\[.*?\]\s*/, ''),
      ),
  },
  {
    title: '创作时间',
    key: 'created_at',
    width: 200,
    render: (row) => h('span', { style: { color: '#86868b' } }, row.created_at),
  },
]

function loadTasks() {
  tasksQuery.refetch()
}

const pwdFormRef = ref()
const pwdLoading = ref(false)
const pwdForm = ref({ oldPassword: '', newPassword: '', confirmPassword: '' })
const pwdRules = {
  oldPassword: { required: true, message: '请输入当前密码', trigger: 'blur' },
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, message: '密码至少6个字符', trigger: 'blur' },
  ],
  confirmPassword: [
    { required: true, message: '请再次输入新密码', trigger: 'blur' },
    {
      validator: (_rule: any, value: string) => value === pwdForm.value.newPassword,
      message: '两次密码不一致',
      trigger: 'blur',
    },
  ],
}

async function handleUpdatePassword() {
  try {
    await pwdFormRef.value?.validate()
    pwdLoading.value = true
    await userStore.updatePassword(pwdForm.value.oldPassword, pwdForm.value.newPassword)
    message.success('更新成功，请重新登录')
    pwdForm.value = { oldPassword: '', newPassword: '', confirmPassword: '' }
    await router.push('/user/login')
  } catch (error: any) {
    if (error.message) message.error(error.message)
  } finally {
    pwdLoading.value = false
  }
}

async function handleLogout() {
  await userStore.logout('/user/home')
}

onMounted(() => {
  userStore.fetchUserInfo()
})
</script>

<style scoped>
.custom-scrollbar::-webkit-scrollbar {
  width: 6px;
}
.custom-scrollbar::-webkit-scrollbar-thumb {
  background: rgba(122, 107, 213, 0.2);
  border-radius: 10px;
}

:deep(.profile-input.n-input) {
  --n-border-radius: 12px;
  --n-height: 48px;
  --n-color: #f8fafc;
  --n-border: 1px solid #e2e8f0;
}

.profile-table :deep(.n-data-table-th) {
  background-color: #f8fafc;
  font-weight: 700;
  color: #64748b;
  border-bottom: none;
}

.profile-table :deep(.n-data-table-td) {
  padding: 16px 12px;
  border-bottom: 1px solid #f1f5f9;
}

.profile-table :deep(.n-pagination-item--active) {
  background-color: #7a6bd5;
  border-color: #7a6bd5;
  color: white;
}

.fade-slide-enter-active,
.fade-slide-leave-active {
  transition: all 0.3s ease;
}
.fade-slide-enter-from {
  opacity: 0;
  transform: translateY(10px);
}
.fade-slide-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}
</style>
