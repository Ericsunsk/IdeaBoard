<template>
  <div class="flex min-w-0 flex-col gap-4">
    <AdminSurfaceFrame content-class="p-0">
      <div
        v-if="showLoading"
        class="flex min-h-72 items-center justify-center"
        aria-label="加载账本健康状态"
      >
        <ProgressSpinner class="h-9 w-9 text-admin-accent" stroke-width="4" />
      </div>

      <div
        v-else-if="loadError"
        class="flex min-h-72 flex-col items-center justify-center px-6 py-12 text-center"
      >
        <XmarkCircle class="size-8 text-admin-danger" aria-hidden="true" />
        <h2 class="mt-4 text-admin-title font-bold text-admin-title-color">账本健康状态不可用</h2>
        <p class="mt-2 text-admin-body text-admin-subtitle-color">数据加载失败，请重试。</p>
        <Button
          class="mt-5"
          label="重新加载"
          :class="adminActionButtonClass"
          severity="secondary"
          outlined
          :loading="resource.query.isFetching.value"
          @click="resource.query.refetch()"
        />
      </div>

      <div v-else-if="health && presentation" class="min-w-0">
        <div class="flex flex-col gap-6 px-6 py-6 lg:flex-row lg:items-start lg:justify-between">
          <div class="flex min-w-0 items-start gap-4">
            <span class="mt-0.5 shrink-0" :class="presentation.iconClass" aria-hidden="true">
              <CheckCircle v-if="presentation.iconKey === 'healthy'" class="size-7" />
              <Clock v-else-if="presentation.iconKey === 'time'" class="size-7" />
              <XmarkCircle v-else-if="presentation.iconKey === 'failed'" class="size-7" />
              <WarningTriangle v-else class="size-7" />
            </span>
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2.5">
                <h2 class="m-0 text-admin-title font-bold text-admin-title-color">
                  {{ presentation.label }}
                </h2>
                <span
                  v-if="health.active_run"
                  class="rounded-[var(--radius-ib-pill)] border border-admin-border px-2 py-1 text-admin-caption font-bold text-admin-text-2"
                >
                  检查进行中 · Run #{{ health.active_run.id }}
                </span>
              </div>
              <p class="mt-1.5 text-admin-body leading-6 text-admin-subtitle-color">
                {{ presentation.summary }}
              </p>
            </div>
          </div>

          <div class="flex shrink-0 flex-wrap items-center gap-2">
            <Button
              label="检查记录"
              :class="adminActionButtonClass"
              severity="secondary"
              outlined
              @click="historyDialogVisible = true"
            />
            <Button
              :label="health.active_run ? '检查中' : '重新检查'"
              :class="adminActionButtonClass"
              severity="contrast"
              :disabled="Boolean(health.active_run) || resource.startPending.value"
              :loading="resource.startPending.value"
              @click="startReconciliation"
            />
          </div>
        </div>

        <Message
          v-if="resource.query.isError.value"
          class="mx-6 mb-5"
          severity="secondary"
          :closable="false"
        >
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span>状态刷新失败，当前显示上次加载的数据。</span>
            <Button
              label="重试刷新"
              severity="secondary"
              size="small"
              text
              :class="adminTextButtonClass"
              :loading="resource.query.isFetching.value"
              @click="resource.query.refetch()"
            />
          </div>
        </Message>

        <div class="grid border-t border-admin-border-soft sm:grid-cols-2 xl:grid-cols-5">
          <div
            v-for="fact in facts"
            :key="fact.label"
            class="min-w-0 border-b border-admin-border-soft px-6 py-5 sm:odd:border-r xl:border-r xl:border-b-0 xl:last:border-r-0"
          >
            <div class="text-admin-caption font-bold tracking-wide text-admin-text-3">
              {{ fact.label }}
            </div>
            <div
              class="mt-2 truncate text-admin-content font-bold text-admin-text-1"
              :title="fact.value"
            >
              {{ fact.value }}
            </div>
            <div v-if="fact.detail" class="mt-1 text-admin-caption text-admin-text-3">
              {{ fact.detail }}
            </div>
          </div>
        </div>

        <div
          v-if="health.latest_failed_run?.error_message"
          class="border-t border-admin-border-soft px-6 py-4 text-admin-body text-admin-text-2"
        >
          <span class="font-bold text-admin-title-color">最近失败：</span>
          {{ formatReconciliationRunError(health.latest_failed_run.error_message) }}
        </div>
      </div>
    </AdminSurfaceFrame>

    <AdminCreditFindingQueue />
  </div>

  <AdminCreditRunHistoryDialog v-model:visible="historyDialogVisible" />
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { CheckCircle, Clock, WarningTriangle, XmarkCircle } from '@iconoir/vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import AdminSurfaceFrame from '../../shared/components/AdminSurfaceFrame.vue'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/toast'
import { adminActionButtonClass, adminTextButtonClass } from '../../shared/lib/uiClasses'
import { formatAdminDateTime, formatReconciliationRunError } from '../format'
import AdminCreditFindingQueue from './AdminCreditFindingQueue.vue'
import AdminCreditRunHistoryDialog from './AdminCreditRunHistoryDialog.vue'
import { useAdminCreditHealthResource } from './resource'
import { getCreditHealthPresentation } from './creditHealthPresentation'

const toast = useAdminToast()
const resource = useAdminCreditHealthResource()
const historyDialogVisible = ref(false)
const manualReconciliationReason = '管理员手动触发账本检查'
const health = resource.health
const showLoading = computed(() => resource.query.isLoading.value && !health.value)
const loadError = computed(() => resource.query.isError.value && !health.value)
const presentation = computed(() =>
  health.value ? getCreditHealthPresentation(health.value.status) : null,
)
const facts = computed(() => {
  const value = health.value
  if (!value) return []
  return [
    {
      label: '最近成功检查',
      value: value.latest_successful_run?.completed_at
        ? formatAdminDateTime(value.latest_successful_run.completed_at)
        : '—',
      detail: value.latest_successful_run ? `Run #${value.latest_successful_run.id}` : '',
    },
    {
      label: '待处理异常',
      value: String(value.open_findings.total),
      detail: `${value.open_findings.warning} warning · ${value.open_findings.error} error`,
    },
    {
      label: '高风险异常',
      value: String(value.open_findings.high_risk),
      detail: `${value.open_findings.critical} critical`,
    },
    {
      label: '受限 Wallet',
      value: String(value.restricted_wallet_count),
      detail: 'Ledger Integrity',
    },
    {
      label: '最近失败检查',
      value: value.latest_failed_run?.completed_at
        ? formatAdminDateTime(value.latest_failed_run.completed_at)
        : '—',
      detail: value.latest_failed_run ? `Run #${value.latest_failed_run.id}` : '',
    },
  ]
})

async function startReconciliation() {
  try {
    await resource.start({ reason: manualReconciliationReason })
    toast.success('账本检查已启动')
  } catch (error) {
    toast.error(formatAdminDisplayError(error, '启动账本检查失败，请重试'))
  }
}
</script>
