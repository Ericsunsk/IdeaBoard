<template>
  <Card :class="adminMasonryCardClass" :pt="adminCardPt">
    <template #title>
      <div class="flex min-w-0 items-center justify-between gap-3">
        <span class="truncate text-admin-title-sm font-bold text-admin-title-color">运行参数</span>
        <Tag
          :value="operationalStatusLabel"
          class="shrink-0"
          :severity="operationalStatusSeverity"
          rounded
        />
      </div>
    </template>

    <template #content>
      <div class="flex flex-col gap-5">
        <Message v-if="error" severity="error" :closable="false">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span>运行参数加载失败，请重试。</span>
            <Button label="重试" severity="secondary" size="small" outlined @click="refetch()" />
          </div>
        </Message>
        <Message v-else-if="loading && !state" severity="secondary" :closable="false">
          正在加载运行参数…
        </Message>

        <section class="grid gap-3" aria-labelledby="operational-task-runtime-title">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <h3
              id="operational-task-runtime-title"
              class="m-0 text-admin-body font-bold text-admin-text-1"
            >
              Task Runtime
            </h3>
            <Tag value="进程启动时生效" severity="secondary" rounded />
          </div>

          <Message
            v-if="state"
            :severity="state.task_runtime.restart_required ? 'warn' : 'success'"
            :closable="false"
          >
            {{
              state.task_runtime.restart_required
                ? '已保存配置尚未应用，重启 API/Worker 后生效。'
                : '已保存配置已在当前进程生效。'
            }}{{ taskRuntimeEffectiveLabel }}
          </Message>

          <div class="grid gap-4 sm:grid-cols-2">
            <AdminFormField label="Worker 并发" for="operational-worker-concurrency">
              <InputNumber
                v-model="form.task_runtime.worker_concurrency"
                input-id="operational-worker-concurrency"
                :min="1"
                :max="64"
                fluid
                :disabled="controlsDisabled"
              />
            </AdminFormField>
            <AdminFormField label="Task 超时 (秒)" for="operational-task-timeout">
              <InputNumber
                v-model="form.task_runtime.task_timeout_seconds"
                input-id="operational-task-timeout"
                :min="30"
                :max="3600"
                fluid
                :disabled="controlsDisabled"
              />
            </AdminFormField>
            <AdminFormField label="无容量重试间隔 (秒)" for="operational-no-capacity-retry">
              <InputNumber
                v-model="form.task_runtime.no_capacity_retry_seconds"
                input-id="operational-no-capacity-retry"
                :min="1"
                :max="3600"
                fluid
                :disabled="controlsDisabled"
              />
            </AdminFormField>
            <AdminFormField label="最大重试次数" for="operational-max-retry">
              <InputNumber
                v-model="form.task_runtime.max_retry"
                input-id="operational-max-retry"
                :min="1"
                :max="100"
                fluid
                :disabled="controlsDisabled"
              />
            </AdminFormField>
          </div>
        </section>

        <section
          class="grid gap-3 border-t border-admin-border-soft pt-5"
          aria-labelledby="operational-egress-title"
        >
          <div class="flex flex-wrap items-center justify-between gap-2">
            <h3
              id="operational-egress-title"
              class="m-0 text-admin-body font-bold text-admin-text-1"
            >
              出站访问策略
            </h3>
            <Tag value="即时生效" severity="success" rounded />
          </div>
          <Message v-if="state" severity="success" :closable="false">
            allowlist 保存后即时生效。当前生效：<span class="break-all">{{
              outboundPolicyEffectiveLabel
            }}</span
            >。
          </Message>
          <AdminFormField
            v-slot="{ describedbyId }"
            label="出站白名单"
            for="operational-allow-hosts"
            hint="每行一个；不填写协议或路径。留空表示不额外放行。"
          >
            <Textarea
              id="operational-allow-hosts"
              v-model="form.outbound_allow_hosts"
              :rows="4"
              fluid
              :disabled="controlsDisabled"
              :aria-describedby="describedbyId"
            />
          </AdminFormField>
        </section>

        <section
          class="grid gap-3 border-t border-admin-border-soft pt-5"
          aria-labelledby="operational-retention-title"
        >
          <div class="flex flex-wrap items-center justify-between gap-2">
            <h3
              id="operational-retention-title"
              class="m-0 text-admin-body font-bold text-admin-text-1"
            >
              Event Retention
            </h3>
            <Tag value="API 启动时生效" severity="secondary" rounded />
          </div>
          <Message
            v-if="state"
            :severity="state.event_retention.restart_required ? 'warn' : 'success'"
            :closable="false"
          >
            {{
              state.event_retention.restart_required
                ? '已保存留存周期尚未应用，重启 API 后生效。'
                : '已保存留存周期已在当前进程生效。'
            }}当前生效：{{ state.event_retention.effective.nats_retention_days }} 天。
          </Message>
          <AdminFormField label="NATS 留存天数" for="operational-nats-retention-days">
            <InputNumber
              v-model="form.nats_retention_days"
              input-id="operational-nats-retention-days"
              :min="1"
              :max="3650"
              suffix=" 天"
              fluid
              :disabled="controlsDisabled"
            />
          </AdminFormField>
        </section>

        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          label="变更原因"
          for="operational-settings-reason"
          hint="必填，1–1000 个字符；将进入 append-only audit。"
          :error="validationError"
          required
        >
          <Textarea
            id="operational-settings-reason"
            v-model="form.reason"
            :rows="3"
            fluid
            :invalid="invalid"
            :disabled="controlsDisabled"
            :aria-describedby="describedbyId"
          />
        </AdminFormField>

        <div class="flex flex-wrap items-center gap-2 pt-1">
          <Button
            :class="adminActionButtonClass"
            label="保存运行参数"
            severity="contrast"
            :disabled="controlsDisabled"
            :loading="saving"
            @click="save"
          />
          <Button
            :class="adminActionButtonClass"
            label="重置"
            severity="secondary"
            outlined
            :disabled="controlsDisabled"
            @click="reset"
          />
        </div>
      </div>
    </template>
  </Card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import AdminFormField from '../shared/components/AdminFormField.vue'
import { adminCardPt } from '../shared/lib/adminCardPt'
import { adminActionButtonClass, adminMasonryCardClass } from '../shared/lib/adminUiClasses'
import { useAdminOperationalSettings } from './useAdminOperationalSettings'
import { useAdminOperationalSettingsFeedback } from './useAdminSystemSettingsFeedback'

const feedback = useAdminOperationalSettingsFeedback()
const { error, form, loading, refetch, reset, save, saving, state, validationError } =
  useAdminOperationalSettings({ feedback })

const controlsDisabled = computed(() => loading.value || saving.value || Boolean(error.value))
const restartRequiredCount = computed(
  () =>
    Number(Boolean(state.value?.task_runtime.restart_required)) +
    Number(Boolean(state.value?.outbound_policy.restart_required)) +
    Number(Boolean(state.value?.event_retention.restart_required)),
)
const operationalStatusLabel = computed(() => {
  if (error.value) return '加载失败'
  if (!state.value) return '加载中'
  return restartRequiredCount.value ? `需重启 ${restartRequiredCount.value} 项` : '配置已生效'
})
const operationalStatusSeverity = computed(() => {
  if (error.value) return 'danger'
  if (!state.value) return 'secondary'
  return restartRequiredCount.value ? 'warn' : 'success'
})
const taskRuntimeEffectiveLabel = computed(() => {
  const effective = state.value?.task_runtime.effective
  if (!effective) return ''
  return `当前生效：并发 ${effective.worker_concurrency} · 超时 ${effective.task_timeout_seconds} 秒 · 无容量重试 ${effective.no_capacity_retry_seconds} 秒 · 最大重试 ${effective.max_retry} 次。`
})
const outboundPolicyEffectiveLabel = computed(() => {
  const allowHosts = state.value?.outbound_policy.effective.allow_hosts ?? []
  return allowHosts.length ? allowHosts.join('、') : '未放行额外主机'
})
</script>
