<template>
  <div>
    <AdminResourceGridFrame
      empty-description=""
      empty-title=""
      :is-empty="providerEndpointCards.length === 0"
      :is-loading="isProviderEndpointLoading"
    >
      <template #toolbar>
        <AdminResourceToolbar>
          <template #search>
            <AdminToolbarSearch v-model="searchKeyword" input-label="搜索供应商端点" />
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
              label="新增端点"
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

      <template #prepend>
        <SearchProviderCard />
      </template>
      <Card
        v-for="card in providerEndpointCards"
        :key="card.providerEndpoint.id"
        :class="[adminMasonryCardClass, 'min-w-0']"
        :pt="adminCardPt"
      >
        <template #title>
          <div class="flex items-start justify-between gap-3">
            <div class="flex min-w-0 items-center gap-3">
              <button
                type="button"
                :class="[adminLogoButtonClass, 'size-10 p-2']"
                :data-control-state="activeLogoTarget?.id === card.providerEndpoint.id ? 'selected' : 'idle'"
                :aria-expanded="activeLogoTarget?.id === card.providerEndpoint.id ? 'true' : 'false'"
                title="点击切换端点 Logo"
                aria-label="切换端点 Logo"
                @click="openLogoPicker($event, card)"
              >
                <img
                  v-if="
                    getProviderEndpointLogo(
                      {
                        name: card.providerEndpoint.name,
                        baseUrl: card.providerEndpoint.baseUrl,
                        apiType: card.providerEndpoint.apiType,
                      },
                      card.providerEndpoint.id,
                    )
                  "
                  :src="
                    getProviderEndpointLogo(
                      {
                        name: card.providerEndpoint.name,
                        baseUrl: card.providerEndpoint.baseUrl,
                        apiType: card.providerEndpoint.apiType,
                      },
                      card.providerEndpoint.id,
                    )
                  "
                  :alt="card.providerEndpoint.name || 'Provider'"
                  class="size-full object-contain"
                />
                <span
                  v-else
                  class="text-xs font-bold text-admin-accent uppercase select-none"
                >
                  {{ (card.providerEndpoint.name || 'EP').substring(0, 2).toUpperCase() }}
                </span>
              </button>
              <div class="min-w-0">
                <div class="truncate text-admin-title-sm font-bold text-admin-title-color">
                  {{ card.providerEndpoint.name }}
                </div>
                <div
                  class="truncate text-admin-caption font-mono text-admin-subtitle-color"
                  :title="card.providerEndpoint.baseUrl"
                >
                  {{ card.providerEndpoint.baseUrl }}
                </div>
              </div>
            </div>

            <div class="flex shrink-0 items-center gap-1.5">
              <button
                type="button"
                class="inline-flex h-6 items-center gap-1.5 px-2.5 rounded-full text-xs font-semibold leading-none border select-none shrink-0 cursor-pointer transition-colors focus-visible:outline-none"
                :class="
                  card.providerEndpoint.status === 1
                    ? 'bg-admin-success-soft text-admin-success border-admin-success/30 shadow-xs hover:border-admin-success/60'
                    : 'bg-admin-surface-muted text-admin-text-3 border-admin-border-soft hover:border-admin-border'
                "
                :title="card.providerEndpoint.status === 1 ? '点击停用端点' : '点击启用端点'"
                :aria-label="card.providerEndpoint.status === 1 ? '点击停用端点' : '点击启用端点'"
                @click.stop="commands.toggleProviderEndpointStatus({ kind: 'providerEndpoint', id: card.providerEndpoint.id })"
              >
                <span
                  class="size-1.5 rounded-full"
                  :class="
                    card.providerEndpoint.status === 1
                      ? 'bg-admin-success animate-pulse'
                      : 'bg-admin-text-3'
                  "
                />
                {{ card.providerEndpoint.status === 1 ? '启用' : '停用' }}
              </button>
            </div>
          </div>
        </template>

        <template #content>
          <div class="flex flex-col gap-3.5">
            <!-- 结构化指标栏 (Sleek Metric Strip) -->
            <div
              class="flex items-center justify-between gap-2 rounded-xl border border-admin-border-soft bg-admin-surface-muted px-3 py-2 text-xs"
            >
              <div class="flex items-center gap-2 truncate">
                <span class="font-semibold text-admin-title-color">
                  {{ card.providerEndpoint.protocolLabel }}
                </span>
                <span
                  v-if="card.providerEndpoint.protocol === 'image-url-json-to-image'"
                  class="text-admin-text-3"
                >
                  ·
                  {{
                    getAdminProviderEndpointCapabilityLabel(card.providerEndpoint.capabilities[0])
                  }}
                </span>
              </div>

              <div class="flex shrink-0 items-center gap-2">
                <span
                  v-if="card.providerEndpoint.protocol === 'image-url-json-to-image'"
                  :class="
                    card.providerEndpoint.apiKeyConfigured
                      ? 'text-admin-success font-medium'
                      : 'text-admin-warning font-medium'
                  "
                >
                  {{ card.providerEndpoint.apiKeyConfigured ? 'API Key 已配置' : '未配置 Key' }}
                </span>
                <template v-else-if="card.providerEndpoint.protocol === 'model-api'">
                  <span class="font-medium text-admin-text-2">
                    🔑 {{ card.accounts.length }} Key
                  </span>
                  <span class="text-admin-border">|</span>
                  <span class="font-medium text-admin-text-2">
                    ⚡ {{ card.modelCount }} 模型
                  </span>
                </template>
              </div>
            </div>

            <!-- Model API 关联 APIKey 与模型管理主体 -->
            <div
              v-if="card.providerEndpoint.protocol === 'model-api'"
              class="flex flex-col gap-2.5"
            >
              <!-- 扁平化 Account / APIKey 列表 -->
              <div v-if="card.accounts.length > 0" class="flex flex-col gap-2">
                <div
                  v-for="account in card.accounts"
                  :key="account.id"
                  class="flex flex-col gap-2 rounded-xl border border-admin-border-soft bg-admin-surface-muted/40 p-2.5 transition-colors hover:border-admin-border"
                >
                  <!-- Account 头部 (点击整栏进入编辑，2K/4K 胶囊并列于 0/3 旁) -->
                  <div class="flex items-center justify-between gap-2">
                    <button
                      type="button"
                      class="group flex min-w-0 w-full items-center justify-between gap-2 border-0 bg-transparent p-0 text-left cursor-pointer focus-visible:outline-none"
                      title="编辑 APIKey"
                      aria-label="编辑 APIKey"
                      @click="accountEditor.openEdit(account.commandTarget)"
                    >
                      <div class="flex min-w-0 items-center gap-1.5">
                        <span
                          class="size-1.5 shrink-0 rounded-full"
                          :class="commands.getAccountStatusDotClass(account)"
                        />
                        <span
                          class="truncate text-xs font-semibold text-admin-title-color transition-colors group-hover:text-admin-accent"
                        >
                          {{ account.name || 'APIKey' }}
                        </span>
                      </div>

                      <div class="flex shrink-0 items-center gap-1">
                        <span
                          v-if="account.models.some((m) => m.supports2K)"
                          class="shrink-0 rounded-md bg-white border border-admin-border-soft px-1.5 py-0.5 font-mono text-admin-micro font-semibold text-admin-text-2"
                        >
                          2K
                        </span>
                        <span
                          v-if="account.models.some((m) => m.supports4K)"
                          class="shrink-0 rounded-md bg-white border border-admin-border-soft px-1.5 py-0.5 font-mono text-admin-micro font-semibold text-admin-text-2"
                        >
                          4K
                        </span>
                        <span
                          class="shrink-0 rounded-md bg-white border border-admin-border-soft px-1.5 py-0.5 font-mono text-admin-micro font-semibold text-admin-text-3"
                        >
                          {{ account.concurrencyLabel }}
                        </span>
                      </div>
                    </button>
                  </div>

                  <!-- 并发占用细进度条 (有并发时显示) -->
                  <ProgressBar
                    v-if="account.concurrencyPercent > 0"
                    :value="account.concurrencyPercent"
                    :show-value="false"
                    class="!h-[2px] !rounded-full !bg-admin-border-soft"
                    :pt="commands.getAccountProgressPt(account)"
                  />

                  <!-- 模型列表 (极简无冗余) -->
                  <div class="flex flex-col gap-1">
                    <div
                      v-for="model in commands.getAccountModelRows(account)"
                      :key="`${account.id}:${model.name}`"
                      class="flex items-center justify-between gap-2 rounded-lg border border-admin-border-soft/70 bg-white px-2.5 py-1.5 transition-colors hover:border-admin-border"
                    >
                      <div class="min-w-0 flex-1 flex items-center gap-2">
                        <span
                          class="truncate text-xs font-medium"
                          :class="model.disabled ? 'text-admin-text-3' : 'text-admin-text-1'"
                        >
                          {{ model.name }}
                        </span>
                      </div>
                      <ToggleSwitch
                        :model-value="!model.disabled"
                        size="small"
                        class="!shrink-0"
                        :aria-label="`${model.name} 模型启用状态`"
                        :disabled="
                          commands.isAccountModelStatusChanging(account.commandTarget, model.name)
                        "
                        @update:model-value="
                          commands.toggleAccountModelStatus(account.commandTarget, model.name)
                        "
                      />
                    </div>
                  </div>
                </div>
              </div>

              <!-- 常驻虚线添加按钮（更紧凑的极简 + 号） -->
              <button
                type="button"
                class="group flex h-8.5 w-full items-center justify-center rounded-xl border border-dashed border-admin-border bg-transparent cursor-pointer transition-colors hover:border-admin-accent hover:bg-admin-surface-muted/60 focus-visible:outline-none focus-visible:border-admin-accent"
                title="添加 APIKey"
                aria-label="添加 APIKey"
                @click="accountEditor.openCreate(card.providerEndpoint.commandTarget)"
              >
                <Plus class="size-4 text-admin-text-3 transition-colors group-hover:text-admin-accent" />
              </button>
            </div>

            <!-- 底部端点操作栏 (编辑与删除) -->
            <div class="mt-auto flex items-center justify-between gap-2 pt-1">
              <div class="flex items-center gap-2">
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
          </div>
        </template>
      </Card>
    </AdminResourceGridFrame>

    <!-- 供应商端点表单弹窗 -->
    <AdminDialogShell
      v-model:visible="showProviderEndpointModal"
      :header="isEditProviderEndpoint ? '编辑供应商端点' : '新增端点'"
      :busy="providerEndpointSaveLoading"
    >
      <AdminFormField
        v-slot="{ describedbyId, invalid, resolvePlaceholder }"
        label="端点名称"
        for="admin-provider-endpoint-name"
        :error="providerEndpointFormErrors.name"
      >
        <InputText
          id="admin-provider-endpoint-name"
          v-model="providerEndpointForm.name"
          class="w-full"
          :placeholder="resolvePlaceholder('例如：Nano Banana 官方端点')"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @update:model-value="clearProviderEndpointFormError('name')"
        />
      </AdminFormField>

      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="请求格式"
        for="admin-provider-endpoint-protocol"
        :error="providerEndpointFormErrors.protocol"
      >
        <Select
          v-model="providerEndpointForm.protocol"
          input-id="admin-provider-endpoint-protocol"
          class="w-full"
          :options="providerProtocolOptions"
          option-label="label"
          option-value="value"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @update:model-value="handleProtocolChange"
        />
      </AdminFormField>

      <AdminFormField
        v-if="providerEndpointForm.protocol === 'image-url-json-to-image'"
        v-slot="{ describedbyId, invalid, resolvePlaceholder }"
        label="处理功能"
        for="admin-provider-endpoint-capability"
        :error="providerEndpointFormErrors.capability"
      >
        <Select
          v-model="providerEndpointForm.capability"
          input-id="admin-provider-endpoint-capability"
          class="w-full"
          :options="providerProcessingCapabilityOptions"
          option-label="label"
          option-value="value"
          :placeholder="resolvePlaceholder('选择处理功能')"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @update:model-value="clearProviderEndpointFormError('capability')"
        />
      </AdminFormField>

      <AdminFormField
        v-if="providerEndpointForm.protocol === 'model-api'"
        v-slot="{ describedbyId, invalid, resolvePlaceholder }"
        label="服务类型"
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
          :placeholder="resolvePlaceholder('选择服务类型')"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @update:model-value="clearProviderEndpointFormError('apiType')"
        />
      </AdminFormField>

      <AdminFormField
        v-slot="{ describedbyId, invalid, resolvePlaceholder }"
        label="Base URL地址"
        for="admin-provider-endpoint-api-url"
        :error="providerEndpointFormErrors.baseUrl"
      >
        <InputText
          id="admin-provider-endpoint-api-url"
          v-model="providerEndpointForm.baseUrl"
          class="w-full"
          :placeholder="resolvePlaceholder('例如：https://api.example.com/v1')"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @blur="providerEndpointEditor.handleApiUrlBlur"
          @update:model-value="clearProviderEndpointFormError('baseUrl')"
        />
      </AdminFormField>
      <AdminFormField
        v-if="isModellessProviderProtocol(providerEndpointForm.protocol)"
        v-slot="{ describedbyId, invalid, resolvePlaceholder }"
        label="API Key"
        for="admin-provider-endpoint-api-key"
        :error="providerEndpointFormErrors.apiKey"
      >
        <InputText
          id="admin-provider-endpoint-api-key"
          v-model="providerEndpointForm.apiKey"
          type="password"
          class="w-full"
          autocomplete="new-password"
          :placeholder="resolvePlaceholder('输入图片处理 API Key')"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @update:model-value="clearProviderEndpointFormError('apiKey')"
        />
      </AdminFormField>
      <template #footer>
        <Button
          label="取消"
          :class="adminActionButtonClass"
          severity="secondary"
          outlined
          :disabled="providerEndpointSaveLoading"
          @click="closeProviderEndpointModal"
        />
        <Button
          label="确定"
          :class="adminActionButtonClass"
          severity="contrast"
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
        v-slot="{ describedbyId, invalid, resolvePlaceholder }"
        label="APIKey 名称"
        for="admin-account-name"
        :error="accountFormErrors.name"
      >
        <InputText
          id="admin-account-name"
          v-model="accountForm.name"
          class="w-full"
          :placeholder="resolvePlaceholder('用于区分同一端点下的不同 APIKey')"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @update:model-value="clearAccountFormError('name')"
        />
      </AdminFormField>

      <AdminFormField
        v-slot="{ describedbyId, invalid, resolvePlaceholder }"
        :label="isEditAccount ? '修改 API 密钥 (留空不修改)' : 'API 密钥 (Key)'"
        for="admin-account-secret"
        :error="accountFormErrors.secret"
      >
        <InputText
          id="admin-account-secret"
          v-model="accountForm.secret"
          class="w-full font-mono text-xs"
          type="password"
          :placeholder="resolvePlaceholder(isEditAccount ? '留空则保持原密钥' : '输入此供应商端点的 API Key')"
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
          v-slot="{ describedbyId, invalid, resolvePlaceholder }"
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
                :placeholder="resolvePlaceholder('输入模型名称回车，或从下拉列表选择...')"
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

    <AdminLogoPickerPopover
      ref="logoPickerPopoverRef"
      scope="provider-endpoint"
      :target-id="activeLogoTarget?.id"
      :target-name="activeLogoTarget?.name"
      :target-base-url="activeLogoTarget?.baseUrl"
      :target-api-type="activeLogoTarget?.apiType"
      @hide="activeLogoTarget = null"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AutoComplete from 'primevue/autocomplete'
import Button from 'primevue/button'
import Card from 'primevue/card'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import ProgressBar from 'primevue/progressbar'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { Plus, Refresh, Xmark } from '@iconoir/vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminResourceGridFrame from '../../shared/components/AdminResourceGridFrame.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { adminCardPt } from '../../shared/lib/adminCardPt'
import {
  adminActionButtonClass,
  adminLogoButtonClass,
  adminMasonryCardClass,
} from '../../shared/lib/adminUiClasses'
import { useAdminProviderConfig } from '../../provider-config/useAdminProviderConfig'
import { useAdminProviderConfigFeedback } from '../../provider-config/useAdminProviderConfigFeedback'
import SearchProviderCard from '../../provider-config/SearchProviderCard.vue'
import AdminLogoPickerPopover from '../../provider-config/AdminLogoPickerPopover.vue'
import {
  getAdminProviderEndpointCapabilityLabel,
  getAdminProviderEndpointProtocolLabel,
  isModellessProviderEndpoint,
  isModellessProviderProtocol,
} from '../../provider-config/providerConfigPayload'
import { getProviderEndpointLogo } from '../../provider-config/providerLogos'

const feedback = useAdminProviderConfigFeedback()
const adminConfirm = useAdminConfirm()

const { cards, providerEndpointEditor, accountEditor, commands } = useAdminProviderConfig({
  feedback,
  confirm: adminConfirm,
})

const activeLogoTarget = ref<{
  id: number
  name: string
  baseUrl: string
  apiType: string
} | null>(null)
const logoPickerPopoverRef = ref<InstanceType<typeof AdminLogoPickerPopover> | null>(null)

function openLogoPicker(
  event: Event,
  card: { providerEndpoint: { id: number; name: string; baseUrl: string; apiType: string } },
) {
  activeLogoTarget.value = {
    id: card.providerEndpoint.id,
    name: card.providerEndpoint.name,
    baseUrl: card.providerEndpoint.baseUrl,
    apiType: card.providerEndpoint.apiType,
  }
  logoPickerPopoverRef.value?.toggle(event)
}

const searchKeyword = cards.searchKeyword
const providerEndpointCards = cards.items
const isProviderEndpointLoading = cards.isLoading
const totalProviderEndpointsLabel = cards.totalProviderEndpointsLabel
const accountRunningLabel = cards.accountRunningLabel
const activeModelCount = cards.activeModelCount

const showProviderEndpointModal = providerEndpointEditor.visible
const isEditProviderEndpoint = providerEndpointEditor.isEditing
const providerEndpointSaveLoading = providerEndpointEditor.saveLoading
const providerEndpointForm = providerEndpointEditor.form
const providerEndpointFormErrors = providerEndpointEditor.errors
const providerProtocolOptions = computed(() =>
  providerEndpointEditor.protocolRegistry.value.map((protocol) => ({
    label: getAdminProviderEndpointProtocolLabel(protocol.id),
    value: protocol.id,
  })),
)
const providerProcessingCapabilityOptions = computed(() => {
  const protocol = providerEndpointEditor.protocolRegistry.value.find(
    (item) => item.id === providerEndpointForm.value.protocol,
  )
  return (protocol?.capabilities ?? []).map((capability) => ({
    label: getAdminProviderEndpointCapabilityLabel(capability),
    value: capability,
  }))
})


function closeProviderEndpointModal() {
  providerEndpointEditor.close()
}

async function handleSaveProviderEndpoint() {
  await providerEndpointEditor.save()
}

function clearProviderEndpointFormError(field: keyof typeof providerEndpointForm.value) {
  providerEndpointEditor.clearError(field)
}

function handleProtocolChange(protocol: string) {
  if (protocol === 'flux-fill-json-image') {
    providerEndpointForm.value.apiType = 'self-hosted'
    providerEndpointForm.value.capability = 'image-fill'
  } else if (protocol === 'image-url-json-to-image') {
    providerEndpointForm.value.apiType = 'self-hosted'
    providerEndpointForm.value.capability = ''
  } else {
    providerEndpointForm.value.apiKey = ''
    providerEndpointForm.value.capability = ''
  }
  providerEndpointEditor.clearError('protocol')
  providerEndpointEditor.clearError('apiKey')
  providerEndpointEditor.clearError('capability')
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

onMounted(() => {
  commands.loadData()
})
</script>
