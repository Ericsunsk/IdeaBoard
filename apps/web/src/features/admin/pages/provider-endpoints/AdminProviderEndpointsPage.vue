<template>
  <div>
    <AdminResourceGridFrame
      :empty-description="
        hasProviderEndpointSearch ? '调整搜索关键词后重试。' : '点击右上角新增供应商端点开始配置。'
      "
      :empty-title="emptyProviderEndpointMessage"
      :is-empty="providerEndpointCards.length === 0"
      :is-loading="isProviderEndpointLoading"
    >
      <template #toolbar>
        <AdminResourceToolbar>
          <template #search>
            <AdminToolbarSearch v-model="searchKeyword" />
          </template>

          <template #metrics="{ metricClass }">
            <span :class="metricClass">
              <span class="h-2.5 w-2.5 shrink-0 rounded-full bg-admin-success"></span>
              <span>{{ totalProviderEndpointsLabel }}</span>
            </span>
            <Tag :value="accountRunningLabel" severity="success" rounded />
            <Tag :value="`${activeModelCount} 模型`" severity="info" rounded />
          </template>

          <template #actions="{ iconButtonClass }">
            <Button
              text
              :class="iconButtonClass"
              aria-label="刷新供应商端点"
              @click="commands.loadData"
            >
              <template #icon>
                <Refresh class="size-4 text-admin-text-2" />
              </template>
            </Button>
            <Button
              label="新增供应商端点"
              :class="adminActionButtonClass"
              @click="providerEndpointEditor.openCreate"
            >
              <template #icon>
                <Plus class="size-4" />
              </template>
            </Button>
          </template>
        </AdminResourceToolbar>
      </template>

      <Card
        v-for="card in providerEndpointCards"
        :key="card.providerEndpoint.id"
        :class="adminMasonryCardClass"
        :pt="adminCardPt"
      >
        <template #title>
          <div class="flex items-start gap-3">
            <div class="flex min-w-0 items-center gap-3">
              <Avatar
                :label="(card.providerEndpoint.name || 'EP').substring(0, 2).toUpperCase()"
                shape="circle"
                class="!h-10 !w-10 !shrink-0 !bg-admin-accent !text-sm !font-bold !text-admin-text-inverse"
              />
              <div class="min-w-0">
                <div class="truncate text-admin-title-sm font-bold text-admin-title-color">
                  {{ card.providerEndpoint.name }}
                </div>
                <div class="truncate text-admin-meta font-medium text-admin-subtitle-color">
                  {{ card.providerEndpoint.baseUrl }}
                </div>
              </div>
            </div>
          </div>
        </template>

        <template #content>
          <div class="flex flex-col gap-3.5">
            <div class="flex flex-wrap items-center gap-1.5">
              <Tag
                :value="card.providerEndpoint.apiTypeLabel"
                severity="info"
                :class="adminCompactTagClass"
                rounded
              />
              <Tag
                :value="card.providerEndpoint.status === 1 ? '启用' : '停用'"
                :severity="card.providerEndpoint.status === 1 ? 'success' : 'warn'"
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
                  :class="adminTextButtonClass"
                  aria-label="添加 APIKey"
                  @click="accountEditor.openCreate(card.providerEndpoint.commandTarget)"
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
                @click="providerEndpointEditor.openEdit(card.providerEndpoint.commandTarget)"
              />
              <Button
                size="small"
                label="删除"
                severity="contrast"
                outlined
                @click="commands.deleteProviderEndpoint(card.providerEndpoint.commandTarget)"
              />
            </div>
          </div>
        </template>
      </Card>
    </AdminResourceGridFrame>

    <!-- 供应商端点表单弹窗 -->
    <AdminDialogShell
      v-model:visible="showProviderEndpointModal"
      :header="isEditProviderEndpoint ? '编辑供应商端点' : '新增供应商端点'"
      :busy="providerEndpointSaveLoading"
    >
      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="端点名称"
        for="admin-provider-endpoint-name"
        :error="providerEndpointFormErrors.name"
      >
        <InputText
          id="admin-provider-endpoint-name"
          v-model="providerEndpointForm.name"
          class="w-full"
          placeholder="例如：Nano Banana 官方端点"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @update:model-value="clearProviderEndpointFormError('name')"
        />
      </AdminFormField>
      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="接口类型"
        for="admin-provider-endpoint-type"
        :error="providerEndpointFormErrors.apiType"
      >
        <Select
          v-model="providerEndpointForm.apiType"
          input-id="admin-provider-endpoint-type"
          class="w-full"
          :options="providerEndpointEditor.apiTypeOptions"
          option-label="label"
          option-value="value"
          placeholder="选择接口协议"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @update:model-value="clearProviderEndpointFormError('apiType')"
        />
      </AdminFormField>
      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="Base URL地址"
        for="admin-provider-endpoint-api-url"
        :error="providerEndpointFormErrors.baseUrl"
      >
        <InputText
          id="admin-provider-endpoint-api-url"
          v-model="providerEndpointForm.baseUrl"
          class="w-full"
          placeholder="例如：https://api.example.com/v1"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @blur="providerEndpointEditor.handleApiUrlBlur"
          @update:model-value="clearProviderEndpointFormError('baseUrl')"
        />
      </AdminFormField>
      <AdminFormField label="端点状态" for="admin-provider-endpoint-status">
        <div
          class="flex min-h-10 items-center justify-between rounded-lg border border-admin-border-soft bg-white px-3"
        >
          <span class="text-admin-body-sm font-medium text-admin-text-1">
            {{ providerEndpointForm.status === 1 ? '启用' : '停用' }}
          </span>
          <ToggleSwitch
            input-id="admin-provider-endpoint-status"
            :model-value="providerEndpointForm.status === 1"
            aria-label="供应商端点启用状态"
            @update:model-value="providerEndpointForm.status = $event ? 1 : 0"
          />
        </div>
      </AdminFormField>
      <template #footer>
        <Button
          label="取消"
          severity="secondary"
          outlined
          :disabled="providerEndpointSaveLoading"
          @click="closeProviderEndpointModal"
        />
        <Button
          label="确定"
          :loading="providerEndpointSaveLoading"
          @click="handleSaveProviderEndpoint"
        />
      </template>
    </AdminDialogShell>

    <!-- APIKey 配置弹窗 -->
    <AdminDialogShell
      v-model:visible="showAccountModal"
      :header="isEditAccount ? '编辑 APIKey' : '新增 APIKey'"
      :busy="accountSaveLoading"
    >
      <div
        v-if="selectedProviderEndpointForAccount"
        class="mb-4 flex items-center gap-1.5 rounded-xl border border-admin-border bg-admin-surface-muted px-3 py-2 text-admin-meta text-admin-text-2"
      >
        <span>所属端点：</span>
        <code class="font-semibold text-admin-text-1">{{
          selectedProviderEndpointForAccount.name
        }}</code>
      </div>

      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="APIKey 名称"
        for="admin-account-name"
        :error="accountFormErrors.name"
      >
        <InputText
          id="admin-account-name"
          v-model="accountForm.name"
          class="w-full"
          placeholder="用于区分同一端点下的不同 APIKey"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @update:model-value="clearAccountFormError('name')"
        />
      </AdminFormField>

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
          :placeholder="isEditAccount ? '留空则保持原密钥' : '输入此供应商端点的 API Key'"
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
          complete-on-focus
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
      <template #footer>
        <Button
          label="取消"
          severity="secondary"
          outlined
          :disabled="accountSaveLoading"
          @click="closeAccountModal"
        />
        <Button label="确定" :loading="accountSaveLoading" @click="accountEditor.save" />
      </template>
    </AdminDialogShell>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import AutoComplete from 'primevue/autocomplete'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Chip from 'primevue/chip'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Panel from 'primevue/panel'
import ProgressBar from 'primevue/progressbar'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { EditPencil, Plus, Refresh, Trash } from '@iconoir/vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminResourceGridFrame from '../../shared/components/AdminResourceGridFrame.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { adminCardPt } from '../../shared/lib/adminCardPt'
import { adminCompactPanelPt } from '../../shared/lib/adminPanelPt'
import {
  adminCompactTagClass,
  adminActionButtonClass,
  adminFramedIconButtonClass,
  adminMasonryCardClass,
  adminTextButtonClass,
} from '../../shared/lib/adminUiClasses'
import { useAdminProviderConfig } from '../../provider-config/useAdminProviderConfig'
import { useAdminProviderConfigFeedback } from '../../provider-config/useAdminProviderConfigFeedback'

const feedback = useAdminProviderConfigFeedback()
const adminConfirm = useAdminConfirm()

const { cards, providerEndpointEditor, accountEditor, commands } = useAdminProviderConfig({
  feedback,
  confirm: adminConfirm,
})

const searchKeyword = cards.searchKeyword
const providerEndpointCards = cards.items
const isProviderEndpointLoading = cards.isLoading
const hasProviderEndpointSearch = cards.hasSearch
const totalProviderEndpointsLabel = cards.totalProviderEndpointsLabel
const emptyProviderEndpointMessage = cards.emptyMessage
const accountRunningLabel = cards.accountRunningLabel
const activeModelCount = cards.activeModelCount

const showProviderEndpointModal = providerEndpointEditor.visible
const isEditProviderEndpoint = providerEndpointEditor.isEditing
const providerEndpointSaveLoading = providerEndpointEditor.saveLoading
const providerEndpointForm = providerEndpointEditor.form
const providerEndpointFormErrors = providerEndpointEditor.errors

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

function closeProviderEndpointModal() {
  providerEndpointEditor.close()
}

async function handleSaveProviderEndpoint() {
  await providerEndpointEditor.save()
}

function clearProviderEndpointFormError(field: keyof typeof providerEndpointForm.value) {
  providerEndpointEditor.clearError(field)
}

const showAccountModal = accountEditor.visible
const isEditAccount = accountEditor.isEditing
const accountSaveLoading = accountEditor.saveLoading
const accountLimits = accountEditor.accountLimits
const selectedProviderEndpointForAccount = accountEditor.selectedProviderEndpoint
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
