<template>
  <div class="flex max-w-full min-w-0 flex-col gap-4 p-0 pb-6">
    <Toolbar>
      <template #start>
        <div class="flex flex-wrap items-center gap-2.5">
          <IconField class="relative w-full sm:w-[300px]">
            <InputIcon>
              <Search />
            </InputIcon>
            <InputText v-model="searchKeyword" placeholder="搜索渠道商名称..." fluid />
            <Button
              v-if="searchKeyword"
              text
              rounded
              class="absolute top-1/2 right-1 !h-8 !w-8 -translate-y-1/2"
              aria-label="清空搜索"
              @click="searchKeyword = ''"
            >
              <template #icon>
                <Xmark />
              </template>
            </Button>
          </IconField>
          <Button text rounded aria-label="刷新渠道商" @click="loadData">
            <template #icon>
              <Refresh />
            </template>
          </Button>
        </div>
      </template>
      <template #end>
        <Button label="新增渠道商" @click="openAddChannelModal">
          <template #icon>
            <Plus />
          </template>
        </Button>
      </template>
    </Toolbar>

    <div
      v-if="loading && channels.length === 0"
      class="flex items-center justify-center py-[60px] text-admin-content text-admin-text-3"
    >
      <ProgressSpinner class="!h-9 !w-9 text-admin-accent" stroke-width="4" />
    </div>

    <div v-else class="grid grid-cols-1 gap-3.5 lg:grid-cols-2">
      <Card
        v-for="channel in filteredChannels"
        :key="channel.id"
      >
        <template #content>
          <div class="flex min-h-[340px] flex-col gap-3.5">
            <Toolbar class="border-0 bg-transparent p-0">
              <template #start>
                <Avatar
                  :label="(channel.name || 'CH').substring(0, 2).toUpperCase()"
                  shape="circle"
                  class="!h-10 !w-10 !bg-admin-accent !text-sm !font-bold !text-admin-text-inverse"
                />
                <div class="ml-3 min-w-0">
                  <div class="truncate text-admin-title-sm font-bold text-admin-title-color">
                    {{ channel.name }}
                  </div>
                  <div class="truncate text-admin-meta text-admin-subtitle-color">
                    {{ channel.api_url }}
                  </div>
                </div>
              </template>
              <template #end>
                <Tag
                  :value="getChannelStatus(channel)"
                  :severity="getChannelStatusSeverity(channel)"
                  rounded
                />
              </template>
            </Toolbar>

            <div class="flex min-h-[30px] items-center justify-between gap-2.5">
              <span class="text-admin-caption font-bold text-admin-text-3">接口类型</span>
              <Tag :value="getApiTypeLabel(channel.type)" severity="info" rounded />
            </div>

            <div class="rounded-xl border border-admin-border bg-admin-surface-muted p-2.5">
              <div class="mb-2 text-admin-caption font-bold text-admin-text-3">当前生效模型</div>
              <div v-if="getChannelActiveModels(channel.id).length > 0" class="flex flex-wrap gap-1.5">
                <Tag
                  v-for="model in getChannelActiveModels(channel.id)"
                  :key="model"
                  :value="model"
                  severity="secondary"
                  rounded
                />
              </div>
              <div v-else class="text-admin-meta text-admin-text-3">未配置生效模型</div>
            </div>

            <div class="flex flex-1 flex-col gap-3">
              <div class="flex items-center justify-between">
                <span class="text-admin-meta font-bold text-admin-title-color">
                  关联 APIKey ({{ getChannelAccounts(channel.id).length }})
                </span>
                <Button
                  text
                  rounded
                  size="small"
                  aria-label="添加 APIKey"
                  @click="openAddAccountModal(channel)"
                >
                  <template #icon>
                    <Plus />
                  </template>
                </Button>
              </div>

              <div
                v-if="getChannelAccounts(channel.id).length > 0"
                class="flex max-h-[200px] flex-col gap-2 overflow-y-auto pr-1"
              >
                <div
                  v-for="account in getChannelAccounts(channel.id)"
                  :key="account.id"
                  class="rounded-[10px] border border-admin-border bg-admin-surface-muted px-2.5 py-2"
                >
                  <div class="mb-1.5 flex items-center justify-between">
                    <div class="flex items-center gap-2">
                      <div
                        class="h-1.5 w-1.5 rounded-full"
                        :class="getAccountStatusDotClass(account)"
                      ></div>
                      <span class="text-admin-meta font-semibold text-admin-title-color">
                        {{ account.model_set || '未分配' }}
                      </span>
                    </div>
                    <div>
                      <span class="font-mono text-admin-caption font-bold text-admin-text-3">
                        {{ account.current_concurrent || 0 }}/{{ account.max_concurrent }}
                      </span>
                    </div>
                  </div>
                  <ProgressBar
                    :value="
                      ((account.current_concurrent || 0) / (account.max_concurrent || 1)) * 100
                    "
                    :show-value="false"
                    class="mb-2 !h-[3px] !rounded-full !bg-admin-border-soft"
                    :pt="getAccountProgressPt(account)"
                  />
                  <div class="flex justify-end gap-1">
                    <Button size="small" text label="编辑" @click="openEditAccountModal(account)" />
                    <Button
                      size="small"
                      text
                      :severity="account.status === 1 ? 'warn' : 'success'"
                      :label="account.status === 1 ? '停止' : '启动'"
                      @click="toggleAccountStatus(account)"
                    />
                    <Button
                      size="small"
                      text
                      severity="danger"
                      label="删除"
                      @click="handleDeleteAccount(account)"
                    />
                  </div>
                </div>
              </div>
              <Message v-else severity="secondary" :closable="false">
                点击上方按钮添加 APIKey
              </Message>
            </div>
            <div class="mt-auto flex flex-wrap gap-2 border-t border-admin-border pt-3">
              <Button
                size="small"
                label="配置详情"
                severity="secondary"
                outlined
                @click="openEditChannelModal(channel)"
              />
              <Button
                size="small"
                label="删除"
                severity="danger"
                text
                @click="handleDeleteChannel(channel)"
              />
            </div>
          </div>
        </template>
      </Card>

      <Card>
        <template #content>
          <Button
            severity="secondary"
            outlined
            class="!flex !min-h-[340px] !w-full !flex-col !items-center !justify-center !gap-2.5"
            @click="openAddChannelModal"
          >
            <template #icon>
              <Plus />
            </template>
            <span>添加渠道商</span>
          </Button>
        </template>
      </Card>

      <!-- Empty state -->
      <div
        v-if="filteredChannels.length === 0"
        class="col-span-full px-[18px] py-9 text-center text-admin-text-3"
      >
        无匹配的渠道商
      </div>
    </div>

    <!-- 渠道商表单弹窗 -->
    <Dialog
      v-model:visible="showChannelModal"
      modal
      :header="isEditChannel ? '编辑渠道商' : '新增渠道商'"
      :style="{ width: '480px' }"
    >
      <div class="flex flex-col gap-3.5">
        <label class="flex min-w-0 flex-col gap-[7px] text-admin-meta font-semibold text-admin-text-2">
          <span>渠道商名称</span>
          <InputText v-model="channelForm.name" class="w-full" placeholder="例如：Nano Banana 官方" />
        </label>
        <label class="flex min-w-0 flex-col gap-[7px] text-admin-meta font-semibold text-admin-text-2">
          <span>接口类型</span>
          <Select
            v-model="channelForm.type"
            class="w-full"
            :options="apiTypeOptions"
            option-label="label"
            option-value="value"
            placeholder="选择接口协议"
          />
        </label>
        <label class="flex min-w-0 flex-col gap-[7px] text-admin-meta font-semibold text-admin-text-2">
          <span>Base URL地址</span>
          <InputText
            v-model="channelForm.api_url"
            class="w-full"
            placeholder="例如：https://api.example.com/v1"
            @blur="inferChannelApiTypeFromUrl"
          />
        </label>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2.5">
          <Button label="取消" severity="secondary" outlined @click="showChannelModal = false" />
          <Button label="确定" :loading="channelSaveLoading" @click="handleSaveChannel" />
        </div>
      </template>
    </Dialog>

    <!-- APIKey 配置弹窗 -->
    <Dialog
      v-model:visible="showAccountModal"
      modal
      :header="isEditAccount ? '编辑 APIKey' : '新增 APIKey'"
      :style="{ width: '500px' }"
    >
      <div class="flex flex-col gap-3.5">
        <div
          v-if="selectedChannelForAccount"
          class="mb-4 flex items-center gap-1.5 rounded-xl border border-admin-border bg-admin-surface-muted px-3 py-2 text-admin-meta text-admin-text-2"
        >
          <span>所属渠道：</span>
          <code class="font-semibold text-admin-text-1">{{ selectedChannelForAccount.name }}</code>
        </div>

        <label class="flex min-w-0 flex-col gap-[7px] text-admin-meta font-semibold text-admin-text-2">
          <span>{{ isEditAccount ? '修改 API 密钥 (留空不修改)' : 'API 密钥 (Key)' }}</span>
          <InputText
            v-model="accountForm.api_key"
            class="w-full"
            type="password"
            :placeholder="isEditAccount ? '留空则保持原密钥' : '输入此渠道商的 API Key'"
          />
        </label>

        <label class="flex min-w-0 flex-col gap-[7px] text-admin-meta font-semibold text-admin-text-2">
          <span>分配模型</span>
          <AutoComplete
            v-model="accountForm.model_values"
            multiple
            fluid
            :suggestions="modelSuggestions"
            :loading="detectModelsLoading"
            placeholder="选择或输入分配给该 APIKey 的模型"
            :pt="modelSelectPt"
            @focus="handleAccountModelSelectShow(true)"
            @complete="searchModelSuggestions"
          />
        </label>

        <div class="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
          <label class="flex min-w-0 flex-col gap-[7px] text-admin-meta font-semibold text-admin-text-2">
            <span>最大并发数</span>
            <InputNumber
              v-model="accountForm.max_concurrent"
              :min="accountLimits.max_concurrent.min"
              :max="accountLimits.max_concurrent.max"
              fluid
            />
          </label>
          <label class="flex min-w-0 flex-col gap-[7px] text-admin-meta font-semibold text-admin-text-2">
            <span>超时时间(秒)</span>
            <InputNumber
              v-model="accountForm.timeout"
              :min="accountLimits.timeout_seconds.min"
              :max="accountLimits.timeout_seconds.max"
              fluid
            />
          </label>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2.5">
          <Button label="取消" severity="secondary" outlined @click="showAccountModal = false" />
          <Button label="确定" :loading="accountSaveLoading" @click="handleSaveAccount" />
        </div>
      </template>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import channelApi from '../../../../shared/api/channel'
import type { Channel, ChannelFormData } from '../../../../shared/api/channel'
import type { AdminAccount } from '../../shared/api/adminAccounts'
import AutoComplete from 'primevue/autocomplete'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Dialog from 'primevue/dialog'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ProgressBar from 'primevue/progressbar'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Toolbar from 'primevue/toolbar'
import { useConfirm } from 'primevue/useconfirm'
import { Plus, Refresh, Search, Xmark } from '@iconoir/vue'
import { useAppToast } from '../../../../shared/composables/useAppToast'
import { extractApiErrorMessage } from './channelModelUtils'
import { useChannelAccounts } from './useChannelAccounts'

const appToast = useAppToast()
const confirm = useConfirm()

// State
const loading = ref(false)
const channels = ref<Channel[]>([])
const searchKeyword = ref('')

const filteredChannels = computed(() => {
  const keyword = searchKeyword.value.trim().toLowerCase()
  if (!keyword) return channels.value
  return channels.value.filter((item) => (item.name || '').toLowerCase().includes(keyword))
})

// Data Loading
async function loadData() {
  loading.value = true
  try {
    const chRes = await channelApi.list()

    if (chRes.code === 0) channels.value = chRes.data?.list || []
    await loadAccounts()
  } catch (error) {
    appToast.error(extractApiErrorMessage(error, '加载数据失败'))
  } finally {
    loading.value = false
  }
}

// Helpers
function getChannelStatus(channel: Channel) {
  const accs = getChannelAccounts(channel.id)
  if (accs.length === 0) return '无账号'
  const active = accs.filter((a) => a.status === 1).length
  return `${active} / ${accs.length} 运行中`
}

function getChannelStatusSeverity(channel: Channel) {
  const accs = getChannelAccounts(channel.id)
  if (accs.length === 0) return 'warn'
  const active = accs.filter((a) => a.status === 1 && a.fail_count < 5).length
  if (active === 0) return 'secondary'
  if (active < accs.length) return 'info'
  return 'success'
}

// --- Channel Form ---
const showChannelModal = ref(false)
const isEditChannel = ref(false)
const channelSaveLoading = ref(false)
const currentChannelId = ref(0)
const createEmptyChannelForm = (): ChannelFormData => ({
  type: 'openai-compatible',
  name: '',
  api_url: '',
})
const channelForm = ref<ChannelFormData>(createEmptyChannelForm())
const apiTypeOptions = [
  { label: 'OpenAI Compatible', value: 'openai-compatible' },
  { label: 'Google', value: 'google-ai' },
]
const apiTypeLabelByValue = new Map(apiTypeOptions.map((item) => [item.value, item.label]))
const modelSelectPt = {
  root: { class: 'w-full' },
  inputMultiple: { class: '!min-h-[42px] !w-full !rounded-xl' },
  pcChip: {
    root: {
      class:
        '!rounded-full !border !border-admin-border !bg-admin-surface-muted !text-admin-text-2-strong',
    },
  },
  input: { class: 'text-admin-body text-admin-text-1' },
} as const
const activeProgressPt = { value: { class: '!bg-admin-success' } } as const
const mutedProgressPt = { value: { class: '!bg-admin-text-3' } } as const

function getApiTypeLabel(value: string) {
  return apiTypeLabelByValue.get(value) || value || '未配置'
}

function getAccountStatusDotClass(account: AdminAccount) {
  if (account.status === 0) return 'bg-admin-warning'
  if (account.fail_count >= 5) return 'bg-admin-danger'
  return 'bg-admin-success'
}

function getAccountProgressPt(account: AdminAccount) {
  return account.status === 1 ? activeProgressPt : mutedProgressPt
}

function inferChannelApiTypeFromUrl() {
  const apiUrl = channelForm.value.api_url.trim().toLowerCase()
  if (!apiUrl) return
  if (apiUrl.includes('generativelanguage.googleapis.com')) {
    channelForm.value.type = 'google-ai'
    return
  }
  if (apiUrl.includes('api.openai.com')) {
    channelForm.value.type = 'openai-compatible'
  }
}

function openAddChannelModal() {
  isEditChannel.value = false
  channelForm.value = createEmptyChannelForm()
  showChannelModal.value = true
}

function openEditChannelModal(row: Channel) {
  isEditChannel.value = true
  currentChannelId.value = row.id
  channelForm.value = { type: row.type, name: row.name, api_url: row.api_url }
  showChannelModal.value = true
}

async function handleSaveChannel() {
  try {
    const validationError = validateChannelForm()
    if (validationError) {
      appToast.warning(validationError)
      return
    }
    channelSaveLoading.value = true
    const submitData = { ...channelForm.value }

    if (isEditChannel.value) {
      await channelApi.update(currentChannelId.value, submitData)
      appToast.success('渠道商已更新')
    } else {
      await channelApi.create(submitData)
      appToast.success('渠道商已创建')
    }
    showChannelModal.value = false
    loadData()
  } catch (error) {
    appToast.error(extractApiErrorMessage(error, '保存渠道商失败'))
  } finally {
    channelSaveLoading.value = false
  }
}

function validateChannelForm() {
  if (!channelForm.value.name.trim()) return '请输入渠道商名称'
  if (!channelForm.value.type) return '请选择接口类型'
  if (!channelForm.value.api_url.trim()) return '请输入 Base URL地址'
  return ''
}

function handleDeleteChannel(row: Channel) {
  requireAdminConfirm({
    header: '确认删除',
    message: `确定删除渠道商「${row.name}」吗？此操作不可恢复。`,
    acceptLabel: '确定',
    accept: async () => {
      try {
        await channelApi.delete(row.id)
        appToast.success('删除成功')
        await loadData()
      } catch (error) {
        appToast.error(extractApiErrorMessage(error, '删除渠道商失败'))
      }
    },
  })
}

function requireAdminConfirm(options: {
  header: string
  message: string
  acceptLabel: string
  accept: () => void | Promise<void>
}) {
  confirm.require({
    header: options.header,
    message: options.message,
    rejectLabel: '取消',
    acceptLabel: options.acceptLabel,
    acceptProps: {
      severity: 'danger',
    },
    rejectProps: {
      severity: 'secondary',
      outlined: true,
    },
    accept: () => {
      void options.accept()
    },
  })
}

const {
  showAccountModal,
  isEditAccount,
  accountSaveLoading,
  accountLimits,
  selectedChannelForAccount,
  accountForm,
  detectModelsLoading,
  modelSuggestions,
  loadAccounts,
  getChannelAccounts,
  getChannelActiveModels,
  openAddAccountModal,
  openEditAccountModal,
  handleDetectModelsOnSelectOpen,
  searchModelSuggestions,
  handleSaveAccount,
  toggleAccountStatus,
  handleDeleteAccount,
} = useChannelAccounts({
  channels,
  message: appToast,
  dialog: {
    warning: ({ title, content, onConfirm }) =>
      requireAdminConfirm({
        header: title,
        message: content,
        acceptLabel: '确定',
        accept: onConfirm,
      }),
  },
  onAfterMutation: loadData,
})

function handleAccountModelSelectShow(show: boolean) {
  if (!show) return
  void handleDetectModelsOnSelectOpen()
}

onMounted(() => {
  loadData()
})
</script>
