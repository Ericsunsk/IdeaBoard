<template>
  <AdminDialogShell
    v-model:visible="showAccountModal"
    :header="isEditAccount ? '编辑 APIKey' : '新增 APIKey'"
    :busy="accountSaveLoading"
  >
    <div
      v-if="selectedProviderEndpointForAccount"
      class="flex items-center justify-between gap-3 rounded-xl border border-admin-border-soft bg-admin-surface-muted/60 px-3.5 py-2.5"
    >
      <div class="flex min-w-0 items-center gap-2">
        <span class="text-xs text-admin-text-3">所属端点</span>
        <span class="truncate font-mono text-xs font-semibold text-admin-title-color">
          {{ selectedProviderEndpointForAccount.name }}
        </span>
      </div>
      <span
        class="shrink-0 rounded-md bg-white border border-admin-border-soft px-2 py-0.5 font-mono text-[10px] font-semibold text-admin-text-2 uppercase"
      >
        {{ selectedProviderEndpointForAccount.apiTypeLabel }}
      </span>
    </div>

    <AdminFormField
      v-slot="{ describedbyId, invalid }"
      label="名称标识"
      for="admin-account-name"
      :error="accountFormErrors.name"
    >
      <InputText
        id="admin-account-name"
        v-model="accountForm.name"
        class="w-full"
        placeholder="例如：主账号、备用Key-01"
        :invalid="invalid"
        :aria-describedby="describedbyId"
        @update:model-value="clearAccountFormError('name')"
      />
    </AdminFormField>

    <AdminFormField
      v-slot="{ describedbyId, invalid }"
      label="API Key (密钥)"
      for="admin-account-secret"
      :error="accountFormErrors.secret"
    >
      <InputText
        id="admin-account-secret"
        v-model="accountForm.secret"
        type="password"
        class="w-full font-mono text-xs"
        autocomplete="new-password"
        :placeholder="
          isEditAccount ? '留空表示保持原有密钥不变' : '请输入该账号对应的 API Key'
        "
        :invalid="invalid"
        :aria-describedby="describedbyId"
        @update:model-value="clearAccountFormError('secret')"
      />
    </AdminFormField>

    <div
      v-if="!isModellessProviderEndpoint(selectedProviderEndpointForAccount)"
      class="flex flex-col gap-3"
    >
      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="分配模型"
        for="admin-account-models"
        :error="accountFormErrors.modelValues"
      >
        <div class="flex flex-col gap-2">
          <!-- 搜索与添加输入框 -->
          <div class="flex items-stretch gap-2">
            <AutoComplete
              id="admin-account-models"
              v-model="modelSearchInput"
              class="flex-1"
              :suggestions="filteredSuggestions"
              :loading="detectModelsLoading"
              placeholder="输入模型名称回车，或从下拉列表选择..."
              complete-on-focus
              fluid
              :invalid="invalid"
              :aria-describedby="describedbyId"
              @focus="accountEditor.handleModelSelectShow(true)"
              @complete="searchModelSuggestions"
              @item-select="handleSelectSuggestedModel"
              @keydown.enter.prevent="handleAddCustomModel"
            />
            <Button
              severity="contrast"
              outlined
              class="aspect-square shrink-0 !rounded-lg !p-0 flex items-center justify-center"
              :disabled="!modelSearchInput.trim()"
              aria-label="添加模型"
              title="添加模型"
              @click="handleAddCustomModel"
            >
              <template #icon>
                <Plus class="size-4" />
              </template>
            </Button>
          </div>

          <!-- 探测到的可用模型一键填入 (如果存在未添加的推荐模型) -->
          <div
            v-if="unassignedSuggestions.length > 0"
            class="flex flex-wrap items-center gap-1.5 pt-0.5"
          >
            <span class="text-admin-caption font-semibold text-admin-text-3">探测到模型：</span>
            <button
              v-for="suggested in unassignedSuggestions.slice(0, 5)"
              :key="suggested"
              type="button"
              class="group flex items-center gap-1 cursor-pointer rounded-md border border-admin-border-soft bg-white px-2 py-0.5 text-admin-caption font-mono font-medium text-admin-accent hover:border-admin-accent transition-colors"
              @click="addAccountModel(suggested)"
            >
              <span>+</span>
              <span>{{ suggested }}</span>
            </button>
            <button
              v-if="unassignedSuggestions.length > 1"
              type="button"
              class="cursor-pointer rounded-md border border-transparent bg-admin-accent/10 px-2 py-0.5 text-admin-caption font-semibold text-admin-accent hover:bg-admin-accent/20 transition-colors"
              @click="addAllSuggestedModels"
            >
              + 全部添加 ({{ unassignedSuggestions.length }})
            </button>
          </div>
        </div>
      </AdminFormField>

      <!-- 已分配模型卡片列表 (一体化整合 2K/4K 特性与移除操作) -->
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between px-0.5">
          <span class="text-xs font-semibold text-admin-text-2">已分配模型及特性配置</span>
          <span class="font-mono text-admin-caption text-admin-text-3">
            {{ accountForm.modelValues.length }} 个模型
          </span>
        </div>

        <div
          v-if="accountForm.modelValues.length > 0"
          class="flex flex-col divide-y divide-admin-border-soft rounded-xl border border-admin-border-soft bg-white overflow-hidden shadow-2xs"
        >
          <div
            v-for="modelName in accountForm.modelValues"
            :key="`model-row:${modelName}`"
            class="flex items-center justify-between gap-3 px-3.5 py-2.5 transition-colors hover:bg-admin-surface-muted/30"
          >
            <div class="flex min-w-0 items-center gap-2">
              <span class="size-1.5 shrink-0 rounded-full bg-admin-accent" />
              <span class="truncate font-mono text-xs font-semibold text-admin-title-color">
                {{ modelName }}
              </span>
            </div>

            <div class="flex shrink-0 items-center gap-4">
              <label
                class="flex items-center gap-1.5 cursor-pointer select-none text-admin-caption font-medium text-admin-text-2 hover:text-admin-title-color transition-colors"
              >
                <span class="font-mono font-semibold">2K</span>
                <ToggleSwitch
                  :model-value="modelCapability(modelName).supports2K"
                  size="small"
                  :aria-label="`${modelName} 2K 支持`"
                  @update:model-value="setModelCapability(modelName, 'supports2K', $event)"
                />
              </label>

              <label
                class="flex items-center gap-1.5 cursor-pointer select-none text-admin-caption font-medium text-admin-text-2 hover:text-admin-title-color transition-colors"
              >
                <span class="font-mono font-semibold">4K</span>
                <ToggleSwitch
                  :model-value="modelCapability(modelName).supports4K"
                  size="small"
                  :aria-label="`${modelName} 4K 支持`"
                  @update:model-value="setModelCapability(modelName, 'supports4K', $event)"
                />
              </label>

              <button
                type="button"
                class="flex size-6 items-center justify-center rounded-md border-0 bg-transparent text-admin-text-3 transition-colors hover:bg-admin-danger/10 hover:text-admin-danger cursor-pointer p-0 focus-visible:outline-none"
                :title="`移除 ${modelName}`"
                :aria-label="`移除 ${modelName}`"
                @click="removeAccountModel(modelName)"
              >
                <Xmark class="size-3.5" />
              </button>
            </div>
          </div>
        </div>

        <div
          v-else
          class="flex items-center justify-center rounded-xl border border-dashed border-admin-border bg-admin-surface-muted/30 py-4 text-xs text-admin-text-3"
        >
          暂未分配模型，请在上方输入模型名称或从推荐列表中添加
        </div>
      </div>
    </div>

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
      <div class="flex w-full items-center justify-between gap-2">
        <div>
          <Button
            v-if="isEditAccount"
            label="删除"
            severity="danger"
            outlined
            :class="adminActionButtonClass"
            :disabled="accountSaveLoading"
            @click="handleDeleteAccountFromModal"
          />
        </div>
        <div class="flex items-center gap-2">
          <Button
            label="取消"
            :class="adminActionButtonClass"
            severity="secondary"
            outlined
            :disabled="accountSaveLoading"
            @click="closeAccountModal"
          />
          <Button
            label="保存"
            :class="adminActionButtonClass"
            severity="contrast"
            :loading="accountSaveLoading"
            @click="accountEditor.save"
          />
        </div>
      </div>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import AutoComplete from 'primevue/autocomplete'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import ToggleSwitch from 'primevue/toggleswitch'
import { Plus, Xmark } from '@iconoir/vue'
import AdminDialogShell from '../shared/components/AdminDialogShell.vue'
import AdminFormField from '../shared/components/AdminFormField.vue'
import { adminActionButtonClass } from '../shared/lib/uiClasses'
import { isModellessProviderEndpoint } from './payload'
import { useAdminProviderConfigContext } from './useProviderConfig'

const { accountEditor, commands } = useAdminProviderConfigContext()

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
const modelSearchInput = accountEditor.modelSearchInput

const filteredSuggestions = computed(() => {
  const query = modelSearchInput.value.trim().toLowerCase()
  if (!query) return modelSuggestions.value
  return modelSuggestions.value.filter((s) => s.toLowerCase().includes(query))
})

const unassignedSuggestions = computed(() => {
  const assigned = new Set(accountForm.value.modelValues.map((m) => m.toLowerCase()))
  return modelSuggestions.value.filter((s) => !assigned.has(s.toLowerCase()))
})

function addAccountModel(modelName: string) {
  const trimmed = modelName.trim()
  if (!trimmed) return
  if (!accountForm.value.modelValues.some((m) => m.toLowerCase() === trimmed.toLowerCase())) {
    accountForm.value.modelValues = [...accountForm.value.modelValues, trimmed]
  }
  clearAccountFormError('modelValues')
}

function removeAccountModel(modelName: string) {
  accountForm.value.modelValues = accountForm.value.modelValues.filter(
    (m) => m.toLowerCase() !== modelName.toLowerCase(),
  )
}

function addAllSuggestedModels() {
  const assigned = new Set(accountForm.value.modelValues.map((m) => m.toLowerCase()))
  const toAdd = modelSuggestions.value.filter((s) => !assigned.has(s.toLowerCase()))
  accountForm.value.modelValues = [...accountForm.value.modelValues, ...toAdd]
  clearAccountFormError('modelValues')
}

function handleAddCustomModel() {
  const val = modelSearchInput.value.trim()
  if (!val) return
  addAccountModel(val)
  modelSearchInput.value = ''
}

function handleSelectSuggestedModel(event: { value: string }) {
  if (event.value) {
    addAccountModel(event.value)
    modelSearchInput.value = ''
  }
}

function closeAccountModal() {
  accountEditor.close()
}

function clearAccountFormError(field: Parameters<typeof accountEditor.clearError>[0]) {
  accountEditor.clearError(field)
}

function modelCapability(modelName: string) {
  const key = modelName.trim().toLowerCase()
  return (accountForm.value.modelCapabilities[key] ??= { supports2K: false, supports4K: false })
}

function setModelCapability(modelName: string, key: 'supports2K' | 'supports4K', value: boolean) {
  const capability = modelCapability(modelName)
  capability[key] = value
}

async function handleDeleteAccountFromModal() {
  if (!accountEditor.currentAccountTarget.value) return
  const target = accountEditor.currentAccountTarget.value
  commands.deleteAccount(target, { afterDeleted: closeAccountModal })
}
</script>
