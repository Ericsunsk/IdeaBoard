<template>
  <div class="admin-page channels-page">
    <div class="toolbar">
      <div class="toolbar-left">
        <n-input
          v-model:value="searchKeyword"
          placeholder="搜索渠道商名称..."
          clearable
          class="admin-search-input"
        >
          <template #prefix
            ><n-icon><Search /></n-icon
          ></template>
        </n-input>
        <n-button secondary class="admin-icon-btn" @click="loadData">
          <template #icon
            ><n-icon><Refresh /></n-icon
          ></template>
        </n-button>
      </div>
      <div class="toolbar-spacer" />
      <div class="toolbar-right">
        <n-button type="primary" @click="openAddChannelModal">
          <template #icon
            ><n-icon><Plus /></n-icon
          ></template>
          新增渠道商
        </n-button>
      </div>
    </div>

    <div v-if="loading && channels.length === 0" class="loading-state">
      <n-spin size="large" />
    </div>

    <div v-else class="channel-grid">
      <article
        v-for="channel in filteredChannels"
        :key="channel.id"
        class="channel-card panel-card admin-card-interactive"
      >
        <AdminCardHeader :title="channel.name" :subtitle="channel.api_url" size="small">
          <template #prefix>
            <div class="provider-badge">
              {{ (channel.name || 'CH').substring(0, 2).toUpperCase() }}
            </div>
          </template>
          <template #extra>
            <AdminChip
              :label="getChannelStatus(channel)"
              :variant="getChannelStatusVariant(channel)"
            />
          </template>
        </AdminCardHeader>

        <div class="api-type-row">
          <span class="api-type-label">接口类型</span>
          <span class="api-type-pill">{{ getApiTypeLabel(channel.type) }}</span>
        </div>

        <div class="models-shelf">
          <div class="shelf-label">当前生效模型</div>
          <div v-if="getChannelActiveModels(channel.id).length > 0" class="shelf-content">
            <span
              v-for="model in getChannelActiveModels(channel.id)"
              :key="model"
              class="model-pill"
              :class="getModelCategoryClass(model)"
            >
              {{ model }}
            </span>
          </div>
          <div v-else class="empty-shelf">未配置生效模型</div>
        </div>

        <div class="accounts-grid">
          <div class="grid-header">
            <span class="grid-title"
              >关联 APIKey ({{ getChannelAccounts(channel.id).length }})</span
            >
            <n-button size="tiny" quaternary circle @click="openAddAccountModal(channel)">
              <template #icon
                ><n-icon><Plus /></n-icon
              ></template>
            </n-button>
          </div>

          <div v-if="getChannelAccounts(channel.id).length > 0" class="accounts-scroller">
            <div
              v-for="account in getChannelAccounts(channel.id)"
              :key="account.id"
              class="account-row"
            >
              <div class="row-main">
                <div class="row-left">
                  <div class="status-indicator" :class="getAccStatusClass(account)"></div>
                  <span class="acc-name">{{ account.model_set || '未分配' }}</span>
                </div>
                <div class="row-right">
                  <span class="acc-stat-val"
                    >{{ account.current_concurrent || 0 }}/{{ account.max_concurrent }}</span
                  >
                </div>
              </div>
              <n-progress
                type="line"
                :percentage="
                  ((account.current_concurrent || 0) / (account.max_concurrent || 1)) * 100
                "
                :show-indicator="false"
                :height="3"
                :color="account.status === 1 ? 'var(--admin-success)' : 'var(--admin-text-3)'"
                class="acc-progress"
              />
              <div class="row-actions">
                <n-button size="tiny" quaternary @click="openEditAccountModal(account)"
                  >编辑</n-button
                >
                <n-button
                  size="tiny"
                  quaternary
                  :type="account.status === 1 ? 'warning' : 'success'"
                  @click="toggleAccountStatus(account)"
                >
                  {{ account.status === 1 ? '停止' : '启动' }}
                </n-button>
                <n-button size="tiny" quaternary type="error" @click="handleDeleteAccount(account)"
                  >删除</n-button
                >
              </div>
            </div>
          </div>
          <div v-else class="empty-accounts">点击上方按钮添加 APIKey</div>
        </div>
        <div class="channel-card-footer">
          <n-button
            size="small"
            color="#ffffff"
            text-color="#000000"
            @click="openEditChannelModal(channel)"
            >配置详情</n-button
          >
          <n-button size="small" tertiary type="error" @click="handleDeleteChannel(channel)"
            >删除</n-button
          >
        </div>
      </article>

      <button class="admin-add-card" type="button" @click="openAddChannelModal">
        <span class="add-mark">+</span>
        <span class="add-title">添加渠道商</span>
      </button>

      <!-- Empty state -->
      <div v-if="filteredChannels.length === 0" class="empty-state">无匹配的渠道商</div>
    </div>

    <!-- 渠道商表单弹窗 -->
    <n-modal
      v-model:show="showChannelModal"
      preset="card"
      :title="isEditChannel ? '编辑渠道商' : '新增渠道商'"
      style="width: 480px"
    >
      <n-form ref="channelFormRef" :model="channelForm" :rules="channelRules" label-placement="top">
        <n-form-item label="渠道商名称" path="name">
          <n-input v-model:value="channelForm.name" placeholder="例如：Nano Banana 官方" />
        </n-form-item>
        <n-form-item label="接口类型" path="type">
          <n-select
            v-model:value="channelForm.type"
            :options="apiTypeOptions"
            placeholder="选择接口协议"
          />
        </n-form-item>
        <n-form-item label="Base URL地址" path="api_url">
          <n-input
            v-model:value="channelForm.api_url"
            placeholder="例如：https://api.example.com/v1"
            @blur="inferChannelApiTypeFromUrl"
          />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showChannelModal = false">取消</n-button>
          <n-button type="primary" :loading="channelSaveLoading" @click="handleSaveChannel"
            >确定</n-button
          >
        </n-space>
      </template>
    </n-modal>

    <!-- APIKey 配置弹窗 -->
    <n-modal
      v-model:show="showAccountModal"
      preset="card"
      :title="isEditAccount ? '编辑 APIKey' : '新增 APIKey'"
      style="width: 500px"
    >
      <n-form ref="accountFormRef" :model="accountForm" :rules="accountRules" label-placement="top">
        <div v-if="selectedChannelForAccount" class="channel-url-hint">
          <span>所属渠道：</span>
          <code>{{ selectedChannelForAccount.name }}</code>
        </div>

        <n-form-item
          :label="isEditAccount ? '修改 API 密钥 (留空不修改)' : 'API 密钥 (Key)'"
          path="api_key"
        >
          <n-input
            v-model:value="accountForm.api_key"
            type="password"
            show-password-on="click"
            :placeholder="isEditAccount ? '留空则保持原密钥' : '输入此渠道商的 API Key'"
          />
        </n-form-item>

        <n-form-item label="分配模型" path="model_values">
          <n-select
            v-model:value="accountForm.model_values"
            multiple
            filterable
            tag
            :loading="detectModelsLoading"
            :options="accountModelOptions"
            placeholder="选择或输入分配给该 APIKey 的模型"
            class="model-select-rounded"
            @update:show="handleAccountModelSelectShow"
          />
        </n-form-item>

        <n-grid :cols="2" :x-gap="16">
          <n-gi>
            <n-form-item label="最大并发数" path="max_concurrent">
              <n-input-number
                v-model:value="accountForm.max_concurrent"
                :min="1"
                :max="50"
                style="width: 100%"
              />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="超时时间(秒)" path="timeout">
              <n-input-number
                v-model:value="accountForm.timeout"
                :min="10"
                :max="600"
                style="width: 100%"
              />
            </n-form-item>
          </n-gi>
        </n-grid>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showAccountModal = false">取消</n-button>
          <n-button type="primary" :loading="accountSaveLoading" @click="handleSaveAccount"
            >确定</n-button
          >
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import channelApi from '../../../shared/api/channel'
import type { Channel, ChannelFormData } from '../../../shared/api/channel'
import { NButton, NIcon } from 'naive-ui'
import { Plus, Refresh, Search } from '@iconoir/vue'
import { extractApiErrorMessage, getModelCategoryClass } from '../shared/assetLibraryShared'
import { useChannelAccounts } from './useChannelAccounts'

const message = useMessage()
const dialog = useDialog()

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
    message.error(extractApiErrorMessage(error, '加载数据失败'))
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

function getChannelStatusVariant(channel: Channel) {
  const accs = getChannelAccounts(channel.id)
  if (accs.length === 0) return 'empty'
  const active = accs.filter((a) => a.status === 1 && a.fail_count < 5).length
  if (active === 0) return 'neutral'
  if (active < accs.length) return 'current'
  return 'enabled'
}

// --- Channel Form ---
const showChannelModal = ref(false)
const isEditChannel = ref(false)
const channelFormRef = ref()
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

const channelRules = {
  name: [{ required: true, message: '请输入渠道商名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择接口类型', trigger: 'change' }],
  api_url: [{ required: true, message: '请输入 Base URL地址', trigger: 'blur' }],
}

function getApiTypeLabel(value: string) {
  return apiTypeLabelByValue.get(value) || value || '未配置'
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
    await channelFormRef.value?.validate()
    channelSaveLoading.value = true
    const submitData = { ...channelForm.value }

    if (isEditChannel.value) {
      await channelApi.update(currentChannelId.value, submitData)
      message.success('渠道商已更新')
    } else {
      await channelApi.create(submitData)
      message.success('渠道商已创建')
    }
    showChannelModal.value = false
    loadData()
  } catch (error) {
    message.error(extractApiErrorMessage(error, '保存渠道商失败'))
  } finally {
    channelSaveLoading.value = false
  }
}

function handleDeleteChannel(row: Channel) {
  dialog.warning({
    title: '确认删除',
    content: `确定删除渠道商「${row.name}」吗？此操作不可恢复。`,
    positiveText: '确定',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await channelApi.delete(row.id)
        message.success('删除成功')
        await loadData()
      } catch (error) {
        message.error(extractApiErrorMessage(error, '删除渠道商失败'))
      }
    },
  })
}

const {
  showAccountModal,
  isEditAccount,
  accountFormRef,
  accountSaveLoading,
  selectedChannelForAccount,
  accountForm,
  detectModelsLoading,
  accountModelOptions,
  accountRules,
  loadAccounts,
  getChannelAccounts,
  getChannelActiveModels,
  getAccStatusClass,
  openAddAccountModal,
  openEditAccountModal,
  handleDetectModelsOnSelectOpen,
  handleSaveAccount,
  toggleAccountStatus,
  handleDeleteAccount,
} = useChannelAccounts({
  channels,
  message,
  dialog,
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

<style scoped>
@import '../shared/adminCommon.css';

.channels-page {
  gap: 16px;
  padding-bottom: 24px;
}

.channel-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.loading-state,
.empty-state {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 60px 0;
  color: var(--admin-text-3);
  font-size: 14px;
}

.channel-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
  position: relative;
  overflow: hidden;
  min-height: 260px;
}

.channel-card:hover {
  border-color: #2e3034;
  background: #141416;
}

/* Card header styles now handled by AdminCardHeader */
.provider-badge {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: #17181a;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 800;
  color: var(--admin-text-1);
  border: 1px solid var(--admin-border);
  flex-shrink: 0;
}

.api-type-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  min-height: 30px;
}

.api-type-label {
  color: var(--admin-text-3);
  font-size: 11px;
  font-weight: 700;
}

.api-type-pill {
  display: inline-flex;
  align-items: center;
  height: 24px;
  border: 1px solid rgba(129, 140, 248, 0.35);
  border-radius: 999px;
  padding: 0 10px;
  color: #c4b5fd;
  background: rgba(129, 140, 248, 0.08);
  font-size: 11px;
  font-weight: 700;
}

.models-shelf {
  background: #17181a;
  border-radius: 12px;
  padding: 10px;
  border: 1px solid var(--admin-border);
}

.shelf-label {
  font-size: 11px;
  font-weight: 700;
  color: var(--admin-text-3);
  margin-bottom: 8px;
}

.shelf-content {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.model-pill {
  display: inline-flex;
  align-items: center;
  height: 24px;
  padding: 0 10px;
  border-radius: 999px;
  background: #1c1d20;
  border: 1px solid var(--admin-border);
  color: var(--admin-text-2);
  font-size: 11px;
}

.model-pill.is-indigo {
  border-color: var(--admin-model-indigo);
  color: var(--admin-model-indigo);
  background: rgba(129, 140, 248, 0.05);
}
.model-pill.is-purple {
  border-color: var(--admin-model-purple);
  color: var(--admin-model-purple);
  background: rgba(192, 132, 252, 0.05);
}
.model-pill.is-cyan {
  border-color: var(--admin-model-cyan);
  color: var(--admin-model-cyan);
  background: rgba(34, 211, 238, 0.05);
}

.accounts-grid {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.grid-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.grid-title {
  font-size: 12px;
  font-weight: 700;
  color: var(--admin-text-1);
}

.accounts-scroller {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 200px;
  overflow-y: auto;
  padding-right: 4px;
}

.account-row {
  background: rgba(255, 255, 255, 0.02);
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.03);
  padding: 8px 10px;
}

.row-main {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}

.row-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.status-indicator {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.status-indicator.acc-success {
  background: var(--admin-success);
}
.status-indicator.acc-warning {
  background: var(--admin-warning);
}
.status-indicator.acc-error {
  background: var(--admin-danger);
}

.acc-name {
  font-size: 12px;
  font-weight: 600;
  color: var(--admin-text-1);
}

.acc-stat-val {
  font-size: 11px;
  font-weight: 700;
  color: var(--admin-text-3);
  font-family: ui-monospace, monospace;
}

.acc-progress {
  margin-bottom: 8px;
}

.row-actions {
  display: flex;
  gap: 4px;
  justify-content: flex-end;
}

.channel-card-footer {
  margin-top: auto;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding-top: 12px;
  border-top: 1px solid var(--admin-border);
}

.channel-card-footer :deep(.n-button) {
  flex: none;
}

.model-select-rounded :deep(.n-base-selection-tag-wrapper) {
  gap: 4px;
}

.model-select-rounded :deep(.n-tag) {
  border-radius: 999px !important;
  padding: 0 12px;
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.model-select-rounded :deep(.n-tag__close) {
  margin-left: 4px;
}

/* Add card styles now global */

.accounts-scroller::-webkit-scrollbar {
  width: 4px;
}

.accounts-scroller::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.1);
  border-radius: 10px;
}

.channel-url-hint {
  background: rgba(255, 255, 255, 0.03);
  border-radius: 12px;
  border: 1px solid #2b2c2f;
  padding: 8px 12px;
  margin-bottom: 16px;
  font-size: 12px;
  color: var(--admin-text-2);
  display: flex;
  align-items: center;
  gap: 6px;
}

.channel-url-hint code {
  color: var(--admin-text-1);
  font-weight: 600;
}

@media (max-width: 980px) {
  .channel-grid {
    grid-template-columns: 1fr;
  }
}
</style>
