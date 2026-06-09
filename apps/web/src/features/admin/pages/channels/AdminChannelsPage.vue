<template>
  <div>
    <AdminManagementFrame
      :is-scrolling="isChannelPanelScrolling"
      @scroll="handleChannelPanelScroll"
    >
      <template #toolbar-start>
        <AdminToolbarSearch v-model="searchKeyword" />
        <span :class="adminToolbarMetricClass">
          <span class="h-2.5 w-2.5 shrink-0 rounded-full bg-admin-success"></span>
          <span>{{ totalChannelsLabel }}</span>
        </span>
        <Tag :value="accountRunningLabel" severity="success" rounded />
        <Tag :value="`${activeModelCount} 模型`" severity="info" rounded />
      </template>

      <template #toolbar-end>
        <Button text :class="adminToolbarIconButtonClass" aria-label="刷新渠道商" @click="loadData">
          <template #icon>
            <Refresh class="size-4 text-admin-text-2" />
          </template>
        </Button>
        <Button label="新增渠道商" class="!h-9 !rounded-lg" @click="openAddChannelModal">
          <template #icon>
            <Plus class="size-4" />
          </template>
        </Button>
      </template>

      <div v-if="loading && channels.length === 0" :class="adminLoadingStateClass">
        <ProgressSpinner class="!h-9 !w-9 text-admin-accent" stroke-width="4" />
      </div>

      <div v-else-if="filteredChannels.length === 0" :class="adminEmptyStateClass">
        <div class="text-admin-title font-semibold text-admin-title-color">
          {{ emptyChannelMessage }}
        </div>
        <div class="mt-2 text-admin-meta text-admin-subtitle-color">
          {{ hasChannelSearch ? '调整搜索关键词后重试。' : '点击右上角新增渠道商开始配置。' }}
        </div>
      </div>

      <div v-else :class="adminMasonryGridClass">
        <Card
          v-for="card in filteredChannelCards"
          :key="card.channel.id"
          :class="adminMasonryCardClass"
          :pt="adminCardPt"
        >
          <template #title>
            <div class="flex items-start gap-3">
              <div class="flex min-w-0 items-center gap-3">
                <Avatar
                  :label="(card.channel.name || 'CH').substring(0, 2).toUpperCase()"
                  shape="circle"
                  class="!h-10 !w-10 !shrink-0 !bg-admin-accent !text-sm !font-bold !text-admin-text-inverse"
                />
                <div class="min-w-0">
                  <div class="truncate text-admin-title-sm font-bold text-admin-title-color">
                    {{ card.channel.name }}
                  </div>
                  <div class="truncate text-admin-meta font-medium text-admin-subtitle-color">
                    {{ card.channel.api_url }}
                  </div>
                </div>
              </div>
            </div>
          </template>

          <template #content>
            <div class="flex flex-col gap-3.5">
              <div class="flex flex-wrap items-center gap-1.5">
                <Tag
                  :value="getApiTypeLabel(card.channel.type)"
                  severity="info"
                  :class="adminCompactTagClass"
                  rounded
                />
                <Tag
                  :value="`${card.accounts.length} APIKey`"
                  severity="secondary"
                  :class="adminCompactTagClass"
                  rounded
                />
                <Tag
                  :value="`${card.modelCount} 模型`"
                  severity="secondary"
                  :class="adminCompactTagClass"
                  rounded
                />
              </div>

              <div class="flex flex-col gap-3">
                <div class="flex items-center justify-between">
                  <span class="text-admin-caption font-bold text-admin-text-3">
                    关联 APIKey ({{ card.accounts.length }})
                  </span>
                  <Button
                    text
                    rounded
                    size="small"
                    aria-label="添加 APIKey"
                    @click="openAddAccountModal(card.channel)"
                  >
                    <template #icon>
                      <Plus />
                    </template>
                  </Button>
                </div>

                <div v-if="card.accounts.length > 0" class="flex flex-col gap-2">
                  <Panel
                    v-for="account in card.accounts"
                    :key="account.id"
                    :pt="adminCompactPanelPt"
                  >
                    <template #header>
                      <div class="flex min-w-0 flex-1 items-center justify-between gap-2">
                        <div class="flex min-w-0 items-center gap-2">
                          <div
                            class="h-1.5 w-1.5 shrink-0 rounded-full"
                            :class="getAccountStatusDotClass(account)"
                          ></div>
                          <span
                            class="truncate text-admin-meta font-semibold text-admin-title-color"
                          >
                            {{ account.name || 'APIKey' }}
                          </span>
                          <span
                            class="shrink-0 font-mono text-admin-caption font-bold text-admin-text-3"
                          >
                            {{ account.current_concurrent || 0 }}/{{ account.max_concurrent }}
                          </span>
                        </div>
                        <div class="flex shrink-0 items-center gap-1">
                          <Button
                            size="small"
                            text
                            :class="adminFramedIconButtonClass"
                            aria-label="编辑 APIKey"
                            title="编辑 APIKey"
                            @click="openEditAccountModal(account)"
                          >
                            <template #icon>
                              <EditPencil class="size-4 text-admin-text-2" />
                            </template>
                          </Button>
                          <Button
                            size="small"
                            text
                            :class="adminFramedIconButtonClass"
                            aria-label="删除 APIKey"
                            title="删除 APIKey"
                            @click="handleDeleteAccount(account)"
                          >
                            <template #icon>
                              <Trash class="size-4 text-admin-text-2" />
                            </template>
                          </Button>
                        </div>
                      </div>
                    </template>

                    <div class="flex flex-col gap-2">
                      <ProgressBar
                        :value="
                          ((account.current_concurrent || 0) / (account.max_concurrent || 1)) * 100
                        "
                        :show-value="false"
                        class="!h-[3px] !rounded-full !bg-admin-border-soft"
                        :pt="getAccountProgressPt(account)"
                      />

                      <div class="flex flex-col gap-1.5">
                        <div
                          v-for="model in getAccountModelRows(account)"
                          :key="`${account.id}:${model.name}`"
                          class="grid min-h-9 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 rounded-lg border border-admin-border-soft bg-white px-2 py-1.5"
                        >
                          <Chip
                            :label="model.name"
                            :pt="modelChipPt"
                            :class="[
                              'min-w-0 !max-w-full !justify-start !overflow-hidden !rounded-none !bg-transparent !px-0 !py-0 !text-admin-meta !font-medium !text-admin-text-1',
                              model.disabled ? '!opacity-55' : '',
                            ]"
                          />
                          <ToggleSwitch
                            :model-value="!model.disabled"
                            size="small"
                            class="!shrink-0"
                            @update:model-value="toggleAccountModelStatus(account, model.name)"
                          />
                        </div>
                      </div>
                    </div>
                  </Panel>
                </div>
                <Message v-else severity="secondary" :closable="false">
                  点击上方按钮添加 APIKey
                </Message>
              </div>
              <div class="flex flex-wrap gap-2">
                <Button
                  size="small"
                  label="编辑"
                  severity="contrast"
                  @click="openEditChannelModal(card.channel)"
                />
                <Button
                  size="small"
                  label="删除"
                  severity="contrast"
                  outlined
                  @click="handleDeleteChannel(card.channel)"
                />
              </div>
            </div>
          </template>
        </Card>
      </div>
    </AdminManagementFrame>

    <!-- 渠道商表单弹窗 -->
    <Dialog
      v-model:visible="showChannelModal"
      modal
      :header="isEditChannel ? '编辑渠道商' : '新增渠道商'"
      :style="{ width: '480px' }"
    >
      <div :class="adminDialogFormBodyClass">
        <label
          class="flex min-w-0 flex-col gap-[7px] text-admin-meta font-semibold text-admin-text-2"
        >
          <span>渠道商名称</span>
          <InputText
            v-model="channelForm.name"
            class="w-full"
            placeholder="例如：Nano Banana 官方"
          />
        </label>
        <label
          class="flex min-w-0 flex-col gap-[7px] text-admin-meta font-semibold text-admin-text-2"
        >
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
        <label
          class="flex min-w-0 flex-col gap-[7px] text-admin-meta font-semibold text-admin-text-2"
        >
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
        <div :class="adminDialogCompactFooterActionsClass">
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
      <div :class="adminDialogFormBodyClass">
        <div
          v-if="selectedChannelForAccount"
          class="mb-4 flex items-center gap-1.5 rounded-xl border border-admin-border bg-admin-surface-muted px-3 py-2 text-admin-meta text-admin-text-2"
        >
          <span>所属渠道：</span>
          <code class="font-semibold text-admin-text-1">{{ selectedChannelForAccount.name }}</code>
        </div>

        <label
          class="flex min-w-0 flex-col gap-[7px] text-admin-meta font-semibold text-admin-text-2"
        >
          <span>{{ isEditAccount ? '修改 API 密钥 (留空不修改)' : 'API 密钥 (Key)' }}</span>
          <InputText
            v-model="accountForm.secret"
            class="w-full"
            type="password"
            :placeholder="isEditAccount ? '留空则保持原密钥' : '输入此渠道商的 API Key'"
          />
        </label>

        <label
          class="flex min-w-0 flex-col gap-[7px] text-admin-meta font-semibold text-admin-text-2"
        >
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
          <label
            class="flex min-w-0 flex-col gap-[7px] text-admin-meta font-semibold text-admin-text-2"
          >
            <span>最大并发数</span>
            <InputNumber
              v-model="accountForm.max_concurrent"
              :min="accountLimits.max_concurrent.min"
              :max="accountLimits.max_concurrent.max"
              fluid
            />
          </label>
          <label
            class="flex min-w-0 flex-col gap-[7px] text-admin-meta font-semibold text-admin-text-2"
          >
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
        <div :class="adminDialogCompactFooterActionsClass">
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
import Chip from 'primevue/chip'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Panel from 'primevue/panel'
import ProgressBar from 'primevue/progressbar'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { EditPencil, Plus, Refresh, Trash } from '@iconoir/vue'
import { useAppToast } from '../../../../shared/composables/useAppToast'
import AdminManagementFrame from '../../shared/components/AdminManagementFrame.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { useTransientScrolling } from '../../shared/composables/useTransientScrolling'
import { adminCardPt } from '../../shared/lib/adminCardPt'
import { adminCompactPanelPt } from '../../shared/lib/adminPanelPt'
import {
  adminCompactTagClass,
  adminDialogCompactFooterActionsClass,
  adminDialogFormBodyClass,
  adminEmptyStateClass,
  adminFramedIconButtonClass,
  adminLoadingStateClass,
  adminMasonryCardClass,
  adminMasonryGridClass,
  adminToolbarIconButtonClass,
  adminToolbarMetricClass,
} from '../../shared/lib/adminUiClasses'
import { createMockAdminChannels, useAdminMockMode } from '../../shared/lib/adminMockData'
import {
  ADMIN_CHANNEL_API_TYPE_OPTIONS,
  DEFAULT_ADMIN_CHANNEL_API_TYPE,
  GOOGLE_ADMIN_CHANNEL_API_TYPE,
  getAdminChannelApiTypeLabel,
} from './channelApiTypes'
import { extractApiErrorMessage } from './channelModelUtils'
import { useChannelAccounts } from './useChannelAccounts'

const appToast = useAppToast()
const confirm = useConfirm()
const isMockMode = useAdminMockMode()

// State
const loading = ref(false)
const channels = ref<Channel[]>([])
const searchKeyword = ref('')
const { isScrolling: isChannelPanelScrolling, handleScroll: handleChannelPanelScroll } =
  useTransientScrolling()

const filteredChannels = computed(() => {
  const keyword = searchKeyword.value.trim().toLowerCase()
  if (!keyword) return channels.value
  return channels.value.filter((item) => (item.name || '').toLowerCase().includes(keyword))
})
const filteredChannelCards = computed(() =>
  filteredChannels.value.map((channel) => {
    const channelAccounts = getChannelAccounts(channel.id)
    const models = new Set<string>()
    channelAccounts.forEach((account) => {
      getAccountModelRows(account).forEach((model) => models.add(model.name))
    })

    return {
      channel,
      accounts: channelAccounts,
      modelCount: models.size,
    }
  }),
)
const hasChannelSearch = computed(() => Boolean(searchKeyword.value.trim()))
const totalChannelsLabel = computed(() =>
  hasChannelSearch.value
    ? `${filteredChannels.value.length} 匹配渠道`
    : `${channels.value.length} 渠道`,
)
const emptyChannelMessage = computed(() => (hasChannelSearch.value ? '无匹配渠道商' : '暂无渠道商'))

// Data Loading
async function loadData() {
  loading.value = true
  try {
    if (isMockMode.value) {
      channels.value = createMockAdminChannels()
      await loadAccounts()
      return
    }

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
// --- Channel Form ---
const showChannelModal = ref(false)
const isEditChannel = ref(false)
const channelSaveLoading = ref(false)
const currentChannelId = ref(0)
const createEmptyChannelForm = (): ChannelFormData => ({
  type: DEFAULT_ADMIN_CHANNEL_API_TYPE,
  name: '',
  api_url: '',
})
const channelForm = ref<ChannelFormData>(createEmptyChannelForm())
const apiTypeOptions = ADMIN_CHANNEL_API_TYPE_OPTIONS
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
const modelChipPt = {
  root: { class: '!inline-flex !min-w-0' },
  label: { class: 'min-w-0 truncate' },
} as const
const activeProgressPt = { value: { class: '!bg-admin-success' } } as const
const mutedProgressPt = { value: { class: '!bg-admin-text-3' } } as const

function getApiTypeLabel(value: string) {
  return getAdminChannelApiTypeLabel(value)
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
    channelForm.value.type = GOOGLE_ADMIN_CHANNEL_API_TYPE
    return
  }
  if (apiUrl.includes('api.openai.com')) {
    channelForm.value.type = DEFAULT_ADMIN_CHANNEL_API_TYPE
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

    if (isMockMode.value) {
      const now = new Date().toISOString()
      if (isEditChannel.value) {
        channels.value = channels.value.map((channel) =>
          channel.id === currentChannelId.value
            ? {
                ...channel,
                ...submitData,
                updated_at: now,
              }
            : channel,
        )
        appToast.success('渠道商已更新')
      } else {
        channels.value = [
          {
            id: Math.max(200, ...channels.value.map((channel) => channel.id)) + 1,
            ...submitData,
            status: 1,
            created_at: now,
            updated_at: now,
          },
          ...channels.value,
        ]
        appToast.success('渠道商已创建')
      }
      showChannelModal.value = false
      return
    }

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
        if (isMockMode.value) {
          channels.value = channels.value.filter((channel) => channel.id !== row.id)
          appToast.success('删除成功')
          return
        }

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
  accounts,
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
  getAccountModelRows,
  openAddAccountModal,
  openEditAccountModal,
  handleDetectModelsOnSelectOpen,
  searchModelSuggestions,
  handleSaveAccount,
  toggleAccountModelStatus,
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
const totalAccountCount = computed(() => accounts.value.length)
const runningAccountCount = computed(
  () => accounts.value.filter((account) => account.status === 1 && account.fail_count < 5).length,
)
const accountRunningLabel = computed(() =>
  totalAccountCount.value > 0
    ? `${runningAccountCount.value}/${totalAccountCount.value} 运行中`
    : '0 APIKey',
)
const activeModelCount = computed(() => {
  const models = new Set<string>()
  accounts.value.forEach((account) => {
    account.models.forEach((model) => models.add(model.name))
  })
  return models.size
})

function handleAccountModelSelectShow(show: boolean) {
  if (!show) return
  void handleDetectModelsOnSelectOpen()
}

onMounted(() => {
  loadData()
})
</script>
