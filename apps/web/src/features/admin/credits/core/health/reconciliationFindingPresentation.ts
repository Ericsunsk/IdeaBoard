import type {
  CreditReconciliationFinding,
  CreditReconciliationFindingKind,
} from '../../session/shared/api'

interface FindingTypePresentation {
  title: string
  summary: string
}

const findingTypePresentation = {
  ledger_postings_invalid: {
    title: 'Ledger Posting 不平衡',
    summary: 'Transaction 的 Posting 数量、资产或净额不符合 Double-entry 约束。',
  },
  command_shape_invalid: {
    title: 'Ledger Command 结构异常',
    summary: 'Transaction 的账户流向或命令形态不符合 Credits contract。',
  },
  wallet_projection_mismatch: {
    title: 'Wallet 余额投影不一致',
    summary: '账户余额 projection 与不可变 Ledger 汇总结果不一致。',
  },
  stale_hold: {
    title: 'Hold 长时间未结算',
    summary: 'Hold 已超过 24 小时仍未 Capture 或 Release，仅报告不限制 Wallet。',
  },
  task_hold_missing_billing_snapshot: {
    title: 'Task Hold 缺少 Billing Snapshot',
    summary: 'Task Hold 没有不可变 Billing Snapshot，属于资金事实缺失，已限制 Wallet。',
  },
  task_trace_missing: {
    title: 'Task 追踪记录缺失',
    summary:
      'Billing Snapshot、Settlement、Hold 与 Ledger 完整，但对应 Task 已缺失，仅报告不限制 Wallet。',
  },
  task_billing_fact_missing: {
    title: 'Task Billing 事实不完整',
    summary:
      'Task Billing Snapshot、Hold 或 Settlement 缺失，或它们的 Wallet、金额与业务引用不一致。',
  },
  active_task_not_held: {
    title: '活动 Task 未保持 Hold',
    summary: 'Pending 或 Running Task 的 Billing settlement 未保持 held 状态。',
  },
  terminal_task_settlement_mismatch: {
    title: '终态 Task 结算不一致',
    summary: 'Task 的持久化结果数量与 Capture、Release 结算事实不一致。',
  },
  task_terminal_transaction_count: {
    title: 'Task 终态交易数量异常',
    summary: 'Task 的 Capture、Release 交易数量与实际结算金额不一致。',
  },
  idempotency_request_hash_conflict: {
    title: 'Idempotency 请求冲突',
    summary: '同一 idempotency key 对应了不同 canonical request hash。',
  },
  task_settlement_reference_invalid: {
    title: 'Task Settlement 引用异常',
    summary: 'Settlement 指向的终态交易与 Task、Wallet 或 Hold 不匹配。',
  },
  duplicate_business_reference: {
    title: '业务引用重复入账',
    summary: '同一 Wallet、business reference 与 command 出现多笔交易。',
  },
  correction_amount_exceeds_original: {
    title: 'Correction 超出原交易',
    summary: '累计 Correction 金额或目标类型超出允许范围。',
  },
  refund_amount_invalid: {
    title: 'Refund 金额不一致',
    summary: 'Charge 的退款 projection 与 Ledger 中的净退款金额不一致。',
  },
  illegal_reference_chain: {
    title: 'Ledger 引用链非法',
    summary: '交易关联的上游 command、Wallet 或业务引用不符合规则。',
  },
  hold_state_invalid: {
    title: 'Hold 状态不一致',
    summary: 'Hold projection 与 Hold、Capture 或 Release 交易事实不一致。',
  },
  hold_terminal_transaction_count: {
    title: 'Hold 终态交易数量异常',
    summary: 'Hold 关联的 Capture 或 Release 数量与当前状态不一致。',
  },
  charge_state_invalid: {
    title: 'Charge 状态不一致',
    summary: 'Charge projection 与 Capture 交易的 Wallet 或金额不一致。',
  },
} satisfies Record<CreditReconciliationFindingKind, FindingTypePresentation>

export function getFindingTypePresentation(findingType: string): FindingTypePresentation {
  return (
    findingTypePresentation[findingType as CreditReconciliationFindingKind] ?? {
      title: '未知账本异常',
      summary: `服务返回了尚未识别的 Finding 类型：${findingType || 'unknown'}。`,
    }
  )
}

const severityLabels: Record<CreditReconciliationFinding['severity'], string> = {
  warning: '警告',
  error: '错误',
  critical: '严重',
}

export function getFindingRiskPresentation(finding: CreditReconciliationFinding) {
  const severity = severityLabels[finding.severity]
  const risk = finding.risk === 'high' ? '高风险' : '低风险'
  const restriction = finding.isolated
    ? 'Wallet 已限制'
    : finding.risk === 'low'
      ? '仅报告'
      : '未限制'
  return {
    label: `${severity} · ${risk} · ${restriction}`,
    variant:
      finding.risk === 'high' || finding.isolated ? ('danger' as const) : ('warning' as const),
  }
}

export function formatFindingDetailsEntries(details: CreditReconciliationFinding['details']) {
  return Object.entries(details).map(([key, value]) => ({
    key,
    value: formatFindingDetailValue(value),
  }))
}

function formatFindingDetailValue(value: unknown): string {
  if (value == null) return '—'
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
    return String(value)
  }
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return '[无法序列化的值]'
  }
}
