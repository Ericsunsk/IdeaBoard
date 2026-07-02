<template>
  <div>
    <AdminResourceGridFrame
      :empty-description="
        hasChannelSearch ? '调整搜索关键词后重试。' : '点击右上角新增渠道商开始配置。'
      "
      :empty-title="emptyChannelMessage"
      :is-empty="channelCards.length === 0"
      :is-loading="isChannelLoading"
      :is-scrolling="isChannelPanelScrolling"
      @scroll="handleChannelPanelScroll"
    >
      <template #toolbar>
        <AdminResourceToolbar>
          <template #search>
            <AdminToolbarSearch v-model="searchKeyword" />
          </template>

          <template #metrics="{ metricClass }">
            <span :class="metricClass">
              <span class="h-2.5 w-2.5 shrink-0 rounded-full bg-admin-success"></span>
              <span>{{ totalChannelsLabel }}</span>
            </span>
            <Tag :value="accountRunningLabel" severity="success" rounded />
            <Tag :value="`${activeModelCount} 模型`" severity="info" rounded />
          </template>

          <template #actions="{ iconButtonClass }">
            <Button
              text
              :class="iconButtonClass"
              aria-label="刷新渠道商"
              @click="commands.loadData"
            >
              <template #icon>
                <Refresh class="size-4 text-admin-text-2" />
              </template>
            </Button>
            <Button label="新增渠道商" class="!h-9 !rounded-lg" @click="channelEditor.openCreate">
              <template #icon>
                <Plus class="size-4" />
              </template>
            </Button>
          </template>
        </AdminResourceToolbar>
      </template>

      <Card
        v-for="card in channelCards"
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
                  {{ card.channel.baseUrl }}
                </div>
              </div>
            </div>
          </div>
        </template>

        <template #content>
          <div class="flex flex-col gap-3.5">
            <div class="flex flex-wrap items-center gap-1.5">
              <Tag
                :value="card.channel.apiTypeLabel"
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
                  @click="accountEditor.openCreate(card.channel.commandTarget)"
                >
                  <template #icon>
                    <Plus />
                  </template>
                </Button>
              </div>

              <div v-if="card.accounts.length > 0" class="flex flex-col gap-2">
                <Panel v-for="account in card.accounts" :key="account.id" :pt="adminCompactPanelPt">
                  <template #header>
                    <div class="flex min-w-0 flex-1 items-center justify-between gap-2">
                      <div class="flex min-w-0 items-center gap-2">
                        <div
                          class="h-1.5 w-1.5 shrink-0 rounded-full"
                          :class="commands.getAccountStatusDotClass(account)"
                        ></div>
                        <span class="truncate text-admin-meta font-semibold text-admin-title-color">
                          {{ account.name || 'APIKey' }}
                        </span>
                        <span
                          class="shrink-0 font-mono text-admin-caption font-bold text-admin-text-3"
                        >
                          {{ account.concurrencyLabel }}
                        </span>
                      </div>
                      <div class="flex shrink-0 items-center gap-1">
                        <Button
                          size="small"
                          text
                          :class="adminFramedIconButtonClass"
                          aria-label="编辑 APIKey"
                          title="编辑 APIKey"
                          @click="accountEditor.openEdit(account.commandTarget)"
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
                          @click="commands.deleteAccount(account.commandTarget)"
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
                      :value="account.concurrencyPercent"
                      :show-value="false"
                      class="!h-[3px] !rounded-full !bg-admin-border-soft"
                      :pt="commands.getAccountProgressPt(account)"
                    />

                    <div class="flex flex-col gap-1.5">
                      <div
                        v-for="model in commands.getAccountModelRows(account)"
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
                          :aria-label="`${model.name} 模型启用状态`"
                          @update:model-value="
                            commands.toggleAccountModelStatus(account.commandTarget, model.name)
                          "
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
                @click="channelEditor.openEdit(card.channel.commandTarget)"
              />
              <Button
                size="small"
                label="删除"
                severity="contrast"
                outlined
                @click="commands.deleteChannel(card.channel.commandTarget)"
              />
            </div>
          </div>
        </template>
      </Card>
    </AdminResourceGridFrame>

    <!-- 渠道商表单弹窗 -->
    <Dialog
      v-model:visible="showChannelModal"
      modal
      :header="isEditChannel ? '编辑渠道商' : '新增渠道商'"
      :style="{ width: 'min(480px, calc(100vw - 24px))' }"
    >
      <div :class="adminDialogFormBodyClass">
        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          label="渠道商名称"
          for="admin-channel-name"
          :error="channelFormErrors.name"
        >
          <InputText
            id="admin-channel-name"
            v-model="channelForm.name"
            class="w-full"
            placeholder="例如：Nano Banana 官方"
            :invalid="invalid"
            :aria-describedby="describedbyId"
            @update:model-value="clearChannelFormError('name')"
          />
        </AdminFormField>
        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          label="接口类型"
          for="admin-channel-type"
          :error="channelFormErrors.apiType"
        >
          <Select
            v-model="channelForm.apiType"
            input-id="admin-channel-type"
            class="w-full"
            :options="channelEditor.apiTypeOptions"
            option-label="label"
            option-value="value"
            placeholder="选择接口协议"
            :invalid="invalid"
            :aria-describedby="describedbyId"
            @update:model-value="clearChannelFormError('apiType')"
          />
        </AdminFormField>
        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          label="Base URL地址"
          for="admin-channel-api-url"
          :error="channelFormErrors.baseUrl"
        >
          <InputText
            id="admin-channel-api-url"
            v-model="channelForm.baseUrl"
            class="w-full"
            placeholder="例如：https://api.example.com/v1"
            :invalid="invalid"
            :aria-describedby="describedbyId"
            @blur="channelEditor.handleApiUrlBlur"
            @update:model-value="clearChannelFormError('baseUrl')"
          />
        </AdminFormField>
      </div>
      <template #footer>
        <div :class="adminDialogCompactFooterActionsClass">
          <Button label="取消" severity="secondary" outlined @click="closeChannelModal" />
          <Button label="确定" :loading="channelSaveLoading" @click="handleSaveChannel" />
        </div>
      </template>
    </Dialog>

    <!-- APIKey 配置弹窗 -->
    <Dialog
      v-model:visible="showAccountModal"
      modal
      :header="isEditAccount ? '编辑 APIKey' : '新增 APIKey'"
      :style="{ width: 'min(500px, calc(100vw - 24px))' }"
    >
      <div :class="adminDialogFormBodyClass">
        <div
          v-if="selectedChannelForAccount"
          class="mb-4 flex items-center gap-1.5 rounded-xl border border-admin-border bg-admin-surface-muted px-3 py-2 text-admin-meta text-admin-text-2"
        >
          <span>所属渠道：</span>
          <code class="font-semibold text-admin-text-1">{{ selectedChannelForAccount.name }}</code>
        </div>

        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          :label="isEditAccount ? '修改 API 密钥 (留空不修改)' : 'API 密钥 (Key)'"
          for="admin-account-secret"
          :error="accountFormErrors.secret"
        >
          <InputText
            id="admin-account-secret"
            v-model="accountForm.secret"
            class="w-full"
            type="password"
            :placeholder="isEditAccount ? '留空则保持原密钥' : '输入此渠道商的 API Key'"
            :invalid="invalid"
            :aria-describedby="describedbyId"
            @update:model-value="clearAccountFormError('secret')"
          />
        </AdminFormField>

        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          label="分配模型"
          for="admin-account-models"
          :error="accountFormErrors.modelValues"
        >
          <AutoComplete
            v-model="accountForm.modelValues"
            input-id="admin-account-models"
            multiple
            fluid
            :suggestions="modelSuggestions"
            :loading="detectModelsLoading"
            placeholder="选择或输入分配给该 APIKey 的模型"
            :pt="modelSelectPt"
            :invalid="invalid"
            :aria-describedby="describedbyId"
            @focus="accountEditor.handleModelSelectShow(true)"
            @complete="searchModelSuggestions"
            @update:model-value="clearAccountFormError('modelValues')"
          />
        </AdminFormField>

        <div class="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
          <AdminFormField
            v-slot="{ describedbyId, invalid }"
            label="最大并发数"
            for="admin-account-max-concurrent"
            :error="accountFormErrors.maxConcurrent"
          >
            <InputNumber
              v-model="accountForm.maxConcurrent"
              input-id="admin-account-max-concurrent"
              :min="accountLimits.maxConcurrent.min"
              :max="accountLimits.maxConcurrent.max"
              fluid
              :invalid="invalid"
              :aria-describedby="describedbyId"
              @update:model-value="clearAccountFormError('maxConcurrent')"
            />
          </AdminFormField>
          <AdminFormField
            v-slot="{ describedbyId, invalid }"
            label="超时时间(秒)"
            for="admin-account-timeout"
            :error="accountFormErrors.timeoutSeconds"
          >
            <InputNumber
              v-model="accountForm.timeoutSeconds"
              input-id="admin-account-timeout"
              :min="accountLimits.timeoutSeconds.min"
              :max="accountLimits.timeoutSeconds.max"
              fluid
              :invalid="invalid"
              :aria-describedby="describedbyId"
              @update:model-value="clearAccountFormError('timeoutSeconds')"
            />
          </AdminFormField>
        </div>
      </div>
      <template #footer>
        <div :class="adminDialogCompactFooterActionsClass">
          <Button label="取消" severity="secondary" outlined @click="closeAccountModal" />
          <Button label="确定" :loading="accountSaveLoading" @click="accountEditor.save" />
        </div>
      </template>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
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
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { EditPencil, Plus, Refresh, Trash } from '@iconoir/vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminResourceGridFrame from '../../shared/components/AdminResourceGridFrame.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { useTransientScrolling } from '../../shared/composables/useTransientScrolling'
import { adminCardPt } from '../../shared/lib/adminCardPt'
import { adminCompactPanelPt } from '../../shared/lib/adminPanelPt'
import {
  adminCompactTagClass,
  adminDialogCompactFooterActionsClass,
  adminDialogFormBodyClass,
  adminFramedIconButtonClass,
  adminMasonryCardClass,
} from '../../shared/lib/adminUiClasses'
import { useAdminProviderConfig } from '../../provider-config/useAdminProviderConfig'
import { useAdminFeedback } from '../../feedback/useAdminFeedback'

const adminFeedback = useAdminFeedback()
const confirm = useConfirm()

const { isScrolling: isChannelPanelScrolling, handleScroll: handleChannelPanelScroll } =
  useTransientScrolling()

const { cards, channelEditor, accountEditor, commands } = useAdminProviderConfig({
  feedback: adminFeedback.channels.providerConfig,
  confirm: {
    danger: ({ title, content, acceptLabel, onConfirm }) =>
      requireAdminConfirm({
        header: title,
        message: content,
        acceptLabel,
        accept: onConfirm,
      }),
  },
})

const searchKeyword = cards.searchKeyword
const channelCards = cards.items
const isChannelLoading = cards.isLoading
const hasChannelSearch = cards.hasSearch
const totalChannelsLabel = cards.totalChannelsLabel
const emptyChannelMessage = cards.emptyMessage
const accountRunningLabel = cards.accountRunningLabel
const activeModelCount = cards.activeModelCount

const showChannelModal = channelEditor.visible
const isEditChannel = channelEditor.isEditing
const channelSaveLoading = channelEditor.saveLoading
const channelForm = channelEditor.form
const channelFormErrors = channelEditor.errors

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

function closeChannelModal() {
  channelEditor.close()
}

async function handleSaveChannel() {
  await channelEditor.save()
}

function clearChannelFormError(field: keyof typeof channelForm.value) {
  channelEditor.clearError(field)
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

const showAccountModal = accountEditor.visible
const isEditAccount = accountEditor.isEditing
const accountSaveLoading = accountEditor.saveLoading
const accountLimits = accountEditor.accountLimits
const selectedChannelForAccount = accountEditor.selectedChannel
const accountForm = accountEditor.form
const accountFormErrors = accountEditor.errors
const detectModelsLoading = accountEditor.detectModelsLoading
const modelSuggestions = accountEditor.modelSuggestions
const searchModelSuggestions = accountEditor.searchModelSuggestions

function closeAccountModal() {
  accountEditor.close()
}

function clearAccountFormError(field: Parameters<typeof accountEditor.clearError>[0]) {
  accountEditor.clearError(field)
}

onMounted(() => {
  commands.loadData()
})
</script>
